package entities

import (
	"strings"

	"github.com/Frey-r/Home-agent/contracts"
	"github.com/Frey-r/Home-agent/edge/internal/ha"
)

func buildCover(raw ha.State) (string, any, []contracts.Capability) {
	category := "cover"
	stateMap := make(map[string]any)

	status := strings.ToLower(raw.State)
	switch status {
	case "open", "closed", "opening", "closing", "stopped":
		stateMap["status"] = status
	default:
		stateMap["status"] = "closed"
	}

	if pos, ok := IntAttr(raw.Attributes, "current_position"); ok && pos >= 0 && pos <= 100 {
		stateMap["position_pct"] = pos
	}

	caps := []contracts.Capability{
		{
			Action: "open",
			Kind:   contracts.KindCommand,
			Risk:   ActionRisk(category, "open"),
		},
		{
			Action: "close",
			Kind:   contracts.KindCommand,
			Risk:   ActionRisk(category, "close"),
		},
		{
			Action: "stop",
			Kind:   contracts.KindCommand,
			Risk:   ActionRisk(category, "stop"),
		},
		{
			Action: "set_position",
			Kind:   contracts.KindCommand,
			Risk:   ActionRisk(category, "set_position"),
			Params: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"position_pct": map[string]any{
						"type":    "integer",
						"minimum": 0,
						"maximum": 100,
					},
				},
				"required":             []any{"position_pct"},
				"additionalProperties": false,
			},
		},
	}

	return category, stateMap, caps
}
