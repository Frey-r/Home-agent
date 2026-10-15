// Package hatest provee un servidor falso de la WebSocket API de Home
// Assistant para tests. No se usa en código de producción.
package hatest

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

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

// Server es un HA falso escuchando en httptest.
type Server struct {
	URL string // ws://.../api/websocket
	srv *httptest.Server
}

// New levanta un servidor falso; se cierra automáticamente al terminar el test.
func New(t testing.TB, opts Options) *Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.Handle("/api/websocket", NewHandler(t, opts))
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return &Server{URL: "ws" + strings.TrimPrefix(srv.URL, "http") + "/api/websocket", srv: srv}
}

// NewHandler devuelve el handler WS, para montarlo junto a otros endpoints.
func NewHandler(t testing.TB, opts Options) http.Handler {
	if opts.Token == "" {
		opts.Token = "test-token"
	}
	if opts.HAVersion == "" {
		opts.HAVersion = "2026.10.0"
	}
	if opts.States == nil {
		opts.States = []any{}
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, err := websocket.Accept(w, r, nil)
		if err != nil {
			t.Logf("hatest: accept: %v", err)
			return
		}
		c.SetReadLimit(64 << 20)
		serve(r.Context(), c, opts)
	})
}

func serve(ctx context.Context, c *websocket.Conn, o Options) {
	defer c.CloseNow()

	switch o.Greeting {
	case GreetingClose:
		return
	case GreetingSilent:
		<-ctx.Done()
		return
	case GreetingBogus:
		_ = wsjson.Write(ctx, c, map[string]any{"type": "bogus"})
		<-ctx.Done()
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
		var cmd map[string]any
		if err := wsjson.Read(ctx, c, &cmd); err != nil {
			return
		}
		typ, _ := cmd["type"].(string)
		if o.DropOn != "" && typ == o.DropOn {
			return // CloseNow diferido: corte abrupto
		}
		resp := respond(cmd, o, calls)
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

func respond(cmd map[string]any, o Options, calls map[string]int) map[string]any {
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
	case "get_states":
		return ok(o.States)
	case "get_config":
		// Como un HA real ya arrancado.
		return ok(Config("RUNNING", o.HAVersion))
	}
	return map[string]any{"id": id, "type": "result", "success": false,
		"error": ErrorObj{Code: "unknown_command", Message: "Unknown command."}}
}

// Config construye una respuesta de get_config con el estado del core de HA
// (NOT_RUNNING, STARTING, RUNNING, STOPPING...).
func Config(state, version string) map[string]any {
	return map[string]any{"state": state, "version": version, "time_zone": "America/Santiago"}
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
