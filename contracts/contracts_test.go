package contracts_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/Frey-r/Home-agent/contracts"
)

func TestSchemas_Compile(t *testing.T) {
	v, err := contracts.NewValidator()
	if err != nil {
		t.Fatalf("NewValidator() failed to compile schemas: %v", err)
	}
	if v == nil {
		t.Fatal("NewValidator() returned nil validator")
	}
}

func TestEntitySchema_ValidFixtures(t *testing.T) {
	v, err := contracts.NewValidator()
	if err != nil {
		t.Fatalf("NewValidator() error: %v", err)
	}

	fixtures := map[string]string{
		"light": `{
			"id": "ha:light.salon",
			"source": {"kind": "ha", "entity_id": "light.salon", "device_id": "dev1"},
			"name": "Luz Salón",
			"aliases": ["luz techo", "lámpara"],
			"category": "light",
			"device_class": null,
			"area": {"id": "salon", "name": "Salón"},
			"exposed": true,
			"available": true,
			"risk": "low",
			"last_changed": "2026-10-15T10:00:00Z",
			"state": {"power": "on", "brightness_pct": 80, "color_temp_k": 2700, "rgb": [255, 200, 150]},
			"capabilities": [
				{
					"action": "set_power",
					"kind": "command",
					"risk": "low",
					"requires_code": false,
					"params": {
						"type": "object",
						"properties": {
							"state": {"type": "string", "enum": ["on", "off"]}
						},
						"required": ["state"],
						"additionalProperties": false
					}
				},
				{
					"action": "set_brightness",
					"kind": "command",
					"risk": "low",
					"requires_code": false,
					"params": {
						"type": "object",
						"properties": {
							"brightness_pct": {"type": "integer", "minimum": 1, "maximum": 100}
						},
						"required": ["brightness_pct"],
						"additionalProperties": false
					}
				}
			]
		}`,
		"switch": `{
			"id": "ha:switch.calentador",
			"source": {"kind": "ha", "entity_id": "switch.calentador"},
			"name": "Calentador",
			"aliases": [],
			"category": "switch",
			"exposed": false,
			"risk": "medium",
			"state": {"power": "off"},
			"capabilities": [
				{
					"action": "set_power",
					"kind": "command",
					"risk": "medium",
					"requires_code": false,
					"params": {
						"type": "object",
						"properties": {"state": {"type": "string", "enum": ["on", "off"]}},
						"required": ["state"],
						"additionalProperties": false
					}
				}
			]
		}`,
		"climate": `{
			"id": "ha:climate.termostato",
			"source": {"kind": "ha", "entity_id": "climate.termostato"},
			"name": "Termostato",
			"aliases": ["aire"],
			"category": "climate",
			"exposed": true,
			"risk": "medium",
			"state": {
				"power": "on",
				"hvac_mode": "heat",
				"target_temp": 21.5,
				"current_temp": 19.8,
				"temp_unit": "C"
			},
			"capabilities": [
				{
					"action": "set_temperature",
					"kind": "command",
					"risk": "medium",
					"requires_code": false,
					"params": {
						"type": "object",
						"properties": {"temperature": {"type": "number", "minimum": 10, "maximum": 30}},
						"required": ["temperature"],
						"additionalProperties": false
					}
				}
			]
		}`,
		"cover": `{
			"id": "ha:cover.persiana_salon",
			"source": {"kind": "ha", "entity_id": "cover.persiana_salon"},
			"name": "Persiana Salón",
			"aliases": [],
			"category": "cover",
			"device_class": "shutter",
			"exposed": true,
			"risk": "medium",
			"state": {"status": "open", "position_pct": 100},
			"capabilities": [
				{
					"action": "open",
					"kind": "command",
					"risk": "medium",
					"requires_code": false,
					"params": {"type": "object", "properties": {}, "additionalProperties": false}
				},
				{
					"action": "stop",
					"kind": "command",
					"risk": "low",
					"requires_code": false,
					"params": {"type": "object", "properties": {}, "additionalProperties": false}
				}
			]
		}`,
		"lock": `{
			"id": "ha:lock.puerta_principal",
			"source": {"kind": "ha", "entity_id": "lock.puerta_principal"},
			"name": "Puerta Principal",
			"aliases": ["cerradura"],
			"category": "lock",
			"exposed": true,
			"risk": "critical",
			"state": {"status": "locked"},
			"capabilities": [
				{
					"action": "unlock",
					"kind": "command",
					"risk": "critical",
					"requires_code": true,
					"params": {"type": "object", "properties": {}, "additionalProperties": false}
				},
				{
					"action": "lock",
					"kind": "command",
					"risk": "medium",
					"requires_code": false,
					"params": {"type": "object", "properties": {}, "additionalProperties": false}
				}
			]
		}`,
		"alarm_control_panel": `{
			"id": "ha:alarm_control_panel.alarma_casa",
			"source": {"kind": "ha", "entity_id": "alarm_control_panel.alarma_casa"},
			"name": "Alarma Hogar",
			"aliases": [],
			"category": "alarm_control_panel",
			"exposed": true,
			"risk": "critical",
			"state": {"status": "armed_home"},
			"capabilities": [
				{
					"action": "disarm",
					"kind": "command",
					"risk": "critical",
					"requires_code": true,
					"params": {"type": "object", "properties": {}, "additionalProperties": false}
				},
				{
					"action": "arm",
					"kind": "command",
					"risk": "high",
					"requires_code": false,
					"params": {
						"type": "object",
						"properties": {"mode": {"type": "string", "enum": ["home", "away"]}},
						"required": ["mode"],
						"additionalProperties": false
					}
				}
			]
		}`,
		"media_player": `{
			"id": "ha:media_player.tv_living",
			"source": {"kind": "ha", "entity_id": "media_player.tv_living"},
			"name": "TV Living",
			"aliases": ["tele"],
			"category": "media_player",
			"exposed": true,
			"risk": "low",
			"state": {"power": "on", "status": "playing", "volume_pct": 35, "muted": false, "media_title": "Noticias"},
			"capabilities": [
				{
					"action": "playback",
					"kind": "command",
					"risk": "low",
					"requires_code": false,
					"params": {
						"type": "object",
						"properties": {"command": {"type": "string", "enum": ["play", "pause", "stop"]}},
						"required": ["command"],
						"additionalProperties": false
					}
				}
			]
		}`,
		"sensor": `{
			"id": "ha:sensor.temperatura_exterior",
			"source": {"kind": "ha", "entity_id": "sensor.temperatura_exterior"},
			"name": "Temperatura Exterior",
			"aliases": [],
			"category": "sensor",
			"device_class": "temperature",
			"exposed": true,
			"risk": "low",
			"state": {"value": 18.4, "unit": "°C"},
			"capabilities": []
		}`,
		"binary_sensor": `{
			"id": "ha:binary_sensor.puerta_contacto",
			"source": {"kind": "ha", "entity_id": "binary_sensor.puerta_contacto"},
			"name": "Contacto Puerta",
			"aliases": [],
			"category": "binary_sensor",
			"device_class": "door",
			"exposed": true,
			"risk": "low",
			"state": {"active": false},
			"capabilities": []
		}`,
		"scene": `{
			"id": "ha:scene.modo_noche",
			"source": {"kind": "ha", "entity_id": "scene.modo_noche"},
			"name": "Modo Noche",
			"aliases": ["a dormir"],
			"category": "scene",
			"exposed": true,
			"risk": "critical",
			"state": {"last_activated": "2026-10-14T23:30:00Z", "members": ["ha:light.salon", "ha:lock.puerta_principal"]},
			"capabilities": [
				{
					"action": "activate",
					"kind": "command",
					"risk": "critical",
					"requires_code": false,
					"params": {"type": "object", "properties": {}, "additionalProperties": false}
				}
			]
		}`,
		"script": `{
			"id": "ha:script.saludo",
			"source": {"kind": "ha", "entity_id": "script.saludo"},
			"name": "Script Saludo",
			"aliases": [],
			"category": "script",
			"exposed": true,
			"risk": "medium",
			"state": {"running": false},
			"capabilities": [
				{
					"action": "run",
					"kind": "command",
					"risk": "medium",
					"requires_code": false,
					"params": {"type": "object", "properties": {}, "additionalProperties": false}
				}
			]
		}`,
		"todo": `{
			"id": "ha:todo.compras",
			"source": {"kind": "ha", "entity_id": "todo.compras"},
			"name": "Lista de Compras",
			"aliases": ["supermercado"],
			"category": "todo",
			"exposed": true,
			"risk": "low",
			"state": {"open_items": 4},
			"capabilities": [
				{
					"action": "add_item",
					"kind": "command",
					"risk": "low",
					"requires_code": false,
					"params": {
						"type": "object",
						"properties": {"item": {"type": "string", "minLength": 1, "maxLength": 255}},
						"required": ["item"],
						"additionalProperties": false
					}
				},
				{
					"action": "list_items",
					"kind": "query",
					"risk": "low",
					"requires_code": false,
					"params": {"type": "object", "properties": {}, "additionalProperties": false}
				}
			]
		}`,
		"weather": `{
			"id": "ha:weather.casa",
			"source": {"kind": "ha", "entity_id": "weather.casa"},
			"name": "Pronóstico Hogar",
			"aliases": ["el tiempo"],
			"category": "weather",
			"exposed": true,
			"risk": "low",
			"state": {"condition": "sunny", "temperature": 22.0, "temp_unit": "°C", "humidity": 45},
			"capabilities": [
				{
					"action": "get_forecast",
					"kind": "query",
					"risk": "low",
					"requires_code": false,
					"params": {
						"type": "object",
						"properties": {"type": {"type": "string", "enum": ["daily", "hourly"]}},
						"required": ["type"],
						"additionalProperties": false
					}
				}
			]
		}`,
		"unavailable_with_empty_state": `{
			"id": "ha:light.fuera_de_linea",
			"source": {"kind": "ha", "entity_id": "light.fuera_de_linea"},
			"name": "Luz Desconectada",
			"aliases": [],
			"category": "light",
			"exposed": true,
			"available": false,
			"risk": "low",
			"state": {},
			"capabilities": [
				{
					"action": "set_power",
					"kind": "command",
					"risk": "low",
					"requires_code": false,
					"params": {
						"type": "object",
						"properties": {"state": {"type": "string", "enum": ["on", "off"]}},
						"required": ["state"],
						"additionalProperties": false
					}
				}
			]
		}`,
	}

	for category, jsonDoc := range fixtures {
		t.Run(category, func(t *testing.T) {
			if err := v.ValidateEntity([]byte(jsonDoc)); err != nil {
				t.Fatalf("category %s expected valid but got: %v", category, err)
			}
		})
	}
}

