package entities

import (
	"github.com/Frey-r/Home-agent/contracts"
	"github.com/Frey-r/Home-agent/edge/internal/ha"
)

func buildSwitch(raw ha.State) (string, any, []contracts.Capability) {
	category := "switch"
	stateMap := make(map[string]any)

	power := "off"
	if raw.State == "on" {
		power = "on"
	}
	stateMap["power"] = power

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
	}

	return category, stateMap, caps
}
