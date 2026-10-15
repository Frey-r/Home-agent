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
}

type result struct {
	raw json.RawMessage
	err error
}

// Conn es una conexión autenticada a la WebSocket API de HA. Es segura para
// uso concurrente.
type Conn struct {
	ws        *websocket.Conn
	haVersion string

	mu       sync.Mutex
	nextID   int64
	pending  map[int64]chan result
	closeErr error // != nil una vez cerrada; siempre envuelve ErrClosed

	closeOnce sync.Once
}

// Connect abre la conexión y realiza el handshake de autenticación.
func Connect(ctx context.Context, url, token string) (*Conn, error) {
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

	c := &Conn{ws: ws, haVersion: version, pending: map[int64]chan result{}}
	go c.readLoop()
	return c, nil
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

// Close cierra la conexión. Es idempotente.
func (c *Conn) Close() error {
	var err error
	c.closeOnce.Do(func() {
		c.fail(ErrClosed)
		err = c.ws.Close(websocket.StatusNormalClosure, "")
	})
	return err
}

func (c *Conn) readLoop() {
	for {
		var msg message
		if err := wsjson.Read(context.Background(), c.ws, &msg); err != nil {
			c.fail(fmt.Errorf("%w: %w", ErrClosed, err))
			return
		}
		if msg.Type != "result" {
			continue // eventos y pongs se manejan desde M1
		}
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
			ch <- r // buffer 1: nunca bloquea
		}
	}
}

// fail marca la conexión como cerrada y despierta a todos los comandos
// pendientes con err. Solo el primer llamado tiene efecto.
func (c *Conn) fail(err error) {
	c.mu.Lock()
	if c.closeErr != nil {
		c.mu.Unlock()
		return
	}
	c.closeErr = err
	pending := c.pending
	c.pending = nil
	c.mu.Unlock()

	for _, ch := range pending {
		ch <- result{err: err}
	}
}

func (c *Conn) dropPending(id int64) {
	c.mu.Lock()
	delete(c.pending, id)
	c.mu.Unlock()
}
