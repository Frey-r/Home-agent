// Package hatest provee un servidor falso de la WebSocket API de Home
// Assistant para tests. No se usa en código de producción.
package hatest

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
)

// Comportamientos del saludo inicial.
const (
	GreetingNormal = ""      // envía auth_required
	GreetingSilent = "none"  // no envía nada
	GreetingClose  = "close" // cierra la conexión de inmediato
	GreetingBogus  = "bogus" // envía un mensaje de tipo inesperado
)

// ErrorObj es el error que HA devuelve en un result con success=false.
type ErrorObj struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// Options configura el comportamiento del servidor falso.
type Options struct {
	Token     string // token válido (default "test-token")
	HAVersion string // default "2026.10.0"
	States    any    // respuesta de get_states (default: slice vacío)

	Greeting string // ver constantes Greeting*

	// ReverseBatch > 1 acumula esa cantidad de comandos y los responde en
	// orden inverso (para probar la correlación por id).
	ReverseBatch int
	// DropOn cierra la conexión abruptamente al recibir este tipo de comando.
	DropOn string
	// Results agrega respuestas para otros tipos de comando.
	Results map[string]any
	// ResultSeq responde llamadas sucesivas de un tipo con los valores en
	// orden; el último se repite (p. ej. get_config STARTING → RUNNING).
	ResultSeq map[string][]any
	// Errors fuerza success=false para un tipo de comando.
	Errors map[string]ErrorObj
}

type clientConn struct {
	c       *websocket.Conn
	mu      sync.Mutex
	subs    map[int64]string // subID -> event_type
	closed  bool
}

// Server es un HA falso escuchando en httptest.
type Server struct {
	URL string // ws://.../api/websocket
	srv *httptest.Server

	mu        sync.Mutex
	opts      Options
	conns     map[*clientConn]struct{}
	commands  []map[string]any
	onCommand func(cmd map[string]any)
	down      bool
	frozen    bool
	freezeCh  chan struct{}

	subChans map[string][]chan struct{} // event_type -> waiters
	states   map[string]map[string]any
}

// New levanta un servidor falso; se cierra automáticamente al terminar el test.
func New(t testing.TB, opts Options) *Server {
	t.Helper()
	s := &Server{
		opts:      opts,
		conns:     make(map[*clientConn]struct{}),
		subChans:  make(map[string][]chan struct{}),
		states:    make(map[string]map[string]any),
		freezeCh:  make(chan struct{}),
	}
	close(s.freezeCh) // initially not frozen

	// Inicializar states si se pasaron en opts
	if slice, ok := opts.States.([]map[string]any); ok {
		for _, st := range slice {
			if id, ok := st["entity_id"].(string); ok {
				s.states[id] = st
			}
		}
	}

	mux := http.NewServeMux()
	mux.Handle("/api/websocket", s.handler(t))
	srv := httptest.NewServer(mux)
	s.srv = srv
	s.URL = "ws" + strings.TrimPrefix(srv.URL, "http") + "/api/websocket"
	t.Cleanup(s.Close)
	return s
}

// NewHandler devuelve el handler WS, para montarlo junto a otros endpoints.
func NewHandler(t testing.TB, opts Options) http.Handler {
	s := &Server{
		opts:     opts,
		conns:    make(map[*clientConn]struct{}),
		subChans: make(map[string][]chan struct{}),
		states:   make(map[string]map[string]any),
		freezeCh: make(chan struct{}),
	}
	close(s.freezeCh)
	return s.handler(t)
}

func (s *Server) handler(t testing.TB) http.Handler {
	if s.opts.Token == "" {
		s.opts.Token = "test-token"
	}
	if s.opts.HAVersion == "" {
		s.opts.HAVersion = "2026.10.0"
	}
	if s.opts.States == nil {
		s.opts.States = []any{}
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.mu.Lock()
		if s.down {
			s.mu.Unlock()
			http.Error(w, "server down", http.StatusServiceUnavailable)
			return
		}
		s.mu.Unlock()

		c, err := websocket.Accept(w, r, nil)
		if err != nil {
			t.Logf("hatest: accept: %v", err)
			return
		}
		c.SetReadLimit(64 << 20)

		cc := &clientConn{
			c:    c,
			subs: make(map[int64]string),
		}

		s.mu.Lock()
		s.conns[cc] = struct{}{}
		s.mu.Unlock()

		defer func() {
			s.mu.Lock()
			delete(s.conns, cc)
			s.mu.Unlock()
			c.CloseNow()
		}()

		s.serve(r.Context(), cc)
	})
}

