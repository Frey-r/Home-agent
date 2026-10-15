package entities

import (
	"context"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Frey-r/Home-agent/edge/internal/ha"
	"github.com/Frey-r/Home-agent/edge/internal/ha/hatest"
)

func defaultResults() map[string]any {
	return map[string]any{
		"config/area_registry/list": []any{
			map[string]any{"id": "salon", "name": "Salón"},
		},
		"config/device_registry/list": []any{
			map[string]any{"id": "dev_1", "name": "Puente Luces", "area_id": "salon"},
		},
		"config/entity_registry/list": []any{
			map[string]any{"id": "e_1", "entity_id": "light.salon", "area_id": "salon"},
		},
		"homeassistant/expose_entity/list": map[string]any{
			"exposed_entities": map[string]any{
				"conversation": map[string]any{
					"light.salon": true,
				},
			},
		},
		"get_states": []map[string]any{
			{
				"entity_id":    "light.salon",
				"state":        "off",
				"attributes":   map[string]any{"friendly_name": "Luz Salón"},
				"last_updated": "2026-10-15T10:00:00.000000+00:00",
			},
		},
	}
}

func TestSyncer_InitialSnapshot(t *testing.T) {
	srv := hatest.New(t, hatest.Options{
		Results: defaultResults(),
	})
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	conn, err := ha.Connect(ctx, srv.URL, "test-token")
	if err != nil {
		t.Fatalf("Connect error: %v", err)
	}
	defer conn.Close()

	dir := NewDirectory()
	syncer := NewSyncer(dir)

	syncCtx, syncCancel := context.WithCancel(ctx)
	defer syncCancel()

	errCh := make(chan error, 1)
	go func() {
		errCh <- syncer.Run(syncCtx, conn)
	}()

	// Esperar a que el snapshot inicial se aplique
	if err := syncer.WaitSynced(ctx, 2*time.Second); err != nil {
		t.Fatalf("WaitSynced timeout: %v", err)
	}

	ent, found := dir.Get("ha:light.salon")
	if !found || ent == nil {
		t.Fatal("entidad ha:light.salon no encontrada en Directory tras snapshot")
	}
	if ent.Name != "Luz Salón" {
		t.Errorf("Name: got %q, want 'Luz Salón'", ent.Name)
	}
	if !ent.Exposed {
		t.Errorf("Exposed: expected true")
	}
	if ent.Area == nil || ent.Area.Name != "Salón" {
		t.Errorf("Area: got %+v, want Salón", ent.Area)
	}
}

func TestSyncer_ConnectedFlag(t *testing.T) {
	srv := hatest.New(t, hatest.Options{
		Results: defaultResults(),
	})
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	conn, err := ha.Connect(ctx, srv.URL, "test-token")
	if err != nil {
		t.Fatalf("Connect error: %v", err)
	}
	defer conn.Close()

	dir := NewDirectory()
	syncer := NewSyncer(dir)

	if syncer.Connected() {
		t.Error("Connected debe ser falso antes de Run")
	}

	syncCtx, syncCancel := context.WithCancel(ctx)
	defer syncCancel()

	go func() {
		_ = syncer.Run(syncCtx, conn)
	}()

	_ = syncer.WaitSynced(ctx, 2*time.Second)
	if !syncer.Connected() {
		t.Error("Connected debe ser verdadero tras sincronizar")
	}

	// Cerrar conexión
	conn.Close()
	time.Sleep(50 * time.Millisecond)

	if syncer.Connected() {
		t.Error("Connected debe volver a falso tras desconexión")
	}
}

