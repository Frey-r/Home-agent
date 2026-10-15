package entities

import (
	"math"
	"testing"
	"time"

	"github.com/Frey-r/Home-agent/contracts"
	"github.com/Frey-r/Home-agent/edge/internal/ha"
)

func strPtr(s string) *string {
	return &s
}

func TestAttr_Helpers(t *testing.T) {
	attrs := map[string]any{
		"valid_float":   23.5,
		"int_as_float":  42,
		"string_float":  "18.2",
		"invalid_str":   "not-a-number",
		"nan_val":       math.NaN(),
		"inf_val":       math.Inf(1),
		"valid_int":     100,
		"float_as_int":  55.0,
		"string_int":    "77",
		"valid_bool":    true,
		"bool_on":       "on",
		"bool_off":      "off",
		"valid_str":     "salón",
	}

	// FloatAttr
	if f, ok := FloatAttr(attrs, "valid_float"); !ok || f != 23.5 {
		t.Errorf("FloatAttr valid_float: got (%v, %v), want (23.5, true)", f, ok)
	}
	if f, ok := FloatAttr(attrs, "int_as_float"); !ok || f != 42.0 {
		t.Errorf("FloatAttr int_as_float: got (%v, %v), want (42.0, true)", f, ok)
	}
	if f, ok := FloatAttr(attrs, "string_float"); !ok || f != 18.2 {
		t.Errorf("FloatAttr string_float: got (%v, %v), want (18.2, true)", f, ok)
	}
	if _, ok := FloatAttr(attrs, "invalid_str"); ok {
		t.Errorf("FloatAttr invalid_str: expected false")
	}
	if _, ok := FloatAttr(attrs, "nan_val"); ok {
		t.Errorf("FloatAttr nan_val: expected false (NaN must be rejected)")
	}
	if _, ok := FloatAttr(attrs, "inf_val"); ok {
		t.Errorf("FloatAttr inf_val: expected false (Inf must be rejected)")
	}
	if _, ok := FloatAttr(attrs, "missing"); ok {
		t.Errorf("FloatAttr missing: expected false")
	}

	// IntAttr
	if i, ok := IntAttr(attrs, "valid_int"); !ok || i != 100 {
		t.Errorf("IntAttr valid_int: got (%v, %v), want (100, true)", i, ok)
	}
	if i, ok := IntAttr(attrs, "float_as_int"); !ok || i != 55 {
		t.Errorf("IntAttr float_as_int: got (%v, %v), want (55, true)", i, ok)
	}
	if i, ok := IntAttr(attrs, "string_int"); !ok || i != 77 {
		t.Errorf("IntAttr string_int: got (%v, %v), want (77, true)", i, ok)
	}
	if _, ok := IntAttr(attrs, "invalid_str"); ok {
		t.Errorf("IntAttr invalid_str: expected false")
	}

	// StringAttr
	if s, ok := StringAttr(attrs, "valid_str"); !ok || s != "salón" {
		t.Errorf("StringAttr valid_str: got (%v, %v), want ('salón', true)", s, ok)
	}
	if _, ok := StringAttr(attrs, "missing"); ok {
		t.Errorf("StringAttr missing: expected false")
	}

	// BoolAttr
	if b, ok := BoolAttr(attrs, "valid_bool"); !ok || !b {
		t.Errorf("BoolAttr valid_bool: got (%v, %v), want (true, true)", b, ok)
	}
	if b, ok := BoolAttr(attrs, "bool_on"); !ok || !b {
		t.Errorf("BoolAttr bool_on: got (%v, %v), want (true, true)", b, ok)
	}
	if b, ok := BoolAttr(attrs, "bool_off"); !ok || b {
		t.Errorf("BoolAttr bool_off: got (%v, %v), want (false, true)", b, ok)
	}
}

func TestResolveName(t *testing.T) {
	// Case 1: entity registry name takes precedence
	entry1 := &ha.EntityRegistryEntry{Name: strPtr("Luz Techo Custom")}
	name1 := ResolveName("light.lamp", map[string]any{"friendly_name": "Lámpara Salón"}, entry1, nil)
	if name1 != "Luz Techo Custom" {
		t.Errorf("ResolveName entry precedence: got %q, want 'Luz Techo Custom'", name1)
	}

	// Case 2: friendly_name fallback
	name2 := ResolveName("light.lamp", map[string]any{"friendly_name": "Lámpara Salón"}, nil, nil)
	if name2 != "Lámpara Salón" {
		t.Errorf("ResolveName friendly_name: got %q, want 'Lámpara Salón'", name2)
	}

	// Case 3: device name fallback
	dev := &ha.DeviceRegistryEntry{Name: strPtr("Philips Hue Hub Lamp")}
	name3 := ResolveName("light.lamp", map[string]any{}, nil, dev)
	if name3 != "Philips Hue Hub Lamp" {
		t.Errorf("ResolveName device name: got %q, want 'Philips Hue Hub Lamp'", name3)
	}

	// Case 4: entity_id fallback
	name4 := ResolveName("light.lamp", map[string]any{}, nil, nil)
	if name4 != "light.lamp" {
		t.Errorf("ResolveName entity_id: got %q, want 'light.lamp'", name4)
	}
}