func (s *Server) serve(ctx context.Context, cc *clientConn) {
	c := cc.c
	o := s.opts

	switch o.Greeting {
	case GreetingClose:
		return
	case GreetingSilent:
		var dummy any
		_ = wsjson.Read(ctx, c, &dummy)
		return
	case GreetingBogus:
		_ = wsjson.Write(ctx, c, map[string]any{"type": "bogus"})
		var dummy any
		_ = wsjson.Read(ctx, c, &dummy)
		return
	}

	if err := wsjson.Write(ctx, c, map[string]any{"type": "auth_required", "ha_version": o.HAVersion}); err != nil {
		return
	}
	var auth struct {
		Type        string `json:"type"`
		AccessToken string `json:"access_token"`
	}
	if err := wsjson.Read(ctx, c, &auth); err != nil {
		return
	}
	if auth.Type != "auth" || auth.AccessToken != o.Token {
		_ = wsjson.Write(ctx, c, map[string]any{"type": "auth_invalid", "message": "Invalid access token or password"})
		return
	}
	if err := wsjson.Write(ctx, c, map[string]any{"type": "auth_ok", "ha_version": o.HAVersion}); err != nil {
		return
	}

	var pending []map[string]any // solo para ReverseBatch; un único goroutine
	calls := map[string]int{}    // llamadas por tipo, para ResultSeq
	for {
		s.mu.Lock()
		freezeCh := s.freezeCh
		isDown := s.down
		s.mu.Unlock()

		if isDown {
			return
		}

		select {
		case <-freezeCh:
		case <-ctx.Done():
			return
		}

		var cmd map[string]any
		if err := wsjson.Read(ctx, c, &cmd); err != nil {
			return
		}

		typ, _ := cmd["type"].(string)
		idVal, _ := cmd["id"].(float64)
		id := int64(idVal)

		s.mu.Lock()
		s.commands = append(s.commands, cmd)
		onCmd := s.onCommand
		s.mu.Unlock()

		if onCmd != nil {
			onCmd(cmd)
		}

		if o.DropOn != "" && typ == o.DropOn {
			return // CloseNow diferido: corte abrupto
		}

		// Manejo de subscripciones
		if typ == "subscribe_events" {
			evType, _ := cmd["event_type"].(string)
			cc.mu.Lock()
			cc.subs[id] = evType
			cc.mu.Unlock()

			s.mu.Lock()
			if waiters, ok := s.subChans[evType]; ok {
				for _, ch := range waiters {
					close(ch)
				}
				delete(s.subChans, evType)
			}
			if waiters, ok := s.subChans["*"]; ok {
				for _, ch := range waiters {
					close(ch)
				}
				delete(s.subChans, "*")
			}
			s.mu.Unlock()
		} else if typ == "unsubscribe_events" {
			subIDVal, _ := cmd["subscription"].(float64)
			subID := int64(subIDVal)
			cc.mu.Lock()
			delete(cc.subs, subID)
			cc.mu.Unlock()
		}

		resp := s.respond(cmd, o, calls)
		if o.ReverseBatch > 1 {
			pending = append(pending, resp)
			if len(pending) < o.ReverseBatch {
				continue
			}
			for i := len(pending) - 1; i >= 0; i-- {
				if err := wsjson.Write(ctx, c, pending[i]); err != nil {
					return
				}
			}
			pending = nil
			continue
		}
		if err := wsjson.Write(ctx, c, resp); err != nil {
			return
		}
	}
}

