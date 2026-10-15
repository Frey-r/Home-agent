package entities

import (
	"testing"

	"github.com/Frey-r/Home-agent/contracts"
	"github.com/Frey-r/Home-agent/edge/internal/ha"
)

func TestMediaPlayer_Build(t *testing.T) {
	validator, err := contracts.NewValidator()
	if err != nil {
		t.Fatalf("contracts.NewValidator error: %v", err)
	}

	raw := ha.State{
		EntityID: "media_player.living_tv",
		State:    "playing",
		Attributes: map[string]any{
			"friendly_name": "TV Salón",
			"volume_level":  0.45,
			"is_volume_muted": false,
			"source":        "HDMI 1",
			"media_title":   "Película",
			"media_artist":  "Director",
		},
	}

	ent, err := Build(raw, nil, nil)
	if err != nil {
		t.Fatalf("Build media_player error: %v", err)
	}
	if ent == nil {
		t.Fatal("expected non-nil entity for media_player")
	}

	if ent.Category != "media_player" {
		t.Errorf("Category: got %q, want media_player", ent.Category)
	}

	stateMap := ent.State
	if stateMap["status"] != "playing" {
		t.Errorf("status: got %v, want 'playing'", stateMap["status"])
	}
	if stateMap["volume_pct"] != 45 {
		t.Errorf("volume_pct: got %v, want 45", stateMap["volume_pct"])
	}

	if err := validator.Validate(ent); err != nil {
		t.Fatalf("schema validation failed for media_player: %v", err)
	}
}

func TestSensor_Build(t *testing.T) {
	validator, err := contracts.NewValidator()
	if err != nil {
		t.Fatalf("contracts.NewValidator error: %v", err)
	}

	raw := ha.State{
		EntityID: "sensor.living_temperature",
		State:    "21.8",
		Attributes: map[string]any{
			"friendly_name":       "Temperatura Salón",
			"unit_of_measurement": "°C",
			"device_class":        "temperature",
		},
	}

	ent, err := Build(raw, nil, nil)
	if err != nil {
		t.Fatalf("Build sensor error: %v", err)
	}
	if ent == nil {
		t.Fatal("expected non-nil entity for sensor")
	}

	if ent.Category != "sensor" {
		t.Errorf("Category: got %q, want sensor", ent.Category)
	}
	if len(ent.Capabilities) != 0 {
		t.Errorf("sensor must not have capabilities, got %d", len(ent.Capabilities))
	}

	stateMap := ent.State
	if stateMap["value"] != 21.8 {
		t.Errorf("value: got %v, want 21.8", stateMap["value"])
	}
	if stateMap["unit"] != "°C" {
		t.Errorf("unit: got %v, want '°C'", stateMap["unit"])
	}

	if err := validator.Validate(ent); err != nil {
		t.Fatalf("schema validation failed for sensor: %v", err)
	}
}

func TestBinarySensor_Build(t *testing.T) {
	validator, err := contracts.NewValidator()
	if err != nil {
		t.Fatalf("contracts.NewValidator error: %v", err)
	}

	raw := ha.State{
		EntityID: "binary_sensor.front_motion",
		State:    "on",
		Attributes: map[string]any{
			"friendly_name": "Movimiento Entrada",
			"device_class":  "motion",
		},
	}

	ent, err := Build(raw, nil, nil)
	if err != nil {
		t.Fatalf("Build binary_sensor error: %v", err)
	}
	if ent == nil {
		t.Fatal("expected non-nil entity for binary_sensor")
	}

	if ent.Category != "binary_sensor" {
		t.Errorf("Category: got %q, want binary_sensor", ent.Category)
	}
	if len(ent.Capabilities) != 0 {
		t.Errorf("binary_sensor must not have capabilities, got %d", len(ent.Capabilities))
	}

	stateMap := ent.State
	if stateMap["active"] != true {
		t.Errorf("active: got %v, want true", stateMap["active"])
	}

	if err := validator.Validate(ent); err != nil {
		t.Fatalf("schema validation failed for binary_sensor: %v", err)
	}
}

func TestScene_Build(t *testing.T) {
	validator, err := contracts.NewValidator()
	if err != nil {
		t.Fatalf("contracts.NewValidator error: %v", err)
	}

	raw := ha.State{
		EntityID: "scene.modo_noche",
		State:    "2026-10-15T22:00:00.000Z",
		Attributes: map[string]any{
			"friendly_name": "Modo Noche",
			"entity_id":     []any{"light.living", "light.cocina"},
		},
	}

	ent, err := Build(raw, nil, nil)
	if err != nil {
		t.Fatalf("Build scene error: %v", err)
	}
	if ent == nil {
		t.Fatal("expected non-nil entity for scene")
	}

	if ent.Category != "scene" {
		t.Errorf("Category: got %q, want scene", ent.Category)
	}

	if err := validator.Validate(ent); err != nil {
		t.Fatalf("schema validation failed for scene: %v", err)
	}
}

