package entities

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/Frey-r/Home-agent/contracts"
	"github.com/Frey-r/Home-agent/edge/internal/ha"
)

// Syncer conecta un cliente WebSocket de Home Assistant con un Directory en memoria,
// garantizando consistencia eventual estricta, debouncing de ráfagas y ordenamiento de eventos.
type Syncer struct {
	dir              *Directory
	mu               sync.RWMutex
	connected        bool
	syncedCh         chan struct{}
	regs             *ha.Registries
	exposed          map[string]bool
	lastUpdated      map[string]time.Time
	RegistryDebounce time.Duration
	ExposureRefresh  time.Duration

	// Callbacks para observabilidad y logging reactivo
	OnEntityUpdated func(ent *contracts.Entity)
	OnEntityRemoved func(id string)
}

// NewSyncer crea un sincronizador asociado al directorio dado.
func NewSyncer(dir *Directory) *Syncer {
	return &Syncer{
		dir:              dir,
		syncedCh:         make(chan struct{}),
		lastUpdated:      make(map[string]time.Time),
		RegistryDebounce: 50 * time.Millisecond,
		ExposureRefresh:  30 * time.Minute,
	}
}

// Connected indica si el sincronizador está activamente conectado y sincronizado con HA.
func (s *Syncer) Connected() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.connected
}

func (s *Syncer) setConnected(c bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.connected = c
}

// WaitSynced espera a que el snapshot inicial haya sido completamente aplicado en el directorio.
func (s *Syncer) WaitSynced(ctx context.Context, timeout time.Duration) error {
	select {
	case <-s.syncedCh:
		return nil
	case <-time.After(timeout):
		return errors.New("timeout esperando sincronización inicial")
	case <-ctx.Done():
		return ctx.Err()
	}
}

// Run ejecuta el ciclo de sincronización como sesión de conexión de ha.Supervisor.
func (s *Syncer) Run(ctx context.Context, conn *ha.Conn) error {
	defer s.setConnected(false)

	// 1. Suscribirse a eventos ANTES del snapshot para no perder ningún cambio intermedio
	subState, err := conn.SubscribeEvents(ctx, 200, "state_changed")
	if err != nil {
		return fmt.Errorf("suscribir state_changed: %w", err)
	}
	defer subState.Close()

	subRegEnt, _ := conn.SubscribeEvents(ctx, 50, "entity_registry_updated")
	if subRegEnt != nil {
		defer subRegEnt.Close()
	}
	subRegDev, _ := conn.SubscribeEvents(ctx, 50, "device_registry_updated")
	if subRegDev != nil {
		defer subRegDev.Close()
	}
	subRegArea, _ := conn.SubscribeEvents(ctx, 50, "area_registry_updated")
	if subRegArea != nil {
		defer subRegArea.Close()
	}

	// 2. Snapshot inicial (Áreas, Dispositivos, Entidades, Expuestos, Estados)
	regs, err := conn.Registries(ctx)
	if err != nil {
		return fmt.Errorf("snapshot registries: %w", err)
	}

	exposed, err := conn.Exposed(ctx, "conversation")
	if err != nil {
		return fmt.Errorf("snapshot exposed: %w", err)
	}

	states, err := conn.GetStates(ctx)
	if err != nil {
		return fmt.Errorf("snapshot states: %w", err)
	}

	s.mu.Lock()
	s.regs = regs
	s.exposed = exposed
	s.lastUpdated = make(map[string]time.Time)

	var built []*contracts.Entity
	for _, raw := range states {
		ent, bErr := Build(raw, regs, exposed)
		if bErr == nil && ent != nil {
			built = append(built, ent)
			if !raw.LastUpdated.IsZero() {
				s.lastUpdated[raw.EntityID] = raw.LastUpdated
			}
		}
	}
	s.dir.Apply(built)
	s.connected = true
	select {
	case <-s.syncedCh:
	default:
		close(s.syncedCh)
	}
	s.mu.Unlock()

	// 3. Bucle de procesamiento de eventos en tiempo real con debouncing de registros
	debounceTimer := time.NewTimer(time.Hour)
	debounceTimer.Stop()
	var pendingRegistryRecheck bool

	var regEntC <-chan ha.Event
	if subRegEnt != nil {
		regEntC = subRegEnt.C()
	}
	var regDevC <-chan ha.Event
	if subRegDev != nil {
		regDevC = subRegDev.C()
	}
	var regAreaC <-chan ha.Event
	if subRegArea != nil {
		regAreaC = subRegArea.C()
	}

	refreshTicker := time.NewTicker(s.ExposureRefresh)
	defer refreshTicker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-conn.Done():
			return conn.Err()
		case subErr := <-subState.Err():
			return subErr

		case ev, ok := <-subState.C():
			if !ok {
				return errors.New("state_changed subscription closed")
			}
			s.handleStateChanged(ev)

		case _, ok := <-regEntC:
			if ok {
				s.triggerRegistryDebounce(debounceTimer, &pendingRegistryRecheck)
			}
		case _, ok := <-regDevC:
			if ok {
				s.triggerRegistryDebounce(debounceTimer, &pendingRegistryRecheck)
			}
		case _, ok := <-regAreaC:
			if ok {
				s.triggerRegistryDebounce(debounceTimer, &pendingRegistryRecheck)
			}

		case <-debounceTimer.C:
			if pendingRegistryRecheck {
				pendingRegistryRecheck = false
				s.refreshRegistries(ctx, conn)
			}

		case <-refreshTicker.C:
			s.refreshExposure(ctx, conn)
		}
	}
}

