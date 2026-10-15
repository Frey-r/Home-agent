package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Frey-r/Home-agent/edge/internal/ha"
	"github.com/Frey-r/Home-agent/edge/internal/ha/hatest"
)

func TestCapture_RefusesNonDemo(t *testing.T) {
	srv := hatest.New(t, hatest.Options{
		Results: map[string]any{
			"get_config": map[string]any{
				"state":       "RUNNING",
				"version":     "2026.10.1",
				"location_name": "Producción Hogar Real", // No es Casa Dev
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

	tmpDir := t.TempDir()
	err = runCapture(ctx, conn, tmpDir)
	if err == nil {
		t.Fatal("expected runCapture to refuse non-demo instance, got nil")
	}
}

func TestCapture_RedactsSecrets(t *testing.T) {
	srv := hatest.New(t, hatest.Options{
		Results: map[string]any{
			"get_config": map[string]any{
				"state":         "RUNNING",
				"version":       "2026.10.1",
				"location_name": "Casa Dev",
			},
			"get_states": []map[string]any{
				{
					"entity_id": "camera.living", // o cualquier entidad con token o foto
					"state":     "idle",
					"attributes": map[string]any{
						"access_token":   "SECRET_TOKEN_12345",
						"entity_picture": "/api/image_proxy/camera.living?token=SECRET_TOKEN",
					},
				},
				{
					"entity_id": "light.salon",
					"state":     "on",
					"attributes": map[string]any{
						"friendly_name": "Luz Salón",
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

	tmpDir := t.TempDir()
	if err := runCapture(ctx, conn, tmpDir); err != nil {
		t.Fatalf("runCapture error: %v", err)
	}

	statesPath := filepath.Join(tmpDir, "states.json")
	data, err := os.ReadFile(statesPath)
	if err != nil {
		t.Fatalf("ReadFile states.json error: %v", err)
	}

	var states []map[string]any
	if err := json.Unmarshal(data, &states); err != nil {
		t.Fatalf("unmarshal states.json: %v", err)
	}

	for _, s := range states {
		attrs, _ := s["attributes"].(map[string]any)
		if tok, ok := attrs["access_token"].(string); ok && tok == "SECRET_TOKEN_12345" {
			t.Errorf("found unredacted access_token: %s", tok)
		}
		if pic, ok := attrs["entity_picture"].(string); ok && pic != "" {
			t.Errorf("found unredacted entity_picture: %s", pic)
		}
	}
}

func TestCapture_DropsUnsupportedDomains(t *testing.T) {
	srv := hatest.New(t, hatest.Options{
		Results: map[string]any{
			"get_config": map[string]any{
				"state":         "RUNNING",
				"version":       "2026.10.1",
				"location_name": "Casa Dev",
			},
			"get_states": []map[string]any{
				{
					"entity_id":  "person.pedro",
					"state":      "home",
					"attributes": map[string]any{"latitude": -33.45, "longitude": -70.66},
				},
				{
					"entity_id":  "device_tracker.movil",
					"state":      "home",
					"attributes": map[string]any{"gps_accuracy": 10},
				},
				{
					"entity_id":  "sun.sun",
					"state":      "above_horizon",
					"attributes": map[string]any{"elevation": 45.2},
				},
				{
					"entity_id":  "light.salon",
					"state":      "on",
					"attributes": map[string]any{"brightness": 200},
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

	tmpDir := t.TempDir()
	if err := runCapture(ctx, conn, tmpDir); err != nil {
		t.Fatalf("runCapture error: %v", err)
	}

	statesPath := filepath.Join(tmpDir, "states.json")
	data, err := os.ReadFile(statesPath)
	if err != nil {
		t.Fatalf("ReadFile states.json error: %v", err)
	}

	var states []map[string]any
	if err := json.Unmarshal(data, &states); err != nil {
		t.Fatalf("unmarshal states.json: %v", err)
	}

	for _, s := range states {
		id, _ := s["entity_id"].(string)
		if id == "person.pedro" || id == "device_tracker.movil" || id == "sun.sun" {
			t.Errorf("unsupported domain entity %s was not dropped", id)
		}
	}

	if len(states) != 1 || states[0]["entity_id"] != "light.salon" {
		t.Errorf("expected only light.salon in filtered states, got %d entities", len(states))
	}
}