func TestEntitySchema_InvalidFixtures(t *testing.T) {
	v, err := contracts.NewValidator()
	if err != nil {
		t.Fatalf("NewValidator() error: %v", err)
	}

	tests := []struct {
		name        string
		jsonDoc     string
		errContains string
	}{
		{
			name: "id sin ha: prefix",
			jsonDoc: `{
				"id": "light.salon",
				"source": {"kind": "ha", "entity_id": "light.salon"},
				"name": "Luz", "aliases": [], "category": "light", "exposed": true, "risk": "low",
				"state": {"power": "on"}, "capabilities": []
			}`,
			errContains: "id",
		},
		{
			name: "riesgo invalido",
			jsonDoc: `{
				"id": "ha:light.salon",
				"source": {"kind": "ha", "entity_id": "light.salon"},
				"name": "Luz", "aliases": [], "category": "light", "exposed": true, "risk": "extreme",
				"state": {"power": "on"}, "capabilities": []
			}`,
			errContains: "risk",
		},
		{
			name: "luz con accion de cerradura (unlock)",
			jsonDoc: `{
				"id": "ha:light.salon",
				"source": {"kind": "ha", "entity_id": "light.salon"},
				"name": "Luz", "aliases": [], "category": "light", "exposed": true, "risk": "low",
				"state": {"power": "on"},
				"capabilities": [
					{
						"action": "unlock",
						"kind": "command",
						"risk": "critical",
						"requires_code": false,
						"params": {"type": "object", "properties": {}, "additionalProperties": false}
					}
				]
			}`,
			errContains: "capabilities",
		},
		{
			name: "params con oneOf prohibido",
			jsonDoc: `{
				"id": "ha:switch.enchufe",
				"source": {"kind": "ha", "entity_id": "switch.enchufe"},
				"name": "Enchufe", "aliases": [], "category": "switch", "exposed": true, "risk": "medium",
				"state": {"power": "on"},
				"capabilities": [
					{
						"action": "set_power",
						"kind": "command",
						"risk": "medium",
						"requires_code": false,
						"params": {
							"type": "object",
							"oneOf": [{"properties": {"a": {"type": "string"}}}],
							"additionalProperties": false
						}
					}
				]
			}`,
			errContains: "params",
		},
		{
			name: "params con additionalProperties true",
			jsonDoc: `{
				"id": "ha:switch.enchufe",
				"source": {"kind": "ha", "entity_id": "switch.enchufe"},
				"name": "Enchufe", "aliases": [], "category": "switch", "exposed": true, "risk": "medium",
				"state": {"power": "on"},
				"capabilities": [
					{
						"action": "set_power",
						"kind": "command",
						"risk": "medium",
						"requires_code": false,
						"params": {
							"type": "object",
							"properties": {},
							"additionalProperties": true
						}
					}
				]
			}`,
			errContains: "params",
		},
		{
			name: "state con campo extra no permitido",
			jsonDoc: `{
				"id": "ha:light.salon",
				"source": {"kind": "ha", "entity_id": "light.salon"},
				"name": "Luz", "aliases": [], "category": "light", "exposed": true, "risk": "low",
				"state": {"power": "on", "campo_fantasma": 123},
				"capabilities": []
			}`,
			errContains: "state",
		},
		{
			name: "categoria desconocida",
			jsonDoc: `{
				"id": "ha:vacuum.robot",
				"source": {"kind": "ha", "entity_id": "vacuum.robot"},
				"name": "Robot", "aliases": [], "category": "vacuum", "exposed": true, "risk": "low",
				"state": {}, "capabilities": []
			}`,
			errContains: "category",
		},
		{
			name: "falta campo obligatorio exposed",
			jsonDoc: `{
				"id": "ha:light.salon",
				"source": {"kind": "ha", "entity_id": "light.salon"},
				"name": "Luz", "aliases": [], "category": "light", "risk": "low",
				"state": {"power": "on"}, "capabilities": []
			}`,
			errContains: "exposed",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := v.ValidateEntity([]byte(tt.jsonDoc))
			if err == nil {
				t.Fatalf("expected error containing %q, got nil", tt.errContains)
			}
			if !strings.Contains(strings.ToLower(err.Error()), strings.ToLower(tt.errContains)) {
				t.Errorf("expected error containing %q, got: %v", tt.errContains, err)
			}
		})
	}
}

