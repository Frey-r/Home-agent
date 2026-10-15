package main

import (
	"context"
	"encoding/json"
	"log/slog"
	"strings"

	"github.com/Frey-r/Home-agent/edge/internal/ha"
)

func runSeed(ctx context.Context, conn *ha.Conn) error {
	slog.Info("iniciando sembrado de Home Assistant")

	// 1. Exponer a conversation cerradura, alarma, script y weather
	entitiesToExpose := []string{
		"lock.front_door",
		"alarm_control_panel.alarm",
		"script.saludo",
		"weather.demo",
	}

	_, _ = conn.Command(ctx, map[string]any{
		"type":          "homeassistant/expose_entity",
		"assistants":    []string{"conversation"},
		"entity_ids":    entitiesToExpose,
		"should_expose": true,
	})

	// 2. Asegurar áreas salon y cocina
	var salonAreaID, cocinaAreaID string
	regs, err := conn.Registries(ctx)
	if err == nil && regs != nil {
		for id, a := range regs.Areas {
			if a.Name == "Salón" || a.ID == "salon" {
				salonAreaID = id
			}
			if a.Name == "Cocina" || a.ID == "cocina" {
				cocinaAreaID = id
			}
		}
	}

	if salonAreaID == "" {
		res, err := conn.Command(ctx, map[string]any{
			"type": "config/area_registry/create",
			"name": "Salón",
		})
		if err == nil {
			var created struct {
				ID string `json:"id"`
			}
			_ = json.Unmarshal(res, &created)
			salonAreaID = created.ID
		}
		if salonAreaID == "" {
			salonAreaID = "salon"
		}
	}

	if cocinaAreaID == "" {
		res, err := conn.Command(ctx, map[string]any{
			"type": "config/area_registry/create",
			"name": "Cocina",
		})
		if err == nil {
			var created struct {
				ID string `json:"id"`
			}
			_ = json.Unmarshal(res, &created)
			cocinaAreaID = created.ID
		}
		if cocinaAreaID == "" {
			cocinaAreaID = "cocina"
		}
	}

	// 3. Asignar áreas a dos dispositivos si existen
	if regs != nil && len(regs.Devices) > 0 {
		count := 0
		for devID := range regs.Devices {
			if count == 0 {
				_, _ = conn.Command(ctx, map[string]any{
					"type":      "config/device_registry/update",
					"device_id": devID,
					"area_id":   salonAreaID,
				})
			} else if count == 1 {
				_, _ = conn.Command(ctx, map[string]any{
					"type":      "config/device_registry/update",
					"device_id": devID,
					"area_id":   cocinaAreaID,
				})
			}
			count++
			if count >= 2 {
				break
			}
		}
	}

	// 4. Agregar alias "luz de pie" a una entidad
	targetEntity := "light.bed_light"
	if regs != nil && len(regs.Entities) > 0 {
		if _, ok := regs.Entities[targetEntity]; !ok {
			for eid := range regs.Entities {
				if strings.HasPrefix(eid, "light.") {
					targetEntity = eid
					break
				}
			}
		}
	}

	_, _ = conn.Command(ctx, map[string]any{
		"type":      "config/entity_registry/update",
		"entity_id": targetEntity,
		"aliases":   []any{"luz de pie"},
	})

	slog.Info("sembrado completado exitosamente")
	return nil
}
