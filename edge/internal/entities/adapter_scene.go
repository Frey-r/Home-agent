package entities

import (
	"strings"

	"github.com/Frey-r/Home-agent/contracts"
	"github.com/Frey-r/Home-agent/edge/internal/ha"
)

func buildScene(raw ha.State) (string, any, []contracts.Capability) {
	category := "scene"
	stateMap := make(map[string]any)

	var members []string
	if rawMembers, ok := raw.Attributes["entity_id"].([]any); ok {
		for _, m := range rawMembers {
			if s, ok := m.(string); ok && strings.TrimSpace(s) != "" {
				members = append(members, strings.TrimSpace(s))
			}
		}
	}
	if members != nil {
		stateMap["members"] = members
	}

	if raw.State != "scening" && raw.State != "" {
		stateMap["last_activated"] = raw.State
	}

	risk := sceneRisk(members)

	caps := []contracts.Capability{
		{
			Action: "activate",
			Kind:   contracts.KindCommand,
			Risk:   risk,
		},
	}

	return category, stateMap, caps
}

func sceneRisk(members []string) contracts.Risk {
	maxR := contracts.RiskLow
	for _, m := range members {
		parts := strings.SplitN(m, ".", 2)
		if len(parts) > 0 {
			switch parts[0] {
			case "lock", "alarm_control_panel":
				maxR = contracts.MaxRisk(maxR, contracts.RiskHigh)
			case "cover", "climate":
				maxR = contracts.MaxRisk(maxR, contracts.RiskMedium)
			}
		}
	}
	return maxR
}