func TestSyncer_StateChangedUnder1s(t *testing.T) {
	srv := hatest.New(t, hatest.Options{
		Results: defaultResults(),
	})
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	conn, err := ha.Connect(ctx, srv.URL, "test-token")
	if err != nil {
		t.Fatalf("Connect error: %v", err)
	}
	defer conn.Close()

	dir := NewDirectory()
	syncer := NewSyncer(dir)

	syncCtx, syncCancel := context.WithCancel(ctx)
	defer syncCancel()

	go func() {
		_ = syncer.Run(syncCtx, conn)
	}()

	if err := syncer.WaitSynced(ctx, 2*time.Second); err != nil {
		t.Fatal(err)
	}

	watchCh := dir.Watch()

	// Emitir evento state_changed en < 1s
	start := time.Now()
	srv.Push(map[string]any{
		"event_type": "state_changed",
		"data": map[string]any{
			"entity_id": "light.salon",
			"new_state": map[string]any{
				"entity_id":    "light.salon",
				"state":        "on",
				"attributes":   map[string]any{"friendly_name": "Luz Salón", "brightness": 255},
				"last_updated": time.Now().UTC().Format(time.RFC3339Nano),
			},
		},
	})

	select {
	case <-watchCh:
		elapsed := time.Since(start)
		if elapsed > time.Second {
			t.Errorf("state_changed tomó %v (> 1s)", elapsed)
		}
		ent, _ := dir.Get("ha:light.salon")
		if ent.State["power"] != "on" {
			t.Errorf("power: got %v, want 'on'", ent.State["power"])
		}
	case <-time.After(1 * time.Second):
		t.Fatal("timeout esperando evento state_changed (< 1s)")
	}
}

func TestSyncer_EntityAddedAfterStart(t *testing.T) {
	srv := hatest.New(t, hatest.Options{
		Results: defaultResults(),
	})
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	conn, err := ha.Connect(ctx, srv.URL, "test-token")
	if err != nil {
		t.Fatalf("Connect error: %v", err)
	}
	defer conn.Close()

	dir := NewDirectory()
	syncer := NewSyncer(dir)

	syncCtx, syncCancel := context.WithCancel(ctx)
	defer syncCancel()

	go func() {
		_ = syncer.Run(syncCtx, conn)
	}()

	_ = syncer.WaitSynced(ctx, 2*time.Second)

	watchCh := dir.Watch()

	// Alta de nueva entidad vía state_changed
	srv.Push(map[string]any{
		"event_type": "state_changed",
		"data": map[string]any{
			"entity_id": "switch.termo",
			"new_state": map[string]any{
				"entity_id":    "switch.termo",
				"state":        "on",
				"attributes":   map[string]any{"friendly_name": "Termo Eléctrico"},
				"last_updated": time.Now().UTC().Format(time.RFC3339Nano),
			},
		},
	})

	select {
	case <-watchCh:
		ent, found := dir.Get("ha:switch.termo")
		if !found || ent == nil {
			t.Fatal("nueva entidad ha:switch.termo no encontrada")
		}
		if ent.Name != "Termo Eléctrico" {
			t.Errorf("Name: got %q, want 'Termo Eléctrico'", ent.Name)
		}
	case <-time.After(1 * time.Second):
		t.Fatal("timeout esperando alta de nueva entidad")
	}
}

func TestSyncer_EntityRemoved(t *testing.T) {
	srv := hatest.New(t, hatest.Options{
		Results: defaultResults(),
	})
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	conn, err := ha.Connect(ctx, srv.URL, "test-token")
	if err != nil {
		t.Fatalf("Connect error: %v", err)
	}
	defer conn.Close()

	dir := NewDirectory()
	syncer := NewSyncer(dir)

	syncCtx, syncCancel := context.WithCancel(ctx)
	defer syncCancel()

	go func() {
		_ = syncer.Run(syncCtx, conn)
	}()

	_ = syncer.WaitSynced(ctx, 2*time.Second)

	watchCh := dir.Watch()

	// Eliminación: new_state = nil
	srv.Push(map[string]any{
		"event_type": "state_changed",
		"data": map[string]any{
			"entity_id": "light.salon",
			"new_state": nil,
		},
	})

	select {
	case <-watchCh:
		_, found := dir.Get("ha:light.salon")
		if found {
			t.Error("ha:light.salon sigue existiendo tras evento new_state=nil")
		}
	case <-time.After(1 * time.Second):
		t.Fatal("timeout esperando baja de entidad")
	}
}

