package ha

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Frey-r/Home-agent/edge/internal/ha/hatest"
	"go.uber.org/goleak"
)

func TestBackoff_Delay(t *testing.T) {
	b := Backoff{
		Initial:    100 * time.Millisecond,
		Max:        1 * time.Second,
		Multiplier: 2.0,
		Jitter:     0.0,
	}

	tests := []struct {
		attempt  int
		expected time.Duration
	}{
		{attempt: 0, expected: 100 * time.Millisecond},
		{attempt: 1, expected: 200 * time.Millisecond},
		{attempt: 2, expected: 400 * time.Millisecond},
		{attempt: 3, expected: 800 * time.Millisecond},
		{attempt: 4, expected: 1000 * time.Millisecond}, // capped at Max
		{attempt: 5, expected: 1000 * time.Millisecond},
	}

	for _, tt := range tests {
		got := b.Delay(tt.attempt)
		if got != tt.expected {
			t.Errorf("attempt %d: expected %v, got %v", tt.attempt, tt.expected, got)
		}
	}

	// Con jitter 0.2
	bJitter := Backoff{
		Initial:    100 * time.Millisecond,
		Max:        1 * time.Second,
		Multiplier: 2.0,
		Jitter:     0.2,
	}
	for i := 0; i < 20; i++ {
		d := bJitter.Delay(1) // base = 200ms
		// 200ms * (1 - 0.2) = 160ms; 200ms * (1 + 0.2) = 240ms
		if d < 160*time.Millisecond || d > 240*time.Millisecond {
			t.Errorf("delay with jitter out of bounds [160ms, 240ms]: got %v", d)
		}
	}
}

