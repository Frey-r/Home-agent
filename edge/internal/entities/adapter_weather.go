package entities

import (
	"strings"

	"github.com/Frey-r/Home-agent/contracts"
	"github.com/Frey-r/Home-agent/edge/internal/ha"
)

func buildWeather(raw ha.State) (string, any, []contracts.Capability) {
	category := "weather"
	stateMap := make(map[string]any)

	cond := strings.ToLower(strings.TrimSpace(raw.State))
	if cond != "" {
		stateMap["condition"] = cond
	}

	if t, ok := FloatAttr(raw.Attributes, "temperature"); ok {
		stateMap["temperature"] = t
	}
	if tu, ok := StringAttr(raw.Attributes, "temperature_unit"); ok {
		stateMap["temp_unit"] = tu
	}
	if h, ok := FloatAttr(raw.Attributes, "humidity"); ok {
		stateMap["humidity"] = h
	}
	if ws, ok := FloatAttr(raw.Attributes, "wind_speed"); ok {
		stateMap["wind_speed"] = ws
	}
	if wsu, ok := StringAttr(raw.Attributes, "wind_speed_unit"); ok {
		stateMap["wind_speed_unit"] = wsu
	}
	if p, ok := FloatAttr(raw.Attributes, "pressure"); ok {
		stateMap["pressure"] = p
	}
	if pu, ok := StringAttr(raw.Attributes, "pressure_unit"); ok {
		stateMap["pressure_unit"] = pu
	}

	caps := []contracts.Capability{
		{
			Action: "get_forecast",
			Kind:   contracts.KindQuery,
			Risk:   ActionRisk(category, "get_forecast"),
		},
	}

	return category, stateMap, caps
}
