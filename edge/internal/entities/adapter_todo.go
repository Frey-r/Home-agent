package entities

import (
	"github.com/Frey-r/Home-agent/contracts"
	"github.com/Frey-r/Home-agent/edge/internal/ha"
)

func buildTodo(raw ha.State) (string, any, []contracts.Capability) {
	category := "todo"
	stateMap := make(map[string]any)

	// En HA, el state de un todo suele ser el número de ítems abiertos como cadena o entero
	if items, ok := IntAttr(map[string]any{"v": raw.State}, "v"); ok && items >= 0 {
		stateMap["open_items"] = items
	} else if items, ok := IntAttr(raw.Attributes, "open_items"); ok && items >= 0 {
		stateMap["open_items"] = items
	} else {
		stateMap["open_items"] = 0
	}

	caps := []contracts.Capability{
		{
			Action: "add_item",
			Kind:   contracts.KindCommand,
			Risk:   ActionRisk(category, "add_item"),
			Params: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"item": map[string]any{
						"type": "string",
					},
				},
				"required":             []any{"item"},
				"additionalProperties": false,
			},
		},
		{
			Action: "complete_item",
			Kind:   contracts.KindCommand,
			Risk:   ActionRisk(category, "complete_item"),
			Params: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"item": map[string]any{
						"type": "string",
					},
				},
				"required":             []any{"item"},
				"additionalProperties": false,
			},
		},
		{
			Action: "remove_item",
			Kind:   contracts.KindCommand,
			Risk:   ActionRisk(category, "remove_item"),
			Params: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"item": map[string]any{
						"type": "string",
					},
				},
				"required":             []any{"item"},
				"additionalProperties": false,
			},
		},
		{
			Action: "list_items",
			Kind:   contracts.KindQuery,
			Risk:   ActionRisk(category, "list_items"),
		},
	}

	return category, stateMap, caps
}