func TestResolveAliases(t *testing.T) {
	entry := &ha.EntityRegistryEntry{
		Aliases: []string{"  luz de pie  ", "", "   ", "Luz De Pie", "Lámpara Principal", "otra luz"},
	}
	primaryName := "Lámpara Principal"

	aliases := ResolveAliases(entry, primaryName)

	expected := []string{"luz de pie", "otra luz"}
	if len(aliases) != len(expected) {
		t.Fatalf("ResolveAliases count: got %v, want %v", aliases, expected)
	}
	for i, a := range aliases {
		if a != expected[i] {
			t.Errorf("ResolveAliases[%d]: got %q, want %q", i, a, expected[i])
		}
	}
}

func TestResolveArea(t *testing.T) {
	regs := &ha.Registries{
		Areas: map[string]ha.AreaRegistryEntry{
			"area_salon":  {ID: "area_salon", Name: "Salón"},
			"area_cocina": {ID: "area_cocina", Name: "Cocina"},
		},
		Devices: map[string]ha.DeviceRegistryEntry{
			"dev_1": {ID: "dev_1", AreaID: strPtr("area_cocina")},
		},
	}

	// Direct area from entity
	entDirect := &ha.EntityRegistryEntry{AreaID: strPtr("area_salon")}
	a1 := ResolveArea(entDirect, regs)
	if a1 == nil || a1.ID != "area_salon" || a1.Name != "Salón" {
		t.Errorf("ResolveArea direct: got %+v", a1)
	}

	// Inherited area from device
	entFromDev := &ha.EntityRegistryEntry{DeviceID: strPtr("dev_1")}
	a2 := ResolveArea(entFromDev, regs)
	if a2 == nil || a2.ID != "area_cocina" || a2.Name != "Cocina" {
		t.Errorf("ResolveArea inherited: got %+v", a2)
	}

	// No area
	entNone := &ha.EntityRegistryEntry{}
	a3 := ResolveArea(entNone, regs)
	if a3 != nil {
		t.Errorf("ResolveArea none: expected nil, got %+v", a3)
	}
}

func TestAvailability(t *testing.T) {
	cases := []struct {
		rawState  string
		available bool
	}{
		{"on", true},
		{"off", true},
		{"open", true},
		{"locked", true},
		{"unavailable", false},
		{"unknown", false},
	}

	for _, tc := range cases {
		avail := ResolveAvailability(tc.rawState)
		if avail != tc.available {
			t.Errorf("ResolveAvailability(%q): got %v, want %v", tc.rawState, avail, tc.available)
		}
	}
}

func TestRisk_Table(t *testing.T) {
	cases := []struct {
		category string
		action   string
		expected contracts.Risk
	}{
		{"light", "set_power", contracts.RiskLow},
		{"light", "set_brightness", contracts.RiskLow},
		{"switch", "set_power", contracts.RiskLow},
		{"cover", "open", contracts.RiskMedium},
		{"cover", "close", contracts.RiskMedium},
		{"cover", "stop", contracts.RiskLow},
		{"cover", "set_position", contracts.RiskMedium},
		{"lock", "lock", contracts.RiskLow},
		{"lock", "unlock", contracts.RiskHigh},
		{"lock", "unlatch", contracts.RiskHigh},
		{"alarm_control_panel", "arm", contracts.RiskMedium},
		{"alarm_control_panel", "disarm", contracts.RiskHigh},
		{"climate", "set_temperature", contracts.RiskMedium},
		{"climate", "set_power", contracts.RiskLow},
	}

	for _, tc := range cases {
		risk := ActionRisk(tc.category, tc.action)
		if risk != tc.expected {
			t.Errorf("ActionRisk(%s, %s): got %s, want %s", tc.category, tc.action, risk, tc.expected)
		}
	}
}

func TestEntityRisk_IsMaxOfCommands(t *testing.T) {
	caps := []contracts.Capability{
		{Action: "lock", Risk: contracts.RiskLow},
		{Action: "unlock", Risk: contracts.RiskHigh},
	}
	r := EntityRisk(caps)
	if r != contracts.RiskHigh {
		t.Errorf("EntityRisk: got %s, want %s", r, contracts.RiskHigh)
	}
}

