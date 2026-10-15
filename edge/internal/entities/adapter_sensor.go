package entities

import (
	"strings"

	"github.com/Frey-r/Home-agent/contracts"
	"github.com/Frey-r/Home-agent/edge/internal/ha"
)

func buildSensor(raw ha.State) (string, any, []contracts.Capability) {
	category := "sensor"
	stateMap := make(map[string]any)

	// Intentar extraer valor numérico; si no, guardar como cadena
	rawVal := strings.TrimSpace(raw.State)
	if f, ok := FloatAttr(map[string]any{"v": rawVal}, "v"); ok {
		stateMap["value"] = f
	} else {
		stateMap["value"] = rawVal
	}

	if unit, ok := StringAttr(raw.Attributes, "unit_of_measurement"); ok {
		stateMap["unit"] = unit
	}

	return category, stateMap, nil
}
