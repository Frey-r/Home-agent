package entities

import (
	"strings"

	"github.com/Frey-r/Home-agent/contracts"
	"github.com/Frey-r/Home-agent/edge/internal/ha"
)

func buildScript(raw ha.State) (string, any, []contracts.Capability) {
	category := "script"
	stateMap := make(map[string]any)

	running := strings.ToLower(strings.TrimSpace(raw.State)) == "on"
	stateMap["running"] = running

	if lt, ok := StringAttr(raw.Attributes, "last_triggered"); ok {
		stateMap["last_triggered"] = lt
	}

	caps := []contracts.Capability{
		{
			Action: "run",
			Kind:   contracts.KindCommand,
			Risk:   ActionRisk(category, "run"),
		},
	}

	return category, stateMap, caps
}
