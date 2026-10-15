package ha_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/Frey-r/Home-agent/edge/internal/ha"
	"github.com/Frey-r/Home-agent/edge/internal/ha/hatest"
)

func TestCallService_Payload(t *testing.T) {
	tests := []struct {
		name     string
		call     ha.ServiceCall
		wantData map[string]any
	}{
		{
			name: "light turn_on con target entity_id y brightness",
			call: ha.ServiceCall{
				Domain:  "light",
				Service: "turn_on",
				Target: ha.ServiceTarget{
					EntityIDs: []string{"light.salon"},
				},
				ServiceData: map[string]any{
					"brightness_pct": 80,
				},
			},
		},
		{
			name: "lock unlock con target device_id",
			call: ha.ServiceCall{
				Domain:  "lock",
				Service: "unlock",
				Target: ha.ServiceTarget{
					DeviceIDs: []string{"dev_lock_1"},
				},
			},
		},
		{
			name: "climate set_temperature",
			call: ha.ServiceCall{
				Domain:  "climate",
				Service: "set_temperature",
				Target: ha.ServiceTarget{
					EntityIDs: []string{"climate.termostato"},
				},
				ServiceData: map[string]any{
					"temperature": 21.5,
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := hatest.New(t, hatest.Options{})
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			conn, err := ha.Connect(ctx, srv.URL, "test-token")
			if err != nil {
				t.Fatalf("Connect error: %v", err)
			}
			defer conn.Close()

			res, err := conn.CallService(ctx, tt.call)
			if err != nil {
				t.Fatalf("CallService error: %v", err)
			}
			if res.ContextID == "" {
				t.Error("expected non-empty ContextID in ServiceResult")
			}

			cmds := srv.Received()
			var found bool
			for _, c := range cmds {
				if c["type"] == "call_service" && c["domain"] == tt.call.Domain && c["service"] == tt.call.Service {
					found = true
					break
				}
			}
			if !found {
				t.Fatalf("call_service command not received by server: %+v", cmds)
			}
		})
	}
}

func TestCallService_ReturnResponse(t *testing.T) {
	srv := hatest.New(t, hatest.Options{
		Results: map[string]any{
			"call_service": map[string]any{
				"context": map[string]any{"id": "ctx-123"},
				"response": map[string]any{
					"forecast": []any{
						map[string]any{"temperature": 22.0},
					},
				},
			},
		},
	})
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	conn, err := ha.Connect(ctx, srv.URL, "test-token")
	if err != nil {
		t.Fatalf("Connect error: %v", err)
	}
	defer conn.Close()

	call := ha.ServiceCall{
		Domain:         "weather",
		Service:        "get_forecasts",
		ReturnResponse: true,
	}

	res, err := conn.CallService(ctx, call)
	if err != nil {
		t.Fatalf("CallService error: %v", err)
	}
	if res.ContextID != "ctx-123" {
		t.Errorf("expected context id ctx-123, got %s", res.ContextID)
	}
	if len(res.Response) == 0 {
		t.Fatal("expected non-empty Response bytes")
	}

	var respMap map[string]any
	if err := json.Unmarshal(res.Response, &respMap); err != nil {
		t.Fatalf("unmarshal Response error: %v", err)
	}
	if _, ok := respMap["forecast"]; !ok {
		t.Error("expected forecast in response")
	}
}

func TestCallService_Errors(t *testing.T) {
	srv := hatest.New(t, hatest.Options{
		Errors: map[string]hatest.ErrorObj{
			"call_service": {
				Code:    "service_not_found",
				Message: "Service not found",
			},
		},
	})
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	conn, err := ha.Connect(ctx, srv.URL, "test-token")
	if err != nil {
		t.Fatalf("Connect error: %v", err)
	}
	defer conn.Close()

	call := ha.ServiceCall{
		Domain:  "light",
		Service: "invalid_service",
	}

	_, err = conn.CallService(ctx, call)
	if err == nil {
		t.Fatal("expected error from CallService, got nil")
	}
	var cmdErr *ha.CommandError
	if !errors.As(err, &cmdErr) {
		t.Fatalf("expected *ha.CommandError, got: %T (%v)", err, err)
	}
	if cmdErr.Code != "service_not_found" {
		t.Errorf("expected code service_not_found, got %s", cmdErr.Code)
	}
}

func TestCallService_InvalidInput(t *testing.T) {
	conn := &ha.Conn{}
	ctx := context.Background()

	t.Run("domain vacio", func(t *testing.T) {
		_, err := conn.CallService(ctx, ha.ServiceCall{Service: "turn_on"})
		if err == nil {
			t.Fatal("expected error for empty domain")
		}
	})

	t.Run("service vacio", func(t *testing.T) {
		_, err := conn.CallService(ctx, ha.ServiceCall{Domain: "light"})
		if err == nil {
			t.Fatal("expected error for empty service")
		}
	})
}

func TestRegistries_Parses(t *testing.T) {
	srv := hatest.New(t, hatest.Options{
		Results: map[string]any{
			"config/entity_registry/list": []any{
				map[string]any{
					"id":        "ent_1",
					"entity_id": "light.living",
					"name":      "Lámpara Principal",
					"device_id": "dev_1",
					"area_id":   "area_living",
					"aliases":   []string{"luz de pie"},
				},
			},
			"config/device_registry/list": []any{
				map[string]any{
					"id":      "dev_1",
					"name":    "Enchufe Zigbee",
					"area_id": "area_living",
				},
			},
			"config/area_registry/list": []any{
				map[string]any{
					"id":   "area_living",
					"name": "Living Room",
				},
			},
		},
	})
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	conn, err := ha.Connect(ctx, srv.URL, "test-token")
	if err != nil {
		t.Fatalf("Connect error: %v", err)
	}
	defer conn.Close()

	regs, err := conn.Registries(ctx)
	if err != nil {
		t.Fatalf("Registries error: %v", err)
	}

	ent, ok := regs.Entities["light.living"]
	if !ok {
		t.Fatal("expected light.living in Entities registry")
	}
	if *ent.Name != "Lámpara Principal" {
		t.Errorf("expected name 'Lámpara Principal', got %v", *ent.Name)
	}
	if len(ent.Aliases) != 1 || ent.Aliases[0] != "luz de pie" {
		t.Errorf("expected aliases ['luz de pie'], got %v", ent.Aliases)
	}

	dev, ok := regs.Devices["dev_1"]
	if !ok {
		t.Fatal("expected dev_1 in Devices registry")
	}
	if *dev.Name != "Enchufe Zigbee" {
		t.Errorf("expected dev name 'Enchufe Zigbee', got %v", *dev.Name)
	}

	area, ok := regs.Areas["area_living"]
	if !ok {
		t.Fatal("expected area_living in Areas registry")
	}
	if area.Name != "Living Room" {
		t.Errorf("expected area name 'Living Room', got %s", area.Name)
	}
}

func TestRegistries_AllOrNothing(t *testing.T) {
	srv := hatest.New(t, hatest.Options{
		Results: map[string]any{
			"config/entity_registry/list": []any{},
			"config/device_registry/list": []any{},
		},
		Errors: map[string]hatest.ErrorObj{
			"config/area_registry/list": {Code: "error", Message: "database locked"},
		},
	})
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	conn, err := ha.Connect(ctx, srv.URL, "test-token")
	if err != nil {
		t.Fatalf("Connect error: %v", err)
	}
	defer conn.Close()

	_, err = conn.Registries(ctx)
	if err == nil {
		t.Fatal("expected error when one registry fails, got nil")
	}
}

func TestEntityDetails_NullEntries(t *testing.T) {
	srv := hatest.New(t, hatest.Options{
		Results: map[string]any{
			"config/entity_registry/get_entries": map[string]any{
				"light.kitchen": map[string]any{
					"entity_id": "light.kitchen",
					"aliases":   []any{nil, "luz cocina", nil},
				},
				"light.hall": nil, // entrada nula
			},
		},
	})
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	conn, err := ha.Connect(ctx, srv.URL, "test-token")
	if err != nil {
		t.Fatalf("Connect error: %v", err)
	}
	defer conn.Close()

	details, err := conn.EntityDetails(ctx, []string{"light.kitchen", "light.hall"})
	if err != nil {
		t.Fatalf("EntityDetails error: %v", err)
	}

	kitchen, ok := details["light.kitchen"]
	if !ok {
		t.Fatal("expected light.kitchen details")
	}
	if len(kitchen.Aliases) != 1 || kitchen.Aliases[0] != "luz cocina" {
		t.Errorf("expected clean aliases without nulls, got: %v", kitchen.Aliases)
	}
}

func TestExposed_FailClosed(t *testing.T) {
	srv := hatest.New(t, hatest.Options{
		Results: map[string]any{
			"homeassistant/expose_entity/list": map[string]any{
				"exposed_entities": map[string]any{
					"conversation": map[string]any{
						"light.living":     true,
						"lock.front_door":  false,
						"cover.garage":     map[string]any{}, // objeto vacío, debe ser fail-closed (false)
						"climate.bedroom":  nil,               // nulo, fail-closed (false)
					},
				},
			},
		},
	})
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	conn, err := ha.Connect(ctx, srv.URL, "test-token")
	if err != nil {
		t.Fatalf("Connect error: %v", err)
	}
	defer conn.Close()

	exposed, err := conn.Exposed(ctx, "conversation")
	if err != nil {
		t.Fatalf("Exposed error: %v", err)
	}

	if !exposed["light.living"] {
		t.Error("expected light.living to be exposed (true)")
	}
	if exposed["lock.front_door"] {
		t.Error("expected lock.front_door to NOT be exposed (false)")
	}
	if exposed["cover.garage"] {
		t.Error("expected cover.garage to NOT be exposed (false)")
	}
	if exposed["climate.bedroom"] {
		t.Error("expected climate.bedroom to NOT be exposed (false)")
	}
	if exposed["sensor.unlisted"] {
		t.Error("expected unlisted entity to NOT be exposed (false)")
	}
}

func TestGetConfig_Units(t *testing.T) {
	srv := hatest.New(t, hatest.Options{
		HAVersion: "2026.10.1",
	})
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	conn, err := ha.Connect(ctx, srv.URL, "test-token")
	if err != nil {
		t.Fatalf("Connect error: %v", err)
	}
	defer conn.Close()

	cfg, err := conn.GetConfig(ctx)
	if err != nil {
		t.Fatalf("GetConfig error: %v", err)
	}

	if cfg.State != "RUNNING" {
		t.Errorf("expected state RUNNING, got %s", cfg.State)
	}
	if cfg.Version != "2026.10.1" {
		t.Errorf("expected version 2026.10.1, got %s", cfg.Version)
	}
	if cfg.UnitSystem.Temperature != "°C" {
		t.Errorf("expected temperature unit °C, got %s", cfg.UnitSystem.Temperature)
	}
}
