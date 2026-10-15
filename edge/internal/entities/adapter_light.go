package entities

import (
	"math"

	"github.com/Frey-r/Home-agent/contracts"
	"github.com/Frey-r/Home-agent/edge/internal/ha"
)

func buildLight(raw ha.State) (string, any, []contracts.Capability) {
	category := "light"
	stateMap := make(map[string]any)

	power := "off"
	if raw.State == "on" {
		power = "on"
	}
	stateMap["power"] = power

	if bVal, ok := IntAttr(raw.Attributes, "brightness"); ok && bVal > 0 {
		pct := int(math.Round(float64(bVal) * 100.0 / 255.0))
		if pct < 1 {
			pct = 1
		} else if pct > 100 {
			pct = 100
		}
		stateMap["brightness_pct"] = pct
	}

	if ctVal, ok := IntAttr(raw.Attributes, "color_temp_kelvin"); ok && ctVal >= 1000 && ctVal <= 10000 {
		stateMap["color_temp_k"] = ctVal
	}

	if rgbRaw, ok := raw.Attributes["rgb_color"].([]any); ok && len(rgbRaw) == 3 {
		r, okR := rgbRaw[0].(int)
		if !okR {
			if rf, okRF := rgbRaw[0].(float64); okRF {
				r = int(rf)
				okR = true
			}
		}
		g, okG := rgbRaw[1].(int)
		if !okG {
			if gf, okGF := rgbRaw[1].(float64); okGF {
				g = int(gf)
				okG = true
			}
		}
		b, okB := rgbRaw[2].(int)
		if !okB {
			if bf, okBF := rgbRaw[2].(float64); okBF {
				b = int(bf)
				okB = true
			}
		}
		if okR && okG && okB {
			stateMap["rgb"] = []any{r, g, b}
		}
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
			Action: "set_brightness",
			Kind:   contracts.KindCommand,
			Risk:   ActionRisk(category, "set_brightness"),
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
		},
	}

	if _, hasCT := raw.Attributes["color_temp_kelvin"]; hasCT {
		caps = append(caps, contracts.Capability{
			Action: "set_color_temp",
			Kind:   contracts.KindCommand,
			Risk:   ActionRisk(category, "set_color_temp"),
			Params: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"color_temp_k": map[string]any{
						"type":    "integer",
						"minimum": 1000,
						"maximum": 10000,
					},
				},
				"required":             []any{"color_temp_k"},
				"additionalProperties": false,
			},
		})
	}

	if _, hasRGB := raw.Attributes["rgb_color"]; hasRGB {
		caps = append(caps, contracts.Capability{
			Action: "set_color",
			Kind:   contracts.KindCommand,
			Risk:   ActionRisk(category, "set_color"),
			Params: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"rgb": map[string]any{
						"type": "array",
						"items": map[string]any{
							"type":    "integer",
							"minimum": 0,
							"maximum": 255,
						},
						"minItems": 3,
						"maxItems": 3,
					},
				},
				"required":             []any{"rgb"},
				"additionalProperties": false,
			},
		})
	}

	return category, stateMap, caps
}