func TestScene_RiskFromMembers(t *testing.T) {
	// Escena con solo luces -> RiskLow
	sceneLow := ha.State{
		EntityID: "scene.luces_relax",
		State:    "scening",
		Attributes: map[string]any{
			"friendly_name": "Luces Relax",
			"entity_id":     []any{"light.l1", "light.l2"},
		},
	}
	entLow, err := Build(sceneLow, nil, nil)
	if err != nil || entLow == nil {
		t.Fatalf("Build sceneLow error: %v", err)
	}
	if entLow.Risk != contracts.RiskLow {
		t.Errorf("scene with only lights risk: got %s, want %s", entLow.Risk, contracts.RiskLow)
	}

	// Escena que incluye una cerradura -> RiskHigh (ADR-0007 Decisión 5)
	sceneHigh := ha.State{
		EntityID: "scene.cerrar_todo",
		State:    "scening",
		Attributes: map[string]any{
			"friendly_name": "Cerrar Todo",
			"entity_id":     []any{"light.l1", "lock.front_door"},
		},
	}
	entHigh, err := Build(sceneHigh, nil, nil)
	if err != nil || entHigh == nil {
		t.Fatalf("Build sceneHigh error: %v", err)
	}
	if entHigh.Risk != contracts.RiskHigh {
		t.Errorf("scene with lock risk: got %s, want %s", entHigh.Risk, contracts.RiskHigh)
	}
}

func TestScript_Build(t *testing.T) {
	validator, err := contracts.NewValidator()
	if err != nil {
		t.Fatalf("contracts.NewValidator error: %v", err)
	}

	raw := ha.State{
		EntityID: "script.saludo",
		State:    "off",
		Attributes: map[string]any{
			"friendly_name":  "Saludo",
			"last_triggered": "2026-10-15T10:00:00.000Z",
		},
	}

	ent, err := Build(raw, nil, nil)
	if err != nil {
		t.Fatalf("Build script error: %v", err)
	}
	if ent == nil {
		t.Fatal("expected non-nil entity for script")
	}

	if ent.Category != "script" {
		t.Errorf("Category: got %q, want script", ent.Category)
	}

	stateMap := ent.State
	if stateMap["running"] != false {
		t.Errorf("running: got %v, want false", stateMap["running"])
	}

	if err := validator.Validate(ent); err != nil {
		t.Fatalf("schema validation failed for script: %v", err)
	}
}

func TestTodo_Build(t *testing.T) {
	validator, err := contracts.NewValidator()
	if err != nil {
		t.Fatalf("contracts.NewValidator error: %v", err)
	}

	raw := ha.State{
		EntityID: "todo.compras",
		State:    "3",
		Attributes: map[string]any{
			"friendly_name": "Lista de Compras",
		},
	}

	ent, err := Build(raw, nil, nil)
	if err != nil {
		t.Fatalf("Build todo error: %v", err)
	}
	if ent == nil {
		t.Fatal("expected non-nil entity for todo")
	}

	if ent.Category != "todo" {
		t.Errorf("Category: got %q, want todo", ent.Category)
	}

	stateMap := ent.State
	if stateMap["open_items"] != 3 {
		t.Errorf("open_items: got %v, want 3", stateMap["open_items"])
	}

	if err := validator.Validate(ent); err != nil {
		t.Fatalf("schema validation failed for todo: %v", err)
	}
}

func TestWeather_Build(t *testing.T) {
	validator, err := contracts.NewValidator()
	if err != nil {
		t.Fatalf("contracts.NewValidator error: %v", err)
	}

	raw := ha.State{
		EntityID: "weather.demo",
		State:    "sunny",
		Attributes: map[string]any{
			"friendly_name":       "Pronóstico Casa",
			"temperature":         24.5,
			"temperature_unit":    "°C",
			"humidity":            50,
			"wind_speed":          12.0,
			"wind_speed_unit":     "km/h",
			"pressure":            1013,
			"pressure_unit":       "hPa",
		},
	}

	ent, err := Build(raw, nil, nil)
	if err != nil {
		t.Fatalf("Build weather error: %v", err)
	}
	if ent == nil {
		t.Fatal("expected non-nil entity for weather")
	}

	if ent.Category != "weather" {
		t.Errorf("Category: got %q, want weather", ent.Category)
	}

	stateMap := ent.State
	if stateMap["condition"] != "sunny" {
		t.Errorf("condition: got %v, want 'sunny'", stateMap["condition"])
	}
	if stateMap["temperature"] != 24.5 {
		t.Errorf("temperature: got %v, want 24.5", stateMap["temperature"])
	}

	if err := validator.Validate(ent); err != nil {
		t.Fatalf("schema validation failed for weather: %v", err)
	}
}
