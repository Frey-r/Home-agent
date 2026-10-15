package ha_test

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/Frey-r/Home-agent/edge/internal/ha"
	"github.com/Frey-r/Home-agent/edge/internal/ha/hatest"
	"go.uber.org/goleak"
)

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}

func TestSubscribeEvents_DeliversInOrder(t *testing.T) {
	srv := hatest.New(t, hatest.Options{})
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	conn, err := ha.Connect(ctx, srv.URL, "test-token")
	if err != nil {
		t.Fatalf("Connect error: %v", err)
	}
	defer conn.Close()

	sub, err := conn.SubscribeEvents(ctx, 10, "state_changed")
	if err != nil {
		t.Fatalf("SubscribeEvents error: %v", err)
	}
	defer sub.Close()

	srv.WaitSubscribed(t, "state_changed", 2*time.Second)

	totalEvents := 5
	for i := 1; i <= totalEvents; i++ {
		srv.Push(map[string]any{
			"event_type": "state_changed",
			"data": map[string]any{
				"entity_id": fmt.Sprintf("light.lamp_%d", i),
				"new_state": map[string]any{"state": "on"},
			},
			"time_fired": time.Now().UTC().Format(time.RFC3339Nano),
		})
	}

	for i := 1; i <= totalEvents; i++ {
		select {
		case ev := <-sub.C():
			sc, err := ha.DecodeStateChanged(ev)
			if err != nil {
				t.Fatalf("DecodeStateChanged error: %v", err)
			}
			expectedID := fmt.Sprintf("light.lamp_%d", i)
			if sc.EntityID != expectedID {
				t.Fatalf("expected event for %s, got %s", expectedID, sc.EntityID)
			}
		case <-time.After(2 * time.Second):
			t.Fatalf("timeout waiting for event %d", i)
		}
	}
}