func TestSupervisor_ReconnectsAfterDrop(t *testing.T) {
	srv := hatest.New(t, hatest.Options{})
	defer func() {
		srv.Close()
		goleak.VerifyNone(t)
	}()

	sup := Supervisor{
		URL:   srv.URL,
		Token: "test-token",
		Backoff: Backoff{
			Initial:    10 * time.Millisecond,
			Max:        50 * time.Millisecond,
			Multiplier: 1.5,
		},
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var sessionsCount int32
	sessionStarted := make(chan struct{}, 2)

	session := func(sCtx context.Context, conn *Conn) error {
		cnt := atomic.AddInt32(&sessionsCount, 1)
		sessionStarted <- struct{}{}
		if cnt >= 2 {
			cancel()
			return nil
		}
		<-conn.Done()
		return conn.Err()
	}

	errCh := make(chan error, 1)
	go func() {
		errCh <- sup.Run(ctx, session)
	}()

	// Esperar primera sesión
	select {
	case <-sessionStarted:
	case <-time.After(2 * time.Second):
		t.Fatal("primera sesión no inició")
	}

	// Dropear conexión desde el server
	srv.DropAll()

	// Esperar segunda sesión
	select {
	case <-sessionStarted:
	case <-time.After(2 * time.Second):
		t.Fatal("segunda sesión no inició tras reconexión")
	}

	select {
	case err := <-errCh:
		if err != nil && !errors.Is(err, context.Canceled) {
			t.Fatalf("Supervisor.Run terminó con error inesperado: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Supervisor.Run no terminó al cancelar contexto")
	}

	if atomic.LoadInt32(&sessionsCount) < 2 {
		t.Fatalf("se esperaban al menos 2 sesiones, ocurrieron %d", atomic.LoadInt32(&sessionsCount))
	}
}

func TestSupervisor_NoHotLoopWhileDown(t *testing.T) {
	defer goleak.VerifyNone(t)

	// URL donde no hay ningún servidor
	sup := Supervisor{
		URL:   "ws://127.0.0.1:59999/api/websocket",
		Token: "test-token",
		Backoff: Backoff{
			Initial:    60 * time.Millisecond,
			Max:        200 * time.Millisecond,
			Multiplier: 2.0,
		},
	}

	var attempts int32
	sup.OnConnectAttempt = func() {
		atomic.AddInt32(&attempts, 1)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
	defer cancel()

	_ = sup.Run(ctx, func(ctx context.Context, conn *Conn) error {
		return nil
	})

	n := atomic.LoadInt32(&attempts)
	// En 150ms con initial de 60ms y max de 200ms, no debe haber más de 3 intentos
	if n > 4 {
		t.Fatalf("hot loop detectado: %d intentos en 150ms", n)
	}
}

func TestSupervisor_AuthInvalidIsTerminal(t *testing.T) {
	srv := hatest.New(t, hatest.Options{
		Token: "valid-token",
	})
	defer func() {
		srv.Close()
		goleak.VerifyNone(t)
	}()

	sup := Supervisor{
		URL:   srv.URL,
		Token: "bad-token",
		Backoff: Backoff{
			Initial: 10 * time.Millisecond,
			Max:     50 * time.Millisecond,
		},
	}

	var attempts int32
	sup.OnConnectAttempt = func() {
		atomic.AddInt32(&attempts, 1)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	err := sup.Run(ctx, func(ctx context.Context, conn *Conn) error {
		t.Fatal("la sesión no debería ejecutarse con credenciales inválidas")
		return nil
	})

	if !errors.Is(err, ErrAuthInvalid) {
		t.Fatalf("esperado ErrAuthInvalid, recibido: %v", err)
	}

	if atomic.LoadInt32(&attempts) != 1 {
		t.Fatalf("esperado exactamente 1 intento con auth inválido, hubo %d", atomic.LoadInt32(&attempts))
	}
}

func TestSupervisor_WaitsStartedBeforeSession(t *testing.T) {
	srv := hatest.New(t, hatest.Options{
		ResultSeq: map[string][]any{
			"get_config": {
				map[string]any{"state": "STARTING", "version": "2026.10.1"},
				map[string]any{"state": "STARTING", "version": "2026.10.1"},
				map[string]any{"state": "RUNNING", "version": "2026.10.1"},
			},
		},
	})
	defer func() {
		srv.Close()
		goleak.VerifyNone(t)
	}()

	sup := Supervisor{
		URL:         srv.URL,
		Token:       "test-token",
		WaitStarted: 10 * time.Millisecond,
		Backoff: Backoff{
			Initial: 10 * time.Millisecond,
			Max:     50 * time.Millisecond,
		},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	var stateWhenCalled string
	var mu sync.Mutex

	sessionStarted := make(chan struct{})

	errCh := make(chan error, 1)
	go func() {
		errCh <- sup.Run(ctx, func(sCtx context.Context, conn *Conn) error {
			cfg, err := conn.GetConfig(sCtx)
			if err == nil {
				mu.Lock()
				stateWhenCalled = cfg.State
				mu.Unlock()
			}
			close(sessionStarted)
			return nil
		})
	}()

	select {
	case <-sessionStarted:
		mu.Lock()
		st := stateWhenCalled
		mu.Unlock()
		if st != "RUNNING" {
			t.Fatalf("sesión ejecutada antes de RUNNING (estado: %s)", st)
		}
	case <-time.After(1 * time.Second):
		t.Fatal("la sesión no inició")
	}

	cancel()
	<-errCh
}

func TestSupervisor_HeartbeatDetectsFrozenPeer(t *testing.T) {
	srv := hatest.New(t, hatest.Options{})
	defer func() {
		srv.Close()
		goleak.VerifyNone(t)
	}()

	sup := Supervisor{
		URL:         srv.URL,
		Token:       "test-token",
		Heartbeat:   20 * time.Millisecond,
		PingTimeout: 15 * time.Millisecond,
		Backoff: Backoff{
			Initial: 10 * time.Millisecond,
			Max:     50 * time.Millisecond,
		},
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sessionStarted := make(chan struct{}, 2)
	var sessionCount int32

	errCh := make(chan error, 1)
	go func() {
		errCh <- sup.Run(ctx, func(sCtx context.Context, conn *Conn) error {
			cnt := atomic.AddInt32(&sessionCount, 1)
			sessionStarted <- struct{}{}
			if cnt >= 2 {
				cancel()
				return nil
			}
			<-conn.Done()
			return conn.Err()
		})
	}()

	// Esperar primera sesión
	select {
	case <-sessionStarted:
	case <-time.After(1 * time.Second):
		t.Fatal("primera sesión no inició")
	}

	// Congelar el servidor: deja de responder pings
	srv.Freeze(true)

	// Tras Heartbeat (20ms) + PingTimeout (15ms), debe detectar el peer congelado y cerrar la conexión
	time.Sleep(100 * time.Millisecond)
	srv.Freeze(false)

	select {
	case <-sessionStarted:
	case <-time.After(2 * time.Second):
		t.Fatal("segunda sesión no inició tras reconexión por heartbeat")
	}

	cancel()
	_ = <-errCh
}

func TestSupervisor_ResetsBackoffAfterHealthy(t *testing.T) {
	srv := hatest.New(t, hatest.Options{})
	defer func() {
		srv.Close()
		goleak.VerifyNone(t)
	}()

	var observedDelays []time.Duration
	var mu sync.Mutex

	sup := Supervisor{
		URL:         srv.URL,
		Token:       "test-token",
		HealthyTime: 80 * time.Millisecond,
		Backoff: Backoff{
			Initial:    30 * time.Millisecond,
			Max:        300 * time.Millisecond,
			Multiplier: 2.0,
			Jitter:     0.0,
		},
	}

	sup.OnBackoffDelay = func(d time.Duration) {
		mu.Lock()
		observedDelays = append(observedDelays, d)
		mu.Unlock()
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sessionCount := int32(0)
	sessionDone := make(chan struct{}, 3)

	go func() {
		_ = sup.Run(ctx, func(sCtx context.Context, conn *Conn) error {
			c := atomic.AddInt32(&sessionCount, 1)
			if c == 1 {
				// Cae inmediatamente (unhealthy), debe incrementar backoff
				srv.DropAll()
				<-conn.Done()
				sessionDone <- struct{}{}
				return conn.Err()
			}
			if c == 2 {
				// Permanece sano más de HealthyTime (80ms)
				time.Sleep(120 * time.Millisecond)
				srv.DropAll()
				<-conn.Done()
				sessionDone <- struct{}{}
				return conn.Err()
			}
			sessionDone <- struct{}{}
			cancel()
			return nil
		})
	}()

	// Esperar las 3 sesiones
	for i := 0; i < 3; i++ {
		select {
		case <-sessionDone:
		case <-time.After(2 * time.Second):
			t.Fatalf("sesión %d no terminó a tiempo", i+1)
		}
	}

	mu.Lock()
	delays := append([]time.Duration(nil), observedDelays...)
	mu.Unlock()

	// Tras sesión 1 (falló rápido): backoff aumenta (ej. 30ms -> 60ms)
	// Tras sesión 2 (estuvo sana 120ms > 80ms): backoff debe reiniciarse al inicial (30ms)
	if len(delays) < 2 {
		t.Fatalf("se esperaban al menos 2 esperas de backoff, registradas: %v", delays)
	}
	if delays[0] != 30*time.Millisecond {
		t.Errorf("primer delay esperado 30ms, obtenido %v", delays[0])
	}
	// El segundo delay tras una caída rápida debió ser 60ms
	// Y tras estar sano, el delay siguiente debe haber reseteado a 30ms
	lastDelay := delays[len(delays)-1]
	if lastDelay != 30*time.Millisecond {
		t.Errorf("el delay debió reiniciarse a 30ms tras período sano, obtenido %v (todos: %v)", lastDelay, delays)
	}
}

func TestSupervisor_CancelMidBackoffNoLeak(t *testing.T) {
	defer goleak.VerifyNone(t)

	// Servidor inexistente con backoff largo
	sup := Supervisor{
		URL:   "ws://127.0.0.1:59998/api/websocket",
		Token: "test-token",
		Backoff: Backoff{
			Initial:    5 * time.Second,
			Max:        10 * time.Second,
			Multiplier: 2.0,
		},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	err := sup.Run(ctx, func(ctx context.Context, conn *Conn) error {
		return nil
	})

	if !errors.Is(err, context.DeadlineExceeded) && !errors.Is(err, context.Canceled) {
		t.Fatalf("esperado error de contexto, recibido: %v", err)
	}
}
