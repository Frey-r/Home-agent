package ha

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"domotica/edge/internal/ha/hatest"
)

const testTimeout = 5 * time.Second

func connect(t *testing.T, opts hatest.Options) *Conn {
	t.Helper()
	srv := hatest.New(t, opts)
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()
	c, err := Connect(ctx, srv.URL, "test-token")
	if err != nil {
		t.Fatalf("Connect() error: %v", err)
	}
	t.Cleanup(func() { c.Close() })
	return c
}

func TestConnect(t *testing.T) {
	tests := []struct {
		name        string
		opts        hatest.Options
		token       string
		timeout     time.Duration
		wantErr     bool
		wantErrIs   error
		wantVersion string
	}{
		{name: "auth ok", opts: hatest.Options{HAVersion: "2026.10.1"}, token: "test-token", wantVersion: "2026.10.1"},
		{name: "auth invalid", token: "otro", wantErr: true, wantErrIs: ErrAuthInvalid},
		{name: "cierra antes del handshake", opts: hatest.Options{Greeting: hatest.GreetingClose}, token: "test-token", wantErr: true},
		{name: "mensaje inesperado", opts: hatest.Options{Greeting: hatest.GreetingBogus}, token: "test-token", wantErr: true},
		{name: "silencio y contexto vence", opts: hatest.Options{Greeting: hatest.GreetingSilent}, token: "test-token",
			timeout: 200 * time.Millisecond, wantErr: true, wantErrIs: context.DeadlineExceeded},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := hatest.New(t, tt.opts)
			timeout := tt.timeout
			if timeout == 0 {
				timeout = testTimeout
			}
			ctx, cancel := context.WithTimeout(context.Background(), timeout)
			defer cancel()

			start := time.Now()
			c, err := Connect(ctx, srv.URL, tt.token)
			if tt.wantErr {
				if err == nil {
					c.Close()
					t.Fatal("Connect() error = nil, se esperaba error")
				}
				if tt.wantErrIs != nil && !errors.Is(err, tt.wantErrIs) {
					t.Fatalf("Connect() error = %v, want errors.Is %v", err, tt.wantErrIs)
				}
				if elapsed := time.Since(start); elapsed > timeout+time.Second {
					t.Fatalf("Connect() tardó %v; no debe colgarse", elapsed)
				}
				return
			}
			if err != nil {
				t.Fatalf("Connect() error: %v", err)
			}
			defer c.Close()
			if got := c.HAVersion(); got != tt.wantVersion {
				t.Fatalf("HAVersion() = %q, want %q", got, tt.wantVersion)
			}
		})
	}
}

func TestGetStates_Parses(t *testing.T) {
	big := make([]any, 2000)
	for i := range big {
		big[i] = hatest.State(fmt.Sprintf("sensor.s%04d", i), "21.5",
			map[string]any{"unit_of_measurement": "°C", "friendly_name": fmt.Sprintf("Sensor %d con un nombre largo para inflar el payload", i)})
	}

	tests := []struct {
		name   string
		states []any
		check  func(t *testing.T, got []State)
	}{
		{
			name: "campos básicos",
			states: []any{
				hatest.State("light.living", "on", map[string]any{"brightness": 153.0, "friendly_name": "Lámpara living"}),
				hatest.State("lock.front_door", "locked", nil),
			},
			check: func(t *testing.T, got []State) {
				if len(got) != 2 {
					t.Fatalf("len = %d, want 2", len(got))
				}
				l := got[0]
				if l.EntityID != "light.living" || l.State != "on" {
					t.Fatalf("estado[0] = %+v", l)
				}
				if l.Attributes["friendly_name"] != "Lámpara living" || l.Attributes["brightness"] != 153.0 {
					t.Fatalf("atributos = %v", l.Attributes)
				}
				wantChanged := time.Date(2026, 10, 15, 12, 0, 0, 0, time.UTC)
				if !l.LastChanged.Equal(wantChanged) {
					t.Fatalf("LastChanged = %v, want %v", l.LastChanged, wantChanged)
				}
				wantUpdated := time.Date(2026, 10, 15, 12, 0, 1, 500_000_000, time.UTC)
				if !l.LastUpdated.Equal(wantUpdated) {
					t.Fatalf("LastUpdated = %v, want %v", l.LastUpdated, wantUpdated)
				}
			},
		},
		{
			name:   "hogar grande supera el límite de lectura por defecto",
			states: big,
			check: func(t *testing.T, got []State) {
				if len(got) != len(big) {
					t.Fatalf("len = %d, want %d", len(got), len(big))
				}
			},
		},
		{
			name:   "sin entidades",
			states: []any{},
			check: func(t *testing.T, got []State) {
				if len(got) != 0 {
					t.Fatalf("len = %d, want 0", len(got))
				}
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := connect(t, hatest.Options{States: tt.states})
			ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
			defer cancel()
			got, err := c.GetStates(ctx)
			if err != nil {
				t.Fatalf("GetStates() error: %v", err)
			}
			tt.check(t, got)
		})
	}
}

