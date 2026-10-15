package main

import (
	"context"
	"testing"
	"time"

	"github.com/Frey-r/Home-agent/edge/internal/ha"
	"github.com/Frey-r/Home-agent/edge/internal/ha/hatest"
)

func TestSeed_SendsExpectedCommands(t *testing.T) {
	srv := hatest.New(t, hatest.Options{
		Results: map[string]any{
			"config/area_registry/create": map[string]any{"id": "salon", "name": "Salón"},
			"config/area_registry/list": []any{
				map[string]any{"id": "salon", "name": "Salón"},
				map[string]any{"id": "cocina", "name": "Cocina"},
			},
			"config/device_registry/list": []any{
				map[string]any{"id": "dev_1", "name": "Lámpara Dev"},
				map[string]any{"id": "dev_2", "name": "Termostato Dev"},
			},
			"config/device_registry/update": map[string]any{},
			"config/entity_registry/update": map[string]any{},
			"homeassistant/expose_entity":   map[string]any{},
		},
	})
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	conn, err := ha.Connect(ctx, srv.URL, "test-token")
	if err != nil {
		t.Fatalf("Connect error: %v", err)
	}
	defer conn.Close()

	if err := runSeed(ctx, conn); err != nil {
		t.Fatalf("runSeed error: %v", err)
	}

	cmds := srv.Received()
	var hasExpose, hasDeviceUpdate, hasEntityAlias bool

	for _, cmd := range cmds {
		typ, _ := cmd["type"].(string)
		switch typ {
		case "homeassistant/expose_entity":
			hasExpose = true
		case "config/device_registry/update":
			hasDeviceUpdate = true
		case "config/entity_registry/update":
			aliases, ok := cmd["aliases"].([]any)
			if ok {
				for _, a := range aliases {
					if a == "luz de pie" {
						hasEntityAlias = true
					}
				}
			}
		}
	}

	if !hasExpose {
		t.Error("expected homeassistant/expose_entity command to be sent")
	}
	if !hasDeviceUpdate {
		t.Error("expected config/device_registry/update command to assign areas to devices")
	}
	if !hasEntityAlias {
		t.Error("expected config/entity_registry/update with alias 'luz de pie'")
	}
}

func TestSeed_Idempotent(t *testing.T) {
	srv := hatest.New(t, hatest.Options{
		Results: map[string]any{
			"config/area_registry/create": map[string]any{"id": "salon", "name": "Salón"},
			"config/area_registry/list": []any{
				map[string]any{"id": "salon", "name": "Salón"},
				map[string]any{"id": "cocina", "name": "Cocina"},
			},
			"config/device_registry/list": []any{
				map[string]any{"id": "dev_1", "name": "Lámpara Dev"},
				map[string]any{"id": "dev_2", "name": "Termostato Dev"},
			},
			"config/device_registry/update": map[string]any{},
			"config/entity_registry/update": map[string]any{},
			"homeassistant/expose_entity":   map[string]any{},
		},
	})
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	conn, err := ha.Connect(ctx, srv.URL, "test-token")
	if err != nil {
		t.Fatalf("Connect error: %v", err)
	}
	defer conn.Close()

	// Primera ejecución
	if err := runSeed(ctx, conn); err != nil {
		t.Fatalf("first runSeed error: %v", err)
	}

	// Segunda ejecución (debe ser idempotente)
	if err := runSeed(ctx, conn); err != nil {
		t.Fatalf("second runSeed error (idempotency check): %v", err)
	}
}
