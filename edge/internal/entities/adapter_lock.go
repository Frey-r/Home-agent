package entities

import (
	"strings"

	"github.com/Frey-r/Home-agent/contracts"
	"github.com/Frey-r/Home-agent/edge/internal/ha"
)

func buildLock(raw ha.State) (string, any, []contracts.Capability) {
	category := "lock"
	stateMap := make(map[string]any)

	status := strings.ToLower(raw.State)
	switch status {
	case "locked", "unlocked", "locking", "unlocking", "jammed":
		stateMap["status"] = status
	default:
		stateMap["status"] = "locked"
	}

	caps := []contracts.Capability{
		{
			Action: "lock",
			Kind:   contracts.KindCommand,
			Risk:   ActionRisk(category, "lock"),
		},
		{
			Action:       "unlock",
			Kind:         contracts.KindCommand,
			Risk:         ActionRisk(category, "unlock"),
			RequiresCode: true,
		},
	}

	return category, stateMap, caps
}