func TestSubscribeEvents_FanIn(t *testing.T) {
	srv := hatest.New(t, hatest.Options{})
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	conn, err := ha.Connect(ctx, srv.URL, "test-token")
	if err != nil {
		t.Fatalf("Connect error: %v", err)
	}
	defer conn.Close()

	sub1, err := conn.SubscribeEvents(ctx, 10, "state_changed")
	if err != nil {
		t.Fatalf("sub1 error: %v", err)
	}
	defer sub1.Close()

	sub2, err := conn.SubscribeEvents(ctx, 10, "call_service")
	if err != nil {
		t.Fatalf("sub2 error: %v", err)
	}
	defer sub2.Close()

	srv.WaitSubscribed(t, "state_changed", 2*time.Second)
	srv.WaitSubscribed(t, "call_service", 2*time.Second)

	srv.Push(map[string]any{"event_type": "state_changed", "data": map[string]any{"entity_id": "light.a"}})
	srv.Push(map[string]any{"event_type": "call_service", "data": map[string]any{"domain": "light"}})

	select {
	case ev1 := <-sub1.C():
		if ev1.Type != "state_changed" {
			t.Fatalf("sub1 got unexpected type: %s", ev1.Type)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("sub1 timeout")
	}

	select {
	case ev2 := <-sub2.C():
		if ev2.Type != "call_service" {
			t.Fatalf("sub2 got unexpected type: %s", ev2.Type)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("sub2 timeout")
	}
}

func TestSubscribeEvents_EventInSameFrameAsResult(t *testing.T) {
	srv := hatest.New(t, hatest.Options{})
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	conn, err := ha.Connect(ctx, srv.URL, "test-token")
	if err != nil {
		t.Fatalf("Connect error: %v", err)
	}
	defer conn.Close()

	// Interceptar cuando llega subscribe_events y enviar inmediatamente en un array o frame conjunto
	sub, err := conn.SubscribeEvents(ctx, 10, "state_changed")
	if err != nil {
		t.Fatalf("SubscribeEvents error: %v", err)
	}
	defer sub.Close()

	srv.WaitSubscribed(t, "state_changed", 2*time.Second)

	// Enviar un frame que contiene el evento
	srv.PushFrame([]byte(`{"id": 1, "type": "event", "event": {"event_type": "state_changed", "data": {"entity_id": "light.fast"}}}`))

	select {
	case ev := <-sub.C():
		if ev.Type != "state_changed" {
			t.Fatalf("unexpected type: %s", ev.Type)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for event in frame")
	}
}

func TestReadLoop_CoalescedArray(t *testing.T) {
	srv := hatest.New(t, hatest.Options{})
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	conn, err := ha.Connect(ctx, srv.URL, "test-token")
	if err != nil {
		t.Fatalf("Connect error: %v", err)
	}
	defer conn.Close()

	sub, err := conn.SubscribeEvents(ctx, 10, "state_changed")
	if err != nil {
		t.Fatalf("SubscribeEvents error: %v", err)
	}
	defer sub.Close()

	srv.WaitSubscribed(t, "state_changed", 2*time.Second)

	// Enviar un lote como array JSON coalescente
	batch := `[
		{"id": 1, "type": "event", "event": {"event_type": "state_changed", "data": {"entity_id": "light.c1"}}},
		{"id": 1, "type": "event", "event": {"event_type": "state_changed", "data": {"entity_id": "light.c2"}}}
	]`
	srv.PushFrame([]byte(batch))

	for _, expected := range []string{"light.c1", "light.c2"} {
		select {
		case ev := <-sub.C():
			sc, err := ha.DecodeStateChanged(ev)
			if err != nil {
				t.Fatalf("DecodeStateChanged error: %v", err)
			}
			if sc.EntityID != expected {
				t.Fatalf("expected %s, got %s", expected, sc.EntityID)
			}
		case <-time.After(2 * time.Second):
			t.Fatalf("timeout waiting for %s", expected)
		}
	}
}

func TestConnect_NegotiatesCoalescing(t *testing.T) {
	srv := hatest.New(t, hatest.Options{})
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	conn, err := ha.Connect(ctx, srv.URL, "test-token", ha.WithCoalescing())
	if err != nil {
		t.Fatalf("Connect error: %v", err)
	}
	defer conn.Close()

	// Comprobar que en los comandos recibidos por el servidor falso se solicitó supported_features o coalesce_messages
	cmds := srv.Received()
	var negotiated bool
	for _, cmd := range cmds {
		if cmd["type"] == "supported_features" || cmd["type"] == "coalesce_messages" {
			negotiated = true
			break
		}
	}
	if !negotiated {
		t.Fatal("expected coalesce_messages negotiation")
	}
}

func TestSubscription_SlowConsumerOverflow(t *testing.T) {
	srv := hatest.New(t, hatest.Options{})
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	conn, err := ha.Connect(ctx, srv.URL, "test-token")
	if err != nil {
		t.Fatalf("Connect error: %v", err)
	}
	defer conn.Close()

	// Buffer pequeño de tamaño 2
	sub, err := conn.SubscribeEvents(ctx, 2, "state_changed")
	if err != nil {
		t.Fatalf("SubscribeEvents error: %v", err)
	}
	defer sub.Close()

	srv.WaitSubscribed(t, "state_changed", 2*time.Second)

	// Llenar buffer con 2 eventos y enviar un tercero para desbordar
	srv.Push(map[string]any{"event_type": "state_changed", "data": map[string]any{"entity_id": "light.1"}})
	srv.Push(map[string]any{"event_type": "state_changed", "data": map[string]any{"entity_id": "light.2"}})
	srv.Push(map[string]any{"event_type": "state_changed", "data": map[string]any{"entity_id": "light.3"}})

	// El suscriptor debe recibir ErrSlowConsumer en sub.Err()
	select {
	case err := <-sub.Err():
		if !errors.Is(err, ha.ErrSlowConsumer) {
			t.Fatalf("expected ErrSlowConsumer, got: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("expected ErrSlowConsumer on overflow, timed out")
	}
}

func TestSubscription_ClosedOnDrop(t *testing.T) {
	srv := hatest.New(t, hatest.Options{})
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	conn, err := ha.Connect(ctx, srv.URL, "test-token")
	if err != nil {
		t.Fatalf("Connect error: %v", err)
	}

	sub, err := conn.SubscribeEvents(ctx, 10, "state_changed")
	if err != nil {
		t.Fatalf("SubscribeEvents error: %v", err)
	}
	defer sub.Close()

	srv.WaitSubscribed(t, "state_changed", 2*time.Second)

	// Cortar servidor
	srv.DropAll()

	// sub.Err() debe emitir error o cerrarse
	select {
	case err := <-sub.Err():
		if err == nil {
			t.Fatal("expected non-nil error from sub.Err() on drop")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for sub.Err() on connection drop")
	}
}

func TestSubscription_CloseRacesWithEvents(t *testing.T) {
	srv := hatest.New(t, hatest.Options{})
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	conn, err := ha.Connect(ctx, srv.URL, "test-token")
	if err != nil {
		t.Fatalf("Connect error: %v", err)
	}
	defer conn.Close()

	sub, err := conn.SubscribeEvents(ctx, 100, "state_changed")
	if err != nil {
		t.Fatalf("SubscribeEvents error: %v", err)
	}

	srv.WaitSubscribed(t, "state_changed", 2*time.Second)

	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()
		for i := 0; i < 50; i++ {
			srv.Push(map[string]any{
				"event_type": "state_changed",
				"data":       map[string]any{"entity_id": fmt.Sprintf("light.%d", i)},
			})
		}
	}()

	go func() {
		defer wg.Done()
		time.Sleep(5 * time.Millisecond)
		_ = sub.Close()
	}()

	wg.Wait()
}

func TestConn_DoneOnDrop(t *testing.T) {
	srv := hatest.New(t, hatest.Options{})
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	conn, err := ha.Connect(ctx, srv.URL, "test-token")
	if err != nil {
		t.Fatalf("Connect error: %v", err)
	}
	defer conn.Close()

	srv.DropAll()

	select {
	case <-conn.Done():
		if conn.Err() == nil {
			t.Fatal("expected non-nil Err() on drop")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for conn.Done()")
	}
}

func TestPing(t *testing.T) {
	srv := hatest.New(t, hatest.Options{})
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	conn, err := ha.Connect(ctx, srv.URL, "test-token")
	if err != nil {
		t.Fatalf("Connect error: %v", err)
	}
	defer conn.Close()

	if err := conn.Ping(ctx); err != nil {
		t.Fatalf("Ping() error: %v", err)
	}
}
