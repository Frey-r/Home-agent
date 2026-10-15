// Package ha implementa el cliente de la WebSocket API de Home Assistant
// (ADR-0005).
package ha

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

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

// Conn es una conexión autenticada a la WebSocket API de HA.
type Conn struct{}

// Connect abre la conexión y realiza el handshake de autenticación.
func Connect(ctx context.Context, url, token string) (*Conn, error) {
	return nil, errors.New("ha: no implementado")
}

// HAVersion devuelve la versión de HA informada en el handshake.
func (c *Conn) HAVersion() string { return "" }

// Command envía un comando (se le asigna id) y devuelve el campo result.
func (c *Conn) Command(ctx context.Context, cmd map[string]any) (json.RawMessage, error) {
	return nil, errors.New("ha: no implementado")
}

// GetStates devuelve el estado de todas las entidades.
func (c *Conn) GetStates(ctx context.Context) ([]State, error) {
	return nil, errors.New("ha: no implementado")
}

// Close cierra la conexión. Es idempotente.
func (c *Conn) Close() error {
	return errors.New("ha: no implementado")
}