func TestRegistry_UnsupportedSkipped(t *testing.T) {
	raw := ha.State{
		EntityID: "camera.front_door",
		State:    "idle",
	}
	ent, err := Build(raw, nil, nil)
	if err != nil {
		t.Fatalf("Build unexpected error: %v", err)
	}
	if ent != nil {
		t.Errorf("Build unsupported domain: expected nil, got %+v", ent)
	}
}

func TestLight_Build(t *testing.T) {
	validator, err := contracts.NewValidator()
	if err != nil {
		t.Fatalf("contracts.NewValidator error: %v", err)
	}

	raw := ha.State{
		EntityID: "light.living_room",
		State:    "on",
		Attributes: map[string]any{
			"friendly_name":       "Luz Living",
			"brightness":          204, // 204/255 = 80%
			"color_temp_kelvin":   3000,
			"rgb_color":           []any{255, 128, 0},
			"supported_features":  32,
		},
		LastChanged: time.Now(),
		LastUpdated: time.Now(),
	}

	ent, err := Build(raw, nil, map[string]bool{"light.living_room": true})
	if err != nil {
		t.Fatalf("Build light error: %v", err)
	}
	if ent == nil {
		t.Fatal("expected non-nil entity for light")
	}

	if ent.ID != "ha:light.living_room" {
		t.Errorf("ID: got %q, want ha:light.living_room", ent.ID)
	}
	if ent.Category != "light" {
		t.Errorf("Category: got %q, want light", ent.Category)
	}
	if !ent.Exposed {
		t.Errorf("Exposed: expected true")
	}
	if ent.Available == nil || !*ent.Available {
		t.Errorf("Available: expected true")
	}

	// Validar estado normalizado
	stateMap := ent.State
	if stateMap["power"] != "on" {
		t.Errorf("power: got %v, want 'on'", stateMap["power"])
	}
	if bPct, ok := stateMap["brightness_pct"].(int); !ok || bPct != 80 {
		t.Errorf("brightness_pct: got %v, want 80", stateMap["brightness_pct"])
	}

	// Validar contra JSON Schema
	if err := validator.Validate(ent); err != nil {
		t.Fatalf("schema validation failed for light: %v", err)
	}
}

func TestSwitch_Build(t *testing.T) {
	validator, err := contracts.NewValidator()
	if err != nil {
		t.Fatalf("contracts.NewValidator error: %v", err)
	}

	raw := ha.State{
		EntityID: "switch.coffee_maker",
		State:    "off",
		Attributes: map[string]any{
			"friendly_name": "Cafetera",
		},
	}

	ent, err := Build(raw, nil, nil)
	if err != nil {
		t.Fatalf("Build switch error: %v", err)
	}
	if ent == nil {
		t.Fatal("expected non-nil entity for switch")
	}

	if ent.Category != "switch" {
		t.Errorf("Category: got %q, want switch", ent.Category)
	}

	stateMap := ent.State
	if stateMap["power"] != "off" {
		t.Errorf("power: got %v, want 'off'", stateMap["power"])
	}

	if err := validator.Validate(ent); err != nil {
		t.Fatalf("schema validation failed for switch: %v", err)
	}
}

func TestCover_Build(t *testing.T) {
	validator, err := contracts.NewValidator()
	if err != nil {
		t.Fatalf("contracts.NewValidator error: %v", err)
	}

	raw := ha.State{
		EntityID: "cover.garage",
		State:    "open",
		Attributes: map[string]any{
			"friendly_name":    "Portón Garage",
			"current_position": 100,
		},
	}

	ent, err := Build(raw, nil, nil)
	if err != nil {
		t.Fatalf("Build cover error: %v", err)
	}
	if ent == nil {
		t.Fatal("expected non-nil entity for cover")
	}

	if ent.Category != "cover" {
		t.Errorf("Category: got %q, want cover", ent.Category)
	}

	stateMap := ent.State
	if stateMap["status"] != "open" {
		t.Errorf("status: got %v, want 'open'", stateMap["status"])
	}
	if pos, ok := stateMap["position_pct"].(int); !ok || pos != 100 {
		t.Errorf("position_pct: got %v, want 100", stateMap["position_pct"])
	}

	if err := validator.Validate(ent); err != nil {
		t.Fatalf("schema validation failed for cover: %v", err)
	}
}

func TestLock_Build(t *testing.T) {
	validator, err := contracts.NewValidator()
	if err != nil {
		t.Fatalf("contracts.NewValidator error: %v", err)
	}

	raw := ha.State{
		EntityID: "lock.front_door",
		State:    "locked",
		Attributes: map[string]any{
			"friendly_name": "Cerradura Principal",
		},
	}

	ent, err := Build(raw, nil, nil)
	if err != nil {
		t.Fatalf("Build lock error: %v", err)
	}
	if ent == nil {
		t.Fatal("expected non-nil entity for lock")
	}

	if ent.Category != "lock" {
		t.Errorf("Category: got %q, want lock", ent.Category)
	}

	stateMap := ent.State
	if stateMap["status"] != "locked" {
		t.Errorf("status: got %v, want 'locked'", stateMap["status"])
	}

	if err := validator.Validate(ent); err != nil {
		t.Fatalf("schema validation failed for lock: %v", err)
	}
}