func (s *Server) respond(cmd map[string]any, o Options, calls map[string]int) map[string]any {
	id := cmd["id"]
	typ, _ := cmd["type"].(string)
	ok := func(result any) map[string]any {
		return map[string]any{"id": id, "type": "result", "success": true, "result": result}
	}
	if e, found := o.Errors[typ]; found {
		return map[string]any{"id": id, "type": "result", "success": false, "error": e}
	}
	if seq := o.ResultSeq[typ]; len(seq) > 0 {
		i := min(calls[typ], len(seq)-1)
		calls[typ]++
		return ok(seq[i])
	}
	if r, found := o.Results[typ]; found {
		return ok(r)
	}
	switch typ {
	case "ping":
		return map[string]any{"id": id, "type": "pong"}
	case "supported_features":
		return ok(map[string]any{"features": map[string]any{"coalesce_messages": 1}})
	case "coalesce_messages":
		return ok(nil)
	case "subscribe_events", "unsubscribe_events":
		return ok(nil)
	case "call_service":
		return ok(map[string]any{
			"context": map[string]any{
				"id":        "01J0000000000000000000001",
				"parent_id": nil,
				"user_id":   nil,
			},
		})
	case "get_states":
		s.mu.Lock()
		defer s.mu.Unlock()
		if len(s.states) > 0 {
			res := make([]map[string]any, 0, len(s.states))
			for _, st := range s.states {
				res = append(res, st)
			}
			return ok(res)
		}
		return ok(o.States)
	case "get_config":
		return ok(Config("RUNNING", o.HAVersion))
	case "config/entity_registry/list":
		return ok([]any{})
	case "config/device_registry/list":
		return ok([]any{})
	case "config/area_registry/list":
		return ok([]any{})
	case "config/entity_registry/get_entries":
		return ok(map[string]any{})
	case "homeassistant/expose_entity/list":
		return ok(map[string]any{"exposed_entities": map[string]any{}})
	}
	return map[string]any{"id": id, "type": "result", "success": false,
		"error": ErrorObj{Code: "unknown_command", Message: "Unknown command."}}
}

// Push emite un evento a todas las conexiones que se hayan suscrito al tipo de evento.
func (s *Server) Push(event map[string]any) {
	s.mu.Lock()
	conns := make([]*clientConn, 0, len(s.conns))
	for cc := range s.conns {
		conns = append(conns, cc)
	}
	s.mu.Unlock()

	evType, _ := event["event_type"].(string)

	for _, cc := range conns {
		cc.mu.Lock()
		for subID, subType := range cc.subs {
			if subType == "" || subType == evType {
				msg := map[string]any{
					"id":    subID,
					"type":  "event",
					"event": event,
				}
				_ = wsjson.Write(context.Background(), cc.c, msg)
			}
		}
		cc.mu.Unlock()
	}
}

// SetEntityState actualiza o inserta el estado de una entidad y emite state_changed.
func (s *Server) SetEntityState(newState map[string]any) {
	entityID, _ := newState["entity_id"].(string)
	s.mu.Lock()
	oldState := s.states[entityID]
	s.states[entityID] = newState
	s.mu.Unlock()

	s.Push(map[string]any{
		"event_type": "state_changed",
		"data": map[string]any{
			"entity_id": entityID,
			"old_state": oldState,
			"new_state": newState,
		},
		"time_fired": time.Now().UTC().Format(time.RFC3339Nano),
		"context": map[string]any{
			"id": "01J0000000000000000000000",
		},
	})
}

// RemoveEntity elimina una entidad y emite state_changed con new_state null.
func (s *Server) RemoveEntity(entityID string) {
	s.mu.Lock()
	oldState := s.states[entityID]
	delete(s.states, entityID)
	s.mu.Unlock()

	s.Push(map[string]any{
		"event_type": "state_changed",
		"data": map[string]any{
			"entity_id": entityID,
			"old_state": oldState,
			"new_state": nil,
		},
		"time_fired": time.Now().UTC().Format(time.RFC3339Nano),
		"context": map[string]any{
			"id": "01J0000000000000000000000",
		},
	})
}

