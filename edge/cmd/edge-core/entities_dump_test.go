package main

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Frey-r/Home-agent/contracts"
	"github.com/Frey-r/Home-agent/edge/internal/ha/hatest"
)

func dumpMockResults() map[string]any {
	return map[string]any{
		"config/area_registry/list": []any{
			map[string]any{"id": "salon", "name": "Salón"},
		},
		"config/device_registry/list": []any{
			map[string]any{"id": "dev_1", "name": "Puente Luces", "area_id": "salon"},
		},
		"config/entity_registry/list": []any{
			map[string]any{"id": "e_1", "entity_id": "light.salon", "area_id": "salon"},
			map[string]any{"id": "e_2", "entity_id": "switch.ventilador", "area_id": "salon"},
		},
		"homeassistant/expose_entity/list": map[string]any{
			"exposed_entities": map[string]any{
				"conversation": map[string]any{
					"light.salon":       true,
					"switch.ventilador": false,
				},
			},
		},
		"get_states": []map[string]any{
			{
				"entity_id":    "light.salon",
				"state":        "on",
				"attributes":   map[string]any{"friendly_name": "Luz Salón", "supported_features": 36},
				"last_updated": "2026-10-15T10:00:00.000000+00:00",
			},
			{
				"entity_id":    "switch.ventilador",
				"state":        "off",
				"attributes":   map[string]any{"friendly_name": "Ventilador"},
				"last_updated": "2026-10-15T10:00:00.000000+00:00",
			},
		},
	}
}

func TestEntitiesDump(t *testing.T) {
	srv := hatest.New(t, hatest.Options{
		Results:   dumpMockResults(),
		HAVersion: "2026.10.0",
	})
	dataDir := t.TempDir()

	env := map[string]string{
		"DOMOTICA_MODE":     "dev",
		"DOMOTICA_HA_URL":   srv.URL,
		"DOMOTICA_HA_TOKEN": "test-token",
		"DOMOTICA_DATA_DIR": dataDir,
	}

	t.Run("dump all entities", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		var stdout, stderr bytes.Buffer
		err := run(ctx, []string{"entities", "dump"}, lookupFrom(env), &stdout, &stderr)
		if err != nil {
			t.Fatalf("run entities dump failed: %v, stderr: %s", err, stderr.String())
		}

		var entities []*contracts.Entity
		if err := json.Unmarshal(stdout.Bytes(), &entities); err != nil {
			t.Fatalf("failed to decode JSON output: %v, raw: %s", err, stdout.String())
		}

		if len(entities) != 2 {
			t.Fatalf("expected 2 entities, got %d", len(entities))
		}
	})

	t.Run("dump exposed only", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		var stdout, stderr bytes.Buffer
		err := run(ctx, []string{"entities", "dump", "--exposed-only"}, lookupFrom(env), &stdout, &stderr)
		if err != nil {
			t.Fatalf("run entities dump failed: %v, stderr: %s", err, stderr.String())
		}

		var entities []*contracts.Entity
		if err := json.Unmarshal(stdout.Bytes(), &entities); err != nil {
			t.Fatalf("failed to decode JSON: %v, raw: %s", err, stdout.String())
		}

		if len(entities) != 1 {
			t.Fatalf("expected 1 exposed entity, got %d", len(entities))
		}
		if entities[0].ID != "ha:light.salon" {
			t.Errorf("expected light.salon, got %s", entities[0].ID)
		}
	})

	t.Run("dump category filter", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		var stdout, stderr bytes.Buffer
		err := run(ctx, []string{"entities", "dump", "--category", "switch"}, lookupFrom(env), &stdout, &stderr)
		if err != nil {
			t.Fatalf("run entities dump failed: %v, stderr: %s", err, stderr.String())
		}

		var entities []*contracts.Entity
		if err := json.Unmarshal(stdout.Bytes(), &entities); err != nil {
			t.Fatalf("failed to decode JSON: %v, raw: %s", err, stdout.String())
		}

		if len(entities) != 1 {
			t.Fatalf("expected 1 entity, got %d", len(entities))
		}
		if entities[0].Category != "switch" {
			t.Errorf("expected switch category, got %s", entities[0].Category)
		}
	})

	t.Run("dump unknown category returns empty array", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		var stdout, stderr bytes.Buffer
		err := run(ctx, []string{"entities", "dump", "--category", "climate"}, lookupFrom(env), &stdout, &stderr)
		if err != nil {
			t.Fatalf("run entities dump failed: %v, stderr: %s", err, stderr.String())
		}

		var entities []*contracts.Entity
		if err := json.Unmarshal(stdout.Bytes(), &entities); err != nil {
			t.Fatalf("failed to decode JSON: %v, raw: %s", err, stdout.String())
		}

		if len(entities) != 0 {
			t.Fatalf("expected 0 entities, got %d", len(entities))
		}
	})

	t.Run("entities without subcommand returns usage error", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()

		var stdout, stderr bytes.Buffer
		err := run(ctx, []string{"entities"}, lookupFrom(env), &stdout, &stderr)
		if err == nil || !strings.Contains(err.Error(), "uso") {
			t.Fatalf("expected usage error, got %v", err)
		}
	})

	t.Run("entities unknown subcommand returns error", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()

		var stdout, stderr bytes.Buffer
		err := run(ctx, []string{"entities", "volar"}, lookupFrom(env), &stdout, &stderr)
		if err == nil || !strings.Contains(err.Error(), "volar") {
			t.Fatalf("expected unknown subcommand error, got %v", err)
		}
	})
}

type safeBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (s *safeBuffer) Write(p []byte) (n int, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.buf.Write(p)
}

func (s *safeBuffer) String() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.buf.String()
}

func TestRun_LiveUpdatesDirectory(t *testing.T) {
	srv := hatest.New(t, hatest.Options{
		Results:   dumpMockResults(),
		HAVersion: "2026.10.0",
	})
	dataDir := t.TempDir()

	env := map[string]string{
		"DOMOTICA_MODE":     "dev",
		"DOMOTICA_HA_URL":   srv.URL,
		"DOMOTICA_HA_TOKEN": "test-token",
		"DOMOTICA_DATA_DIR": dataDir,
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var stdout, stderr safeBuffer
	errCh := make(chan error, 1)

	go func() {
		errCh <- run(ctx, []string{"run"}, lookupFrom(env), &stdout, &stderr)
	}()

	// Esperar que arranque y sincronice inicialmente
	var synced bool
	for start := time.Now(); time.Since(start) < 3*time.Second; {
		if strings.Contains(stderr.String(), "conectado a Home Assistant") {
			synced = true
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if !synced {
		t.Fatalf("edge-core run no alcanzó estado conectado; logs:\n%s", stderr.String())
	}

	// Enviar evento de cambio de estado a través de hatest
	srv.Push(map[string]any{
		"event_type": "state_changed",
		"data": map[string]any{
			"entity_id": "light.salon",
			"new_state": map[string]any{
				"entity_id":    "light.salon",
				"state":        "off",
				"attributes":   map[string]any{"friendly_name": "Luz Salón"},
				"last_updated": "2026-10-15T10:05:00.000000+00:00",
			},
		},
	})

	// El cambio debe registrarse en el log en < 1s
	var changeLogged bool
	for start := time.Now(); time.Since(start) < 2*time.Second; {
		if strings.Contains(stderr.String(), "ha:light.salon") || strings.Contains(stderr.String(), "light.salon") {
			if strings.Contains(stderr.String(), "off") || strings.Contains(stderr.String(), "actualizad") {
				changeLogged = true
				break
			}
		}
		time.Sleep(30 * time.Millisecond)
	}

	cancel()
	err := <-errCh
	if err != nil && !strings.Contains(err.Error(), "context canceled") {
		t.Logf("run finalizado con: %v", err)
	}

	if !changeLogged {
		t.Errorf("cambio de estado en vivo no fue reflejado en log; stderr:\n%s", stderr.String())
	}
}