func TestAlarm_Build(t *testing.T) {
	validator, err := contracts.NewValidator()
	if err != nil {
		t.Fatalf("contracts.NewValidator error: %v", err)
	}

	raw := ha.State{
		EntityID: "alarm_control_panel.home_alarm",
		State:    "armed_home",
		Attributes: map[string]any{
			"friendly_name": "Alarma Hogar",
		},
	}

	ent, err := Build(raw, nil, nil)
	if err != nil {
		t.Fatalf("Build alarm error: %v", err)
	}
	if ent == nil {
		t.Fatal("expected non-nil entity for alarm")
	}

	if ent.Category != "alarm_control_panel" {
		t.Errorf("Category: got %q, want alarm_control_panel", ent.Category)
	}

	stateMap := ent.State
	if stateMap["status"] != "armed_home" {
		t.Errorf("status: got %v, want 'armed_home'", stateMap["status"])
	}

	if err := validator.Validate(ent); err != nil {
		t.Fatalf("schema validation failed for alarm: %v", err)
	}
}

func TestClimate_Build(t *testing.T) {
	validator, err := contracts.NewValidator()
	if err != nil {
		t.Fatalf("contracts.NewValidator error: %v", err)
	}

	raw := ha.State{
		EntityID: "climate.thermostat",
		State:    "heat",
		Attributes: map[string]any{
			"friendly_name":       "Termostato",
			"temperature":         21.5,
			"current_temperature": 19.8,
			"current_humidity":    45,
			"hvac_action":         "heating",
			"preset_mode":         "comfort",
		},
	}

	ent, err := Build(raw, nil, nil)
	if err != nil {
		t.Fatalf("Build climate error: %v", err)
	}
	if ent == nil {
		t.Fatal("expected non-nil entity for climate")
	}

	if ent.Category != "climate" {
		t.Errorf("Category: got %q, want climate", ent.Category)
	}

	stateMap := ent.State
	if stateMap["hvac_mode"] != "heat" {
		t.Errorf("hvac_mode: got %v, want 'heat'", stateMap["hvac_mode"])
	}
	if stateMap["target_temp"] != 21.5 {
		t.Errorf("target_temp: got %v, want 21.5", stateMap["target_temp"])
	}
	if stateMap["current_temp"] != 19.8 {
		t.Errorf("current_temp: got %v, want 19.8", stateMap["current_temp"])
	}

	if err := validator.Validate(ent); err != nil {
		t.Fatalf("schema validation failed for climate: %v", err)
	}
}

func TestCanonicalActions_MatchSchema(t *testing.T) {
	// Comprobar que todas las capacidades de las 6 entidades construidas
	// tienen esquemas de parámetros válidos y no contienen oneOf/anyOf/allOf
	states := []ha.State{
		{EntityID: "light.test", State: "on", Attributes: map[string]any{"brightness": 128}},
		{EntityID: "switch.test", State: "on"},
		{EntityID: "cover.test", State: "open", Attributes: map[string]any{"current_position": 50}},
		{EntityID: "lock.test", State: "locked"},
		{EntityID: "alarm_control_panel.test", State: "disarmed"},
		{EntityID: "climate.test", State: "heat", Attributes: map[string]any{"temperature": 22.0}},
	}

	for _, s := range states {
		ent, err := Build(s, nil, nil)
		if err != nil {
			t.Fatalf("Build %s error: %v", s.EntityID, err)
		}
		if ent == nil {
			t.Fatalf("Build %s returned nil", s.EntityID)
		}
		for _, cap := range ent.Capabilities {
			if cap.Params != nil {
				// Debe tener "type": "object" y "additionalProperties": false
				typ, _ := cap.Params["type"].(string)
				if typ != "object" {
					t.Errorf("action %s params type must be 'object', got %v", cap.Action, typ)
				}
				addProp, ok := cap.Params["additionalProperties"].(bool)
				if !ok || addProp {
					t.Errorf("action %s params must have additionalProperties: false", cap.Action)
				}
				if _, hasOneOf := cap.Params["oneOf"]; hasOneOf {
					t.Errorf("action %s params contains forbidden oneOf", cap.Action)
				}
				if _, hasAnyOf := cap.Params["anyOf"]; hasAnyOf {
					t.Errorf("action %s params contains forbidden anyOf", cap.Action)
				}
				if _, hasAllOf := cap.Params["allOf"]; hasAllOf {
					t.Errorf("action %s params contains forbidden allOf", cap.Action)
				}
			}
		}
	}
}
