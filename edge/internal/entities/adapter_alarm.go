package entities

import (
	"strings"

	"github.com/Frey-r/Home-agent/contracts"
	"github.com/Frey-r/Home-agent/edge/internal/ha"
)

func buildAlarm(raw ha.State) (string, any, []contracts.Capability) {
	category := "alarm_control_panel"
	stateMap := make(map[string]any)

	status := strings.ToLower(raw.State)
	switch status {
	case "disarmed", "armed_home", "armed_away", "armed_night", "armed_vacation", "armed_custom_bypass", "arming", "disarming", "triggered", "pending":
		stateMap["status"] = status
	default:
		stateMap["status"] = "disarmed"
	}

	caps := []contracts.Capability{
		{
			Action: "arm",
			Kind:   contracts.KindCommand,
			Risk:   ActionRisk(category, "arm"),
			Params: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"mode": map[string]any{
						"type": "string",
						"enum": []any{"home", "away", "night", "vacation", "custom_bypass"},
					},
				},
				"required":             []any{"mode"},
				"additionalProperties": false,
			},
		},
		{
			Action:       "disarm",
			Kind:         contracts.KindCommand,
			Risk:         ActionRisk(category, "disarm"),
			RequiresCode: true,
		},
	}

	return category, stateMap, caps
}
