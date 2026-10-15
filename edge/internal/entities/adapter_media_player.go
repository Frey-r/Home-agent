package entities

import (
	"math"
	"strings"

	"github.com/Frey-r/Home-agent/contracts"
	"github.com/Frey-r/Home-agent/edge/internal/ha"
)

func buildMediaPlayer(raw ha.State) (string, any, []contracts.Capability) {
	category := "media_player"
	stateMap := make(map[string]any)

	status := strings.ToLower(strings.TrimSpace(raw.State))
	switch status {
	case "playing", "paused", "idle", "off", "standby", "buffering":
		stateMap["status"] = status
	default:
		stateMap["status"] = "idle"
	}

	power := "off"
	if status == "playing" || status == "paused" || status == "idle" || status == "buffering" {
		power = "on"
	}
	stateMap["power"] = power

	if vol, ok := FloatAttr(raw.Attributes, "volume_level"); ok && vol >= 0 && vol <= 1 {
		pct := int(math.Round(vol * 100.0))
		stateMap["volume_pct"] = pct
	}

	if muted, ok := BoolAttr(raw.Attributes, "is_volume_muted"); ok {
		stateMap["muted"] = muted
	}

	if src, ok := StringAttr(raw.Attributes, "source"); ok {
		stateMap["source"] = src
	}
	if title, ok := StringAttr(raw.Attributes, "media_title"); ok {
		stateMap["media_title"] = title
	}
	if artist, ok := StringAttr(raw.Attributes, "media_artist"); ok {
		stateMap["media_artist"] = artist
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
			Action: "playback",
			Kind:   contracts.KindCommand,
			Risk:   ActionRisk(category, "playback"),
			Params: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"command": map[string]any{
						"type": "string",
						"enum": []any{"play", "pause", "stop", "next", "previous"},
					},
				},
				"required":             []any{"command"},
				"additionalProperties": false,
			},
		},
		{
			Action: "set_volume",
			Kind:   contracts.KindCommand,
			Risk:   ActionRisk(category, "set_volume"),
			Params: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"volume_pct": map[string]any{
						"type":    "integer",
						"minimum": 0,
						"maximum": 100,
					},
				},
				"required":             []any{"volume_pct"},
				"additionalProperties": false,
			},
		},
		{
			Action: "set_mute",
			Kind:   contracts.KindCommand,
			Risk:   ActionRisk(category, "set_mute"),
			Params: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"muted": map[string]any{
						"type": "boolean",
					},
				},
				"required":             []any{"muted"},
				"additionalProperties": false,
			},
		},
	}

	if _, hasSources := raw.Attributes["source_list"]; hasSources {
		caps = append(caps, contracts.Capability{
			Action: "select_source",
			Kind:   contracts.KindCommand,
			Risk:   ActionRisk(category, "select_source"),
			Params: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"source": map[string]any{
						"type": "string",
					},
				},
				"required":             []any{"source"},
				"additionalProperties": false,
			},
		})
	}

	return category, stateMap, caps
}