// PushFrame envía bytes crudos directamente a todas las conexiones activas.
func (s *Server) PushFrame(raw []byte) {
	s.mu.Lock()
	conns := make([]*clientConn, 0, len(s.conns))
	for cc := range s.conns {
		conns = append(conns, cc)
	}
	s.mu.Unlock()

	for _, cc := range conns {
		_ = cc.c.Write(context.Background(), websocket.MessageText, raw)
	}
}

// WaitSubscribed espera a que un cliente se suscriba al tipo de evento.
func (s *Server) WaitSubscribed(t testing.TB, eventType string, timeout time.Duration) {
	t.Helper()
	s.mu.Lock()
	// Comprobar si ya está suscrito
	for cc := range s.conns {
		cc.mu.Lock()
		for _, subType := range cc.subs {
			if subType == eventType || subType == "" {
				cc.mu.Unlock()
				s.mu.Unlock()
				return
			}
		}
		cc.mu.Unlock()
	}

	ch := make(chan struct{})
	s.subChans[eventType] = append(s.subChans[eventType], ch)
	s.mu.Unlock()

	select {
	case <-ch:
	case <-time.After(timeout):
		t.Fatalf("hatest: timeout esperando suscripción a %s", eventType)
	}
}

// OnCommand registra una función que se ejecuta con cada comando recibido.
func (s *Server) OnCommand(fn func(cmd map[string]any)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.onCommand = fn
}

// Received devuelve una copia de los comandos recibidos hasta el momento.
func (s *Server) Received() []map[string]any {
	s.mu.Lock()
	defer s.mu.Unlock()
	res := make([]map[string]any, len(s.commands))
	copy(res, s.commands)
	return res
}

// DropAll corta abruptamente todas las conexiones abiertas.
func (s *Server) DropAll() {
	s.mu.Lock()
	conns := make([]*clientConn, 0, len(s.conns))
	for cc := range s.conns {
		conns = append(conns, cc)
	}
	s.mu.Unlock()

	for _, cc := range conns {
		cc.c.CloseNow()
	}
}

// SetDown define si el servidor simula caída de red.
func (s *Server) SetDown(down bool) {
	s.mu.Lock()
	s.down = down
	s.mu.Unlock()
	if down {
		s.DropAll()
	}
}

// Freeze detiene o reanuda la lectura/escritura del servidor sin cerrar conexiones.
func (s *Server) Freeze(frozen bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.frozen == frozen {
		return
	}
	s.frozen = frozen
	if frozen {
		s.freezeCh = make(chan struct{})
	} else {
		close(s.freezeCh)
	}
}

// Close cierra todas las conexiones abiertas y apaga el servidor HTTP de pruebas.
func (s *Server) Close() {
	s.DropAll()
	if s.srv != nil {
		s.srv.CloseClientConnections()
		s.srv.Close()
	}
}

// Config construye una respuesta de get_config con el estado del core de HA
// (NOT_RUNNING, STARTING, RUNNING, STOPPING...).
func Config(state, version string) map[string]any {
	return map[string]any{
		"state":     state,
		"version":   version,
		"time_zone": "America/Santiago",
		"unit_system": map[string]any{
			"length":      "km",
			"mass":        "g",
			"pressure":    "Pa",
			"temperature": "°C",
			"volume":      "L",
		},
	}
}

// State construye un estado con el formato de HA para fixtures.
func State(entityID, state string, attrs map[string]any) map[string]any {
	if attrs == nil {
		attrs = map[string]any{}
	}
	return map[string]any{
		"entity_id":    entityID,
		"state":        state,
		"attributes":   attrs,
		"last_changed": "2026-10-15T12:00:00.000000+00:00",
		"last_updated": "2026-10-15T12:00:01.500000+00:00",
		"context":      map[string]any{"id": "01J0000000000000000000000", "parent_id": nil, "user_id": nil},
	}
}

// MustJSON serializa v o hace fallar el test.
func MustJSON(t testing.TB, v any) json.RawMessage {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return b
}
