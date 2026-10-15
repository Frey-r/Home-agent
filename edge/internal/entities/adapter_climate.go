package entities

import (
	"strings"

	"github.com/Frey-r/Home-agent/contracts"
	"github.com/Frey-r/Home-agent/edge/internal/ha"
)

func buildClimate(raw ha.State) (string, any, []contracts.Capability) {
	category := "climate"
	stateMap := make(map[string]any)

	hvacMode := strings.ToLower(strings.TrimSpace(raw.State))
	stateMap["hvac_mode"] = hvacMode

	power := "on"
	if hvacMode == "off" {
		power = "off"
	}
	stateMap["power"] = power

	if t, ok := FloatAttr(raw.Attributes, "temperature"); ok {
		stateMap["target_temp"] = t
	}
	if tLow, ok := FloatAttr(raw.Attributes, "target_temp_low"); ok {
		stateMap["target_temp_low"] = tLow
	}
	if tHigh, ok := FloatAttr(raw.Attributes, "target_temp_high"); ok {
		stateMap["target_temp_high"] = tHigh
	}
	if curT, ok := FloatAttr(raw.Attributes, "current_temperature"); ok {
		stateMap["current_temp"] = curT
	}
	if curH, ok := FloatAttr(raw.Attributes, "current_humidity"); ok {
		stateMap["current_humidity"] = curH
	}
	if act, ok := StringAttr(raw.Attributes, "hvac_action"); ok {
		stateMap["hvac_action"] = act
	}
	if fan, ok := StringAttr(raw.Attributes, "fan_mode"); ok {
		stateMap["fan_mode"] = fan
	}
	if preset, ok := StringAttr(raw.Attributes, "preset_mode"); ok {
		stateMap["preset_mode"] = preset
	}
	if unit, ok := StringAttr(raw.Attributes, "unit_of_measurement"); ok {
		stateMap["temp_unit"] = unit
	} else if unit, ok := StringAttr(raw.Attributes, "temperature_unit"); ok {
		stateMap["temp_unit"] = unit
	}

	caps := []contracts.Capability{
		{
			Action: "set_power",
			Kind:   contracts.KindCommand,
			Risk:   ActionRisk(category, "set_power"),
			Params: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"power": map[string]any{
						"type": "string",
						"enum": []any{"on", "off"},
					},
				},
				"required":             []any{"power"},
				"additionalProperties": false,
			},
		},
		{
			Action: "set_hvac_mode",
			Kind:   contracts.KindCommand,
			Risk:   ActionRisk(category, "set_hvac_mode"),
			Params: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"hvac_mode": map[string]any{
						"type": "string",
					},
				},
				"required":             []any{"hvac_mode"},
				"additionalProperties": false,
			},
		},
		{
			Action: "set_temperature",
			Kind:   contracts.KindCommand,
			Risk:   ActionRisk(category, "set_temperature"),
			Params: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"temperature": map[string]any{
						"type": "number",
					},
				},
				"required":             []any{"temperature"},
				"additionalProperties": false,
			},
		},
	}

	if _, hasPreset := raw.Attributes["preset_mode"]; hasPreset {
		caps = append(caps, contracts.Capability{
			Action: "set_preset_mode",
			Kind:   contracts.KindCommand,
			Risk:   ActionRisk(category, "set_preset_mode"),
			Params: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"preset_mode": map[string]any{
						"type": "string",
					},
				},
				"required":             []any{"preset_mode"},
				"additionalProperties": false,
			},
		})
	}

	return category, stateMap, caps
}
