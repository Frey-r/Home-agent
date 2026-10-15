// Package ha implementa el cliente de la WebSocket API de Home Assistant
// (ADR-0005).
package ha

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"sync"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
)

// readLimit cubre hogares grandes: get_states puede pesar varios MB y el
// límite por defecto de la librería es 32 KiB.
const readLimit = 64 << 20

var (
	// ErrAuthInvalid indica que HA rechazó el token.
	ErrAuthInvalid = errors.New("ha: autenticación inválida")
	// ErrClosed indica que la conexión está cerrada.
	ErrClosed = errors.New("ha: conexión cerrada")
)

// CommandError es el error devuelto por HA en un result con success=false.
type CommandError struct {
	Code    string
	Message string
}

func (e *CommandError) Error() string {
	return fmt.Sprintf("ha: comando rechazado (%s): %s", e.Code, e.Message)
}

// State es el estado de una entidad tal como lo entrega HA.
type State struct {
	EntityID    string         `json:"entity_id"`
	State       string         `json:"state"`
	Attributes  map[string]any `json:"attributes"`
	LastChanged time.Time      `json:"last_changed"`
	LastUpdated time.Time      `json:"last_updated"`
}

// message cubre todos los mensajes servidor→cliente que interpretamos.
type message struct {
	ID        int64           `json:"id"`
	Type      string          `json:"type"`
	Success   bool            `json:"success"`
	Result    json.RawMessage `json:"result"`
	Error     *CommandError   `json:"error"`
	HAVersion string          `json:"ha_version"`
	Message   string          `json:"message"`
	Event     json.RawMessage `json:"event"`
}

type result struct {
	raw json.RawMessage
	err error
}

type connOptions struct {
	coalescing bool
}

// Option permite configurar el comportamiento de la conexión.
type Option func(*connOptions)

// WithCoalescing habilita la negociación de mensajes coalescentes (arrays JSON).
func WithCoalescing() Option {
	return func(o *connOptions) {
		o.coalescing = true
	}
}

// Conn es una conexión autenticada a la WebSocket API de HA. Es segura para
// uso concurrente.
type Conn struct {
	ws        *websocket.Conn
	haVersion string

	mu       sync.Mutex
	nextID   int64
	pending  map[int64]chan result
	subs     map[int64]*Subscription
	closeErr error // != nil una vez cerrada; siempre envuelve ErrClosed
	doneCh   chan struct{}
	readDone chan struct{}

	closeOnce sync.Once
}

// Connect abre la conexión y realiza el handshake de autenticación.
func Connect(ctx context.Context, url, token string, opts ...Option) (*Conn, error) {
	var opt connOptions
	for _, o := range opts {
		o(&opt)
	}

	ws, _, err := websocket.Dial(ctx, url, nil)
	if err != nil {
		return nil, ctxErr(ctx, fmt.Errorf("ha: conectar a %s: %w", url, err))
	}
	ws.SetReadLimit(readLimit)

	version, err := handshake(ctx, ws, token)
	if err != nil {
		ws.CloseNow()
		return nil, err
	}

	c := &Conn{
		ws:        ws,
		haVersion: version,
		pending:   make(map[int64]chan result),
		subs:      make(map[int64]*Subscription),
		doneCh:    make(chan struct{}),
		readDone:  make(chan struct{}),
	}
	go c.readLoop()

	if opt.coalescing {
		if err := c.negotiateCoalescing(ctx); err != nil {
			c.Close()
			return nil, err
		}
	}

	return c, nil
}

func (c *Conn) negotiateCoalescing(ctx context.Context) error {
	raw, err := c.Command(ctx, map[string]any{"type": "supported_features"})
	if err != nil {
		return fmt.Errorf("ha: negociar coalescing: %w", err)
	}
	var sf struct {
		Features map[string]int `json:"features"`
	}
	if err := json.Unmarshal(raw, &sf); err != nil {
		return fmt.Errorf("ha: decodificar supported_features: %w", err)
	}
	if sf.Features["coalesce_messages"] > 0 {
		_, err = c.Command(ctx, map[string]any{
			"type": "coalesce_messages",
			"supported_features": map[string]int{
				"coalesce_messages": 1,
			},
		})
		if err != nil {
			return fmt.Errorf("ha: activar coalesce_messages: %w", err)
		}
	}
	return nil
}

