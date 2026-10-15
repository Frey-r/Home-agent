package entities

import (
	"strings"

	"github.com/Frey-r/Home-agent/contracts"
	"github.com/Frey-r/Home-agent/edge/internal/ha"
)

// ResolveName resuelve el nombre primario de una entidad siguiendo la precedencia:
// 1. Nombre en el registro de entidades.
// 2. friendly_name en los atributos de estado.
// 3. Nombre del dispositivo físico asociado.
// 4. entity_id original de HA como fallback.
func ResolveName(entityID string, attrs map[string]any, entry *ha.EntityRegistryEntry, dev *ha.DeviceRegistryEntry) string {
	if entry != nil && entry.Name != nil {
		s := strings.TrimSpace(*entry.Name)
		if s != "" {
			return s
		}
	}
	if fn, ok := StringAttr(attrs, "friendly_name"); ok {
		return fn
	}
	if dev != nil && dev.Name != nil {
		s := strings.TrimSpace(*dev.Name)
		if s != "" {
			return s
		}
	}
	return entityID
}

// ResolveAliases normaliza, limpia y deduplica los alias definidos en el registro de entidades,
// descartando cadenas vacías y cualquier alias coincidente (case-insensitive) con el nombre primario.
func ResolveAliases(entry *ha.EntityRegistryEntry, primaryName string) []string {
	if entry == nil || len(entry.Aliases) == 0 {
		return []string{}
	}

	primaryLower := strings.ToLower(strings.TrimSpace(primaryName))
	seen := make(map[string]bool)
	result := make([]string, 0)

	for _, a := range entry.Aliases {
		trimmed := strings.TrimSpace(a)
		if trimmed == "" {
			continue
		}
		lower := strings.ToLower(trimmed)
		if lower == primaryLower || seen[lower] {
			continue
		}
		seen[lower] = true
		result = append(result, trimmed)
	}
	return result
}

// ResolveArea resuelve el área asignada a la entidad o la hereda de su dispositivo asociado.
func ResolveArea(entry *ha.EntityRegistryEntry, regs *ha.Registries) *contracts.Area {
	if entry == nil || regs == nil {
		return nil
	}

	// 1. Asignación directa a la entidad
	if entry.AreaID != nil && *entry.AreaID != "" {
		if a, ok := regs.Areas[*entry.AreaID]; ok {
			return &contracts.Area{ID: a.ID, Name: a.Name}
		}
	}

	// 2. Asignación heredada del dispositivo
	if entry.DeviceID != nil && *entry.DeviceID != "" {
		if dev, ok := regs.Devices[*entry.DeviceID]; ok && dev.AreaID != nil && *dev.AreaID != "" {
			if a, ok := regs.Areas[*dev.AreaID]; ok {
				return &contracts.Area{ID: a.ID, Name: a.Name}
			}
		}
	}

	return nil
}

// ResolveAvailability determina si la entidad está operativa en función de su estado crudo.
func ResolveAvailability(rawState string) bool {
	s := strings.ToLower(strings.TrimSpace(rawState))
	return s != "unavailable" && s != "unknown"
}
