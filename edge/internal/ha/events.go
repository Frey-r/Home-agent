package ha

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"
)

// ErrSlowConsumer indica que el suscriptor no leyó a tiempo y su canal se desbordó.
var ErrSlowConsumer = errors.New("ha: consumidor lento descartó la suscripción")

// EventContext contiene la información de trazabilidad de un evento de HA.
type EventContext struct {
	ID       string  `json:"id"`
	ParentID *string `json:"parent_id,omitempty"`
	UserID   *string `json:"user_id,omitempty"`
}

// Event representa un evento emitido por la WebSocket API de HA.
type Event struct {
	EventID   int64           `json:"event_id,omitempty"`
	Type      string          `json:"event_type"`
	Data      json.RawMessage `json:"data"`
	TimeFired time.Time       `json:"time_fired"`
	Context   EventContext    `json:"context"`
}

// StateChangedEvent representa el payload de un evento state_changed.
type StateChangedEvent struct {
	EntityID string `json:"entity_id"`
	OldState *State `json:"old_state"`
	NewState *State `json:"new_state"`
}

// DecodeStateChanged extrae los datos de un evento state_changed.
func DecodeStateChanged(ev Event) (*StateChangedEvent, error) {
	var sc StateChangedEvent
	if err := json.Unmarshal(ev.Data, &sc); err != nil {
		return nil, fmt.Errorf("ha: decodificar state_changed: %w", err)
	}
	return &sc, nil
}

// RegistryUpdatedEvent representa una actualización en un registro de HA.
type RegistryUpdatedEvent struct {
	Action   string `json:"action"` // "create", "update", "remove"
	EntityID string `json:"entity_id,omitempty"`
	DeviceID string `json:"device_id,omitempty"`
	AreaID   string `json:"area_id,omitempty"`
}

// DecodeRegistryUpdated extrae los datos de un evento de registro.
func DecodeRegistryUpdated(ev Event) (*RegistryUpdatedEvent, error) {
	var ru RegistryUpdatedEvent
	if err := json.Unmarshal(ev.Data, &ru); err != nil {
		return nil, fmt.Errorf("ha: decodificar registry_updated: %w", err)
	}
	return &ru, nil
}

// Subscription representa una suscripción activa a eventos de HA.
type Subscription struct {
	id        int64
	conn      *Conn
	ch        chan Event
	errCh     chan error
	closeOnce sync.Once
}

// C devuelve el canal de lectura de eventos.
func (s *Subscription) C() <-chan Event { return s.ch }

// Err devuelve el canal que notifica errores fatales de la suscripción (ej. ErrSlowConsumer o desconexión).
func (s *Subscription) Err() <-chan error { return s.errCh }

// Close finaliza la suscripción y envía unsubscribe_events a HA.
func (s *Subscription) Close() error {
	var err error
	s.closeOnce.Do(func() {
		s.conn.removeSubscription(s.id)
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_, err = s.conn.Command(ctx, map[string]any{
			"type":         "unsubscribe_events",
			"subscription": s.id,
		})
	})
	return err
}

func (s *Subscription) fail(err error) {
	s.closeOnce.Do(func() {
		s.conn.removeSubscription(s.id)
		select {
		case s.errCh <- err:
		default:
		}
	})
}
