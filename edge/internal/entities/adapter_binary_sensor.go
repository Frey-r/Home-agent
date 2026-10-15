package entities

import (
	"strings"

	"github.com/Frey-r/Home-agent/contracts"
	"github.com/Frey-r/Home-agent/edge/internal/ha"
)

func buildBinarySensor(raw ha.State) (string, any, []contracts.Capability) {
	category := "binary_sensor"
	stateMap := make(map[string]any)

	active := strings.ToLower(strings.TrimSpace(raw.State)) == "on"
	stateMap["active"] = active

	return category, stateMap, nil
}