func handshake(ctx context.Context, ws *websocket.Conn, token string) (string, error) {
	var msg message
	if err := wsjson.Read(ctx, ws, &msg); err != nil {
		return "", ctxErr(ctx, fmt.Errorf("ha: handshake: %w", err))
	}
	if msg.Type != "auth_required" {
		return "", fmt.Errorf("ha: handshake: se esperaba auth_required, llegó %q", msg.Type)
	}
	if err := wsjson.Write(ctx, ws, map[string]string{"type": "auth", "access_token": token}); err != nil {
		return "", ctxErr(ctx, fmt.Errorf("ha: handshake: %w", err))
	}
	msg = message{}
	if err := wsjson.Read(ctx, ws, &msg); err != nil {
		return "", ctxErr(ctx, fmt.Errorf("ha: handshake: %w", err))
	}
	switch msg.Type {
	case "auth_ok":
		return msg.HAVersion, nil
	case "auth_invalid":
		return "", fmt.Errorf("%w: %s", ErrAuthInvalid, msg.Message)
	default:
		return "", fmt.Errorf("ha: handshake: respuesta de auth inesperada %q", msg.Type)
	}
}

// ctxErr prioriza el error del contexto, para que el llamador pueda usar
// errors.Is(err, context.DeadlineExceeded).
func ctxErr(ctx context.Context, err error) error {
	if ctx.Err() != nil {
		return fmt.Errorf("%w (%w)", ctx.Err(), err)
	}
	return err
}

// HAVersion devuelve la versión de HA informada en el handshake.
func (c *Conn) HAVersion() string { return c.haVersion }

// Done devuelve un canal que se cierra cuando la conexión finaliza o se interrumpe.
func (c *Conn) Done() <-chan struct{} { return c.doneCh }

// Err devuelve el error de cierre si la conexión falló.
func (c *Conn) Err() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.closeErr
}

// Ping envía un mensaje de latido y espera la respuesta pong.
func (c *Conn) Ping(ctx context.Context) error {
	_, err := c.Command(ctx, map[string]any{"type": "ping"})
	return err
}

// Command envía un comando (se le asigna id) y devuelve el campo result.
func (c *Conn) Command(ctx context.Context, cmd map[string]any) (json.RawMessage, error) {
	if typ, _ := cmd["type"].(string); typ == "" {
		return nil, errors.New("ha: comando sin campo type")
	}
	ch := make(chan result, 1)

	c.mu.Lock()
	if c.closeErr != nil {
		err := c.closeErr
		c.mu.Unlock()
		return nil, err
	}
	c.nextID++
	id := c.nextID
	c.pending[id] = ch
	c.mu.Unlock()

	msg := maps.Clone(cmd)
	msg["id"] = id
	if err := wsjson.Write(ctx, c.ws, msg); err != nil {
		c.dropPending(id)
		return nil, ctxErr(ctx, fmt.Errorf("ha: enviar %v: %w", cmd["type"], err))
	}

	select {
	case r := <-ch:
		return r.raw, r.err
	case <-ctx.Done():
		c.dropPending(id)
		return nil, ctx.Err()
	}
}

// SubscribeEvents registra una suscripción a eventos en HA y devuelve un *Subscription.
func (c *Conn) SubscribeEvents(ctx context.Context, bufSize int, eventTypes ...string) (*Subscription, error) {
	if bufSize <= 0 {
		bufSize = 10
	}
	sub := &Subscription{
		conn:  c,
		ch:    make(chan Event, bufSize),
		errCh: make(chan error, 1),
	}

	cmd := map[string]any{"type": "subscribe_events"}
	if len(eventTypes) > 0 && eventTypes[0] != "" {
		cmd["event_type"] = eventTypes[0]
	}

	ch := make(chan result, 1)

	c.mu.Lock()
	if c.closeErr != nil {
		err := c.closeErr
		c.mu.Unlock()
		return nil, err
	}
	c.nextID++
	id := c.nextID
	sub.id = id
	c.pending[id] = ch
	c.subs[id] = sub
	c.mu.Unlock()

	msg := maps.Clone(cmd)
	msg["id"] = id
	if err := wsjson.Write(ctx, c.ws, msg); err != nil {
		c.mu.Lock()
		delete(c.pending, id)
		delete(c.subs, id)
		c.mu.Unlock()
		return nil, ctxErr(ctx, fmt.Errorf("ha: suscribir eventos: %w", err))
	}

	select {
	case r := <-ch:
		if r.err != nil {
			c.removeSubscription(id)
			return nil, r.err
		}
		return sub, nil
	case <-ctx.Done():
		c.mu.Lock()
		delete(c.pending, id)
		delete(c.subs, id)
		c.mu.Unlock()
		return nil, ctx.Err()
	}
}