func TestSyncer_EventDuringSnapshot_OlderIgnored(t *testing.T) {
	srv := hatest.New(t, hatest.Options{
		Results: defaultResults(),
	})
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	conn, err := ha.Connect(ctx, srv.URL, "test-token")
	if err != nil {
		t.Fatalf("Connect error: %v", err)
	}
	defer conn.Close()

	dir := NewDirectory()
	syncer := NewSyncer(dir)

	syncCtx, syncCancel := context.WithCancel(ctx)
	defer syncCancel()

	go func() {
		_ = syncer.Run(syncCtx, conn)
	}()

	_ = syncer.WaitSynced(ctx, 2*time.Second)

	// Inyectar evento más nuevo
	newerTime := time.Now().UTC()
	srv.Push(map[string]any{
		"event_type": "state_changed",
		"data": map[string]any{
			"entity_id": "light.salon",
			"new_state": map[string]any{
				"entity_id":    "light.salon",
				"state":        "on",
				"attributes":   map[string]any{"friendly_name": "Luz Salón"},
				"last_updated": newerTime.Format(time.RFC3339Nano),
			},
		},
	})

	time.Sleep(50 * time.Millisecond)

	// Inyectar evento viejo (anterior a newerTime) -> debe ser ignorado
	olderTime := newerTime.Add(-10 * time.Minute)
	vBefore := dir.Version()

	srv.Push(map[string]any{
		"event_type": "state_changed",
		"data": map[string]any{
			"entity_id": "light.salon",
			"new_state": map[string]any{
				"entity_id":    "light.salon",
				"state":        "off",
				"attributes":   map[string]any{"friendly_name": "Luz Salón"},
				"last_updated": olderTime.Format(time.RFC3339Nano),
			},
		},
	})

	time.Sleep(50 * time.Millisecond)

	// La versión no debe haber cambiado y el estado debe seguir en "on"
	ent, _ := dir.Get("ha:light.salon")
	if ent.State["power"] != "on" {
		t.Errorf("evento viejo sobreescribió estado más nuevo: power=%v", ent.State["power"])
	}
	if dir.Version() != vBefore {
		t.Errorf("evento viejo incrementó la versión: antes=%d, ahora=%d", vBefore, dir.Version())
	}
}

func TestSyncer_RegistryStormDebounced(t *testing.T) {
	var registryFetchCount int32
	srv := hatest.New(t, hatest.Options{
		Results: defaultResults(),
	})
	srv.OnCommand(func(cmd map[string]any) {
		if typ, ok := cmd["type"].(string); ok && typ == "config/entity_registry/list" {
			atomic.AddInt32(&registryFetchCount, 1)
		}
	})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	conn, err := ha.Connect(ctx, srv.URL, "test-token")
	if err != nil {
		t.Fatalf("Connect error: %v", err)
	}
	defer conn.Close()

	dir := NewDirectory()
	syncer := NewSyncer(dir)
	syncer.RegistryDebounce = 40 * time.Millisecond

	syncCtx, syncCancel := context.WithCancel(ctx)
	defer syncCancel()

	go func() {
		_ = syncer.Run(syncCtx, conn)
	}()

	_ = syncer.WaitSynced(ctx, 2*time.Second)

	fetchesBeforeStorm := atomic.LoadInt32(&registryFetchCount)

	// Ráfaga de 50 eventos de registro en < 20ms
	for i := 0; i < 50; i++ {
		srv.Push(map[string]any{
			"event_type": "entity_registry_updated",
			"data": map[string]any{
				"action":    "update",
				"entity_id": fmt.Sprintf("light.lamp_%d", i),
			},
		})
	}

	// Esperar que pase el debounce
	time.Sleep(120 * time.Millisecond)

	fetchesAfterStorm := atomic.LoadInt32(&registryFetchCount)
	additionalFetches := fetchesAfterStorm - fetchesBeforeStorm

	// Debe haber coalescido la ráfaga de 50 eventos en <= 2 relecturas
	if additionalFetches > 2 {
		t.Fatalf("ráfaga no fue debounced adecuadamente: %d relecturas para 50 eventos", additionalFetches)
	}
}

func TestSyncer_DropDuringSnapshotNoPartialApply(t *testing.T) {
	srv := hatest.New(t, hatest.Options{
		DropOn: "get_states", // simula caída en medio del snapshot
		Results: defaultResults(),
	})
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	conn, err := ha.Connect(ctx, srv.URL, "test-token")
	if err != nil {
		t.Fatalf("Connect error: %v", err)
	}
	defer conn.Close()

	dir := NewDirectory()
	syncer := NewSyncer(dir)

	err = syncer.Run(ctx, conn)
	if err == nil {
		t.Fatal("esperado error al caerse la conexión durante el snapshot")
	}

	// El directorio no debe contener estados parciales
	list := dir.List(Filter{})
	if len(list) != 0 {
		t.Fatalf("el directorio contiene estados parciales (%d entidades) tras fallo en snapshot", len(list))
	}
}