func TestValidator_ErrorHasPointer(t *testing.T) {
	v, err := contracts.NewValidator()
	if err != nil {
		t.Fatalf("NewValidator() error: %v", err)
	}

	badDoc := []byte(`{
		"id": "sin_prefijo",
		"source": {"kind": "ha", "entity_id": "light.salon"},
		"name": "Luz", "aliases": [], "category": "light", "exposed": true, "risk": "low",
		"state": {"power": "on"}, "capabilities": []
	}`)

	err = v.ValidateEntity(badDoc)
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	// Error should indicate pointer/location /id
	errStr := err.Error()
	if !strings.Contains(errStr, "id") && !strings.Contains(errStr, "/id") {
		t.Errorf("error %q should mention JSON pointer or property name", errStr)
	}
}

func TestValidateParams(t *testing.T) {
	cap := contracts.Capability{
		Action:       "set_brightness",
		Kind:         contracts.KindCommand,
		Risk:         contracts.RiskLow,
		RequiresCode: false,
		Params: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"brightness_pct": map[string]any{
					"type":    "integer",
					"minimum": 1,
					"maximum": 100,
				},
			},
			"required":             []any{"brightness_pct"},
			"additionalProperties": false,
		},
	}

	t.Run("parametro valido", func(t *testing.T) {
		validParams := map[string]any{"brightness_pct": 50}
		if err := contracts.ValidateParams(cap, validParams); err != nil {
			t.Fatalf("expected valid params, got: %v", err)
		}
	})

	t.Run("parametro fuera de rango", func(t *testing.T) {
		invalidParams := map[string]any{"brightness_pct": 150}
		if err := contracts.ValidateParams(cap, invalidParams); err == nil {
			t.Fatal("expected error for brightness_pct > 100, got nil")
		}
	})

	t.Run("parametro faltante requerido", func(t *testing.T) {
		emptyParams := map[string]any{}
		if err := contracts.ValidateParams(cap, emptyParams); err == nil {
			t.Fatal("expected error for missing required brightness_pct, got nil")
		}
	})

	t.Run("propiedad extra no permitida", func(t *testing.T) {
		extraParams := map[string]any{"brightness_pct": 50, "extra": "prohibido"}
		if err := contracts.ValidateParams(cap, extraParams); err == nil {
			t.Fatal("expected error for unexpected property, got nil")
		}
	})
}