// Regresión: tras el onboarding HA responde antes de cargar integraciones;
// sincronizar en ese momento entrega una casa a medias (CI de M0).
func TestWaitStarted(t *testing.T) {
	cfg := func(state string) any { return hatest.Config(state, "2026.10.1") }

	tests := []struct {
		name      string
		opts      hatest.Options
		timeout   time.Duration
		wantErrIs error
		wantCmd   bool // se espera *CommandError
	}{
		{name: "ya RUNNING", opts: hatest.Options{}},
		{name: "STARTING y luego RUNNING", opts: hatest.Options{ResultSeq: map[string][]any{
			"get_config": {cfg("NOT_RUNNING"), cfg("STARTING"), cfg("STARTING"), cfg("RUNNING")},
		}}},
		{name: "nunca termina de arrancar", timeout: 200 * time.Millisecond, wantErrIs: context.DeadlineExceeded,
			opts: hatest.Options{ResultSeq: map[string][]any{"get_config": {cfg("STARTING")}}}},
		{name: "get_config rechazado", wantCmd: true,
			opts: hatest.Options{Errors: map[string]hatest.ErrorObj{"get_config": {Code: "unauthorized", Message: "Unauthorized"}}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := connect(t, tt.opts)
			timeout := tt.timeout
			if timeout == 0 {
				timeout = testTimeout
			}
			ctx, cancel := context.WithTimeout(context.Background(), timeout)
			defer cancel()

			err := c.WaitStarted(ctx, 10*time.Millisecond)
			switch {
			case tt.wantErrIs != nil:
				if !errors.Is(err, tt.wantErrIs) {
					t.Fatalf("WaitStarted() error = %v, want errors.Is %v", err, tt.wantErrIs)
				}
			case tt.wantCmd:
				var cerr *CommandError
				if !errors.As(err, &cerr) {
					t.Fatalf("WaitStarted() error = %v, want *CommandError", err)
				}
			case err != nil:
				t.Fatalf("WaitStarted() error: %v", err)
			}
		})
	}
}

func TestCommand_ErrorResult(t *testing.T) {
	c := connect(t, hatest.Options{Errors: map[string]hatest.ErrorObj{
		"get_states": {Code: "unauthorized", Message: "Unauthorized"},
	}})
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	_, err := c.GetStates(ctx)
	var cerr *CommandError
	if !errors.As(err, &cerr) {
		t.Fatalf("error = %v, want *CommandError", err)
	}
	if cerr.Code != "unauthorized" || cerr.Message != "Unauthorized" {
		t.Fatalf("CommandError = %+v", cerr)
	}
}

func TestCommand_GenericResult(t *testing.T) {
	c := connect(t, hatest.Options{Results: map[string]any{"get_config": map[string]any{"time_zone": "America/Santiago"}}})
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	raw, err := c.Command(ctx, map[string]any{"type": "get_config"})
	if err != nil {
		t.Fatalf("Command() error: %v", err)
	}
	var cfg struct {
		TimeZone string `json:"time_zone"`
	}
	if err := json.Unmarshal(raw, &cfg); err != nil {
		t.Fatal(err)
	}
	if cfg.TimeZone != "America/Santiago" {
		t.Fatalf("time_zone = %q", cfg.TimeZone)
	}
}

func TestCommand_InvalidInput(t *testing.T) {
	c := connect(t, hatest.Options{})
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	tests := []struct {
		name string
		cmd  map[string]any
	}{
		{name: "comando nil", cmd: nil},
		{name: "sin type", cmd: map[string]any{"entity_id": "light.x"}},
		{name: "type vacío", cmd: map[string]any{"type": ""}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := c.Command(ctx, tt.cmd); err == nil {
				t.Fatal("Command() error = nil, se esperaba error de validación")
			}
		})
	}
	// La conexión sigue sana tras rechazar entradas inválidas.
	if _, err := c.GetStates(ctx); err != nil {
		t.Fatalf("GetStates() tras validación error: %v", err)
	}
}

func TestConcurrentRequests_CorrelateByID(t *testing.T) {
	const n = 8
	results := map[string]any{}
	for i := range n {
		results[fmt.Sprintf("echo_%d", i)] = i
	}
	c := connect(t, hatest.Options{ReverseBatch: n, Results: results})
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	var wg sync.WaitGroup
	errs := make(chan error, n)
	for i := range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			raw, err := c.Command(ctx, map[string]any{"type": fmt.Sprintf("echo_%d", i)})
			if err != nil {
				errs <- err
				return
			}
			var got int
			if err := json.Unmarshal(raw, &got); err != nil {
				errs <- err
				return
			}
			if got != i {
				errs <- fmt.Errorf("comando echo_%d recibió respuesta %d", i, got)
			}
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Error(err)
	}
}

func TestServerDropsMidRequest_ReturnsError(t *testing.T) {
	c := connect(t, hatest.Options{DropOn: "get_states"})
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	start := time.Now()
	if _, err := c.GetStates(ctx); err == nil {
		t.Fatal("GetStates() error = nil, se esperaba error por corte")
	}
	if ctx.Err() != nil || time.Since(start) > 2*time.Second {
		t.Fatalf("GetStates() no debe esperar al timeout del contexto (tardó %v)", time.Since(start))
	}

	if _, err := c.Command(ctx, map[string]any{"type": "get_config"}); !errors.Is(err, ErrClosed) {
		t.Fatalf("tras el corte, Command() error = %v, want ErrClosed", err)
	}
}

func TestClose_Idempotent(t *testing.T) {
	c := connect(t, hatest.Options{})
	if err := c.Close(); err != nil {
		t.Fatalf("primer Close() error: %v", err)
	}
	if err := c.Close(); err != nil {
		t.Fatalf("segundo Close() error: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()
	if _, err := c.GetStates(ctx); !errors.Is(err, ErrClosed) {
		t.Fatalf("GetStates() tras Close error = %v, want ErrClosed", err)
	}
}