func (s *Syncer) triggerRegistryDebounce(timer *time.Timer, pending *bool) {
	*pending = true
	timer.Stop()
	select {
	case <-timer.C:
	default:
	}
	timer.Reset(s.RegistryDebounce)
}

func (s *Syncer) refreshRegistries(ctx context.Context, conn *ha.Conn) {
	newRegs, err := conn.Registries(ctx)
	if err != nil {
		return
	}
	newExposed, err := conn.Exposed(ctx, "conversation")
	if err != nil {
		return
	}

	s.mu.Lock()
	s.regs = newRegs
	s.exposed = newExposed
	s.mu.Unlock()
}

func (s *Syncer) refreshExposure(ctx context.Context, conn *ha.Conn) {
	newExposed, err := conn.Exposed(ctx, "conversation")
	if err != nil {
		return
	}
	s.mu.Lock()
	s.exposed = newExposed
	s.mu.Unlock()
}

type stateChangedPayload struct {
	EntityID string          `json:"entity_id"`
	NewState json.RawMessage `json:"new_state"`
}

type rawHAState struct {
	EntityID    string         `json:"entity_id"`
	State       string         `json:"state"`
	Attributes  map[string]any `json:"attributes"`
	LastChanged time.Time      `json:"last_changed"`
	LastUpdated time.Time      `json:"last_updated"`
}

func (s *Syncer) handleStateChanged(ev ha.Event) {
	if len(ev.Data) == 0 {
		return
	}

	var payload stateChangedPayload
	if err := json.Unmarshal(ev.Data, &payload); err != nil {
		return
	}

	entityID := payload.EntityID
	if entityID == "" {
		return
	}

	// new_state == null o vacío significa eliminación de la entidad
	if len(payload.NewState) == 0 || string(payload.NewState) == "null" {
		s.mu.Lock()
		delete(s.lastUpdated, entityID)
		s.mu.Unlock()
		s.dir.Remove("ha:" + entityID)
		if s.OnEntityRemoved != nil {
			s.OnEntityRemoved("ha:" + entityID)
		}
		return
	}

	var rawState rawHAState
	if err := json.Unmarshal(payload.NewState, &rawState); err != nil {
		return
	}

	s.mu.Lock()
	// Validación de orden temporal (descarte de eventos más viejos)
	if !rawState.LastUpdated.IsZero() {
		if prev, ok := s.lastUpdated[entityID]; ok && rawState.LastUpdated.Before(prev) {
			s.mu.Unlock()
			return // Evento obsoleto
		}
		s.lastUpdated[entityID] = rawState.LastUpdated
	}

	regs := s.regs
	exposed := s.exposed
	s.mu.Unlock()

	haSt := ha.State{
		EntityID:    rawState.EntityID,
		State:       rawState.State,
		Attributes:  rawState.Attributes,
		LastChanged: rawState.LastChanged,
		LastUpdated: rawState.LastUpdated,
	}

	ent, err := Build(haSt, regs, exposed)
	if err == nil && ent != nil {
		s.dir.Update(ent)
		if s.OnEntityUpdated != nil {
			s.OnEntityUpdated(ent)
		}
	}
}