func TestRiskRank(t *testing.T) {
	if contracts.RiskLow.Rank() >= contracts.RiskMedium.Rank() {
		t.Error("expected low < medium")
	}
	if contracts.RiskMedium.Rank() >= contracts.RiskHigh.Rank() {
		t.Error("expected medium < high")
	}
	if contracts.RiskHigh.Rank() >= contracts.RiskCritical.Rank() {
		t.Error("expected high < critical")
	}

	if got := contracts.MaxRisk(contracts.RiskLow, contracts.RiskCritical); got != contracts.RiskCritical {
		t.Errorf("expected critical, got %s", got)
	}
	if got := contracts.MaxRisk(contracts.RiskHigh, contracts.RiskMedium); got != contracts.RiskHigh {
		t.Errorf("expected high, got %s", got)
	}
}

func TestEntity_JSONSerializationRoundTrip(t *testing.T) {
	ent := contracts.Entity{
		ID: "ha:light.salon",
		Source: contracts.Source{
			Kind:     "ha",
			EntityID: "light.salon",
		},
		Name:     "Luz Salón",
		Aliases:  []string{"luz techo"},
		Category: "light",
		Exposed:  true,
		Risk:     contracts.RiskLow,
		State:    map[string]any{"power": "on"},
		Capabilities: []contracts.Capability{
			{
				Action: contracts.ActionSetPower,
				Kind:   contracts.KindCommand,
				Risk:   contracts.RiskLow,
				Params: map[string]any{
					"type":                 "object",
					"properties":           map[string]any{"state": map[string]any{"type": "string", "enum": []any{"on", "off"}}},
					"required":             []any{"state"},
					"additionalProperties": false,
				},
			},
		},
	}

	data, err := json.Marshal(ent)
	if err != nil {
		t.Fatalf("Marshal error: %v", err)
	}

	v, err := contracts.NewValidator()
	if err != nil {
		t.Fatalf("NewValidator error: %v", err)
	}
	if err := v.ValidateEntity(data); err != nil {
		t.Fatalf("ValidateEntity serialized struct error: %v", err)
	}
}