func (c *Conn) removeSubscription(id int64) {
	c.mu.Lock()
	delete(c.subs, id)
	c.mu.Unlock()
}

// GetStates devuelve el estado de todas las entidades.
func (c *Conn) GetStates(ctx context.Context) ([]State, error) {
	raw, err := c.Command(ctx, map[string]any{"type": "get_states"})
	if err != nil {
		return nil, err
	}
	var states []State
	if err := json.Unmarshal(raw, &states); err != nil {
		return nil, fmt.Errorf("ha: decodificar get_states: %w", err)
	}
	return states, nil
}

// WaitStarted espera a que el core de HA termine de arrancar (estado RUNNING
// en get_config), consultando cada interval.
func (c *Conn) WaitStarted(ctx context.Context, interval time.Duration) error {
	for {
		raw, err := c.Command(ctx, map[string]any{"type": "get_config"})
		if err != nil {
			return err
		}
		var cfg struct {
			State string `json:"state"`
		}
		if err := json.Unmarshal(raw, &cfg); err != nil {
			return fmt.Errorf("ha: decodificar get_config: %w", err)
		}
		if cfg.State == "RUNNING" {
			return nil
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("ha: el core sigue en estado %q: %w", cfg.State, ctx.Err())
		case <-time.After(interval):
		}
	}
}

// Close cierra la conexión. Es idempotente.
func (c *Conn) Close() error {
	var err error
	c.closeOnce.Do(func() {
		c.fail(ErrClosed)
		err = c.ws.Close(websocket.StatusNormalClosure, "")
		<-c.readDone
	})
	return err
}

func (c *Conn) readLoop() {
	defer close(c.readDone)
	defer c.fail(ErrClosed)

	for {
		typ, data, err := c.ws.Read(context.Background())
		if err != nil {
			c.fail(fmt.Errorf("%w: %w", ErrClosed, err))
			return
		}
		if typ != websocket.MessageText {
			continue
		}

		trimmed := data
		for len(trimmed) > 0 && (trimmed[0] == ' ' || trimmed[0] == '\t' || trimmed[0] == '\r' || trimmed[0] == '\n') {
			trimmed = trimmed[1:]
		}
		if len(trimmed) == 0 {
			continue
		}

		if trimmed[0] == '[' {
			var msgs []message
			if err := json.Unmarshal(trimmed, &msgs); err == nil {
				for _, m := range msgs {
					c.handleMessage(m)
				}
			}
		} else {
			var msg message
			if err := json.Unmarshal(trimmed, &msg); err == nil {
				c.handleMessage(msg)
			}
		}
	}
}

func (c *Conn) handleMessage(msg message) {
	if msg.Type == "pong" {
		c.mu.Lock()
		ch, ok := c.pending[msg.ID]
		delete(c.pending, msg.ID)
		c.mu.Unlock()
		if ok {
			ch <- result{}
		}
		return
	}

	if msg.Type == "result" {
		r := result{raw: msg.Result}
		if !msg.Success {
			cerr := msg.Error
			if cerr == nil {
				cerr = &CommandError{Code: "unknown", Message: "result sin éxito y sin detalle"}
			}
			r = result{err: cerr}
		}
		c.mu.Lock()
		ch, ok := c.pending[msg.ID]
		delete(c.pending, msg.ID)
		c.mu.Unlock()
		if ok {
			ch <- r
		}
		return
	}

	if msg.Type == "event" {
		var ev Event
		if len(msg.Event) > 0 {
			_ = json.Unmarshal(msg.Event, &ev)
		}
		ev.EventID = msg.ID

		c.mu.Lock()
		sub, ok := c.subs[msg.ID]
		c.mu.Unlock()

		if ok {
			select {
			case sub.ch <- ev:
			default:
				sub.fail(ErrSlowConsumer)
			}
		}
		return
	}
}

// fail marca la conexión como cerrada y despierta a todos los comandos
// pendientes y suscriptores con err. Solo el primer llamado tiene efecto.
func (c *Conn) fail(err error) {
	c.mu.Lock()
	if c.closeErr != nil {
		c.mu.Unlock()
		return
	}
	c.closeErr = err
	pending := c.pending
	c.pending = nil
	subs := c.subs
	c.subs = nil

	select {
	case <-c.doneCh:
	default:
		close(c.doneCh)
	}
	c.mu.Unlock()

	for _, ch := range pending {
		ch <- result{err: err}
	}
	for _, sub := range subs {
		sub.fail(err)
	}
}

func (c *Conn) dropPending(id int64) {
	c.mu.Lock()
	delete(c.pending, id)
	c.mu.Unlock()
}
