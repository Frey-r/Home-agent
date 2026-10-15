package ha

import (
	"context"
	"encoding/json"
	"fmt"
)

// EntityRegistryEntry representa una entrada en el registro de entidades de HA.
type EntityRegistryEntry struct {
	ID         string   `json:"id"`
	EntityID   string   `json:"entity_id"`
	Name       *string  `json:"name"`
	DeviceID   *string  `json:"device_id"`
	AreaID     *string  `json:"area_id"`
	DisabledBy *string  `json:"disabled_by"`
	HiddenBy   *string  `json:"hidden_by"`
	Aliases    []string `json:"aliases"`
}

// DeviceRegistryEntry representa un dispositivo físico en el registro de HA.
type DeviceRegistryEntry struct {
	ID           string  `json:"id"`
	Name         *string `json:"name"`
	AreaID       *string `json:"area_id"`
	Model        *string `json:"model"`
	Manufacturer *string `json:"manufacturer"`
}

// AreaRegistryEntry representa una habitación o zona en el registro de HA.
type AreaRegistryEntry struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// Registries agrupa las entidades, dispositivos y áreas leídas en bloque.
type Registries struct {
	Entities map[string]EntityRegistryEntry
	Devices  map[string]DeviceRegistryEntry
	Areas    map[string]AreaRegistryEntry
}

// Registries lee los registros de entidades, dispositivos y áreas en un pipeline todo-o-nada.
func (c *Conn) Registries(ctx context.Context) (*Registries, error) {
	rawEntities, err := c.Command(ctx, map[string]any{"type": "config/entity_registry/list"})
	if err != nil {
		return nil, fmt.Errorf("ha: entity_registry/list: %w", err)
	}

	rawDevices, err := c.Command(ctx, map[string]any{"type": "config/device_registry/list"})
	if err != nil {
		return nil, fmt.Errorf("ha: device_registry/list: %w", err)
	}

	rawAreas, err := c.Command(ctx, map[string]any{"type": "config/area_registry/list"})
	if err != nil {
		return nil, fmt.Errorf("ha: area_registry/list: %w", err)
	}

	var entList []EntityRegistryEntry
	if err := json.Unmarshal(rawEntities, &entList); err != nil {
		return nil, fmt.Errorf("ha: parsear entity_registry: %w", err)
	}

	var devList []DeviceRegistryEntry
	if err := json.Unmarshal(rawDevices, &devList); err != nil {
		return nil, fmt.Errorf("ha: parsear device_registry: %w", err)
	}

	var areaList []AreaRegistryEntry
	if err := json.Unmarshal(rawAreas, &areaList); err != nil {
		return nil, fmt.Errorf("ha: parsear area_registry: %w", err)
	}

	regs := &Registries{
		Entities: make(map[string]EntityRegistryEntry, len(entList)),
		Devices:  make(map[string]DeviceRegistryEntry, len(devList)),
		Areas:    make(map[string]AreaRegistryEntry, len(areaList)),
	}

	for _, e := range entList {
		regs.Entities[e.EntityID] = e
	}
	for _, d := range devList {
		regs.Devices[d.ID] = d
	}
	for _, a := range areaList {
		regs.Areas[a.ID] = a
	}

	return regs, nil
}

// EntityDetails contiene información detallada complementaria de una entidad.
type EntityDetails struct {
	EntityID string   `json:"entity_id"`
	Aliases  []string `json:"aliases"`
}

// EntityDetails consulta get_entries tolerando entradas nulas y alias nulos.
func (c *Conn) EntityDetails(ctx context.Context, entityIDs []string) (map[string]EntityDetails, error) {
	raw, err := c.Command(ctx, map[string]any{
		"type":       "config/entity_registry/get_entries",
		"entity_ids": entityIDs,
	})
	if err != nil {
		return nil, fmt.Errorf("ha: entity_registry/get_entries: %w", err)
	}

	var rawMap map[string]json.RawMessage
	if err := json.Unmarshal(raw, &rawMap); err != nil {
		return nil, fmt.Errorf("ha: decodificar get_entries: %w", err)
	}

	res := make(map[string]EntityDetails, len(rawMap))
	for k, data := range rawMap {
		if string(data) == "null" || len(data) == 0 {
			continue
		}
		var item struct {
			EntityID string `json:"entity_id"`
			Aliases  []any  `json:"aliases"`
		}
		if err := json.Unmarshal(data, &item); err != nil {
			continue
		}

		cleanAliases := make([]string, 0, len(item.Aliases))
		for _, a := range item.Aliases {
			if str, ok := a.(string); ok && str != "" {
				cleanAliases = append(cleanAliases, str)
			}
		}

		eid := item.EntityID
		if eid == "" {
			eid = k
		}
		res[k] = EntityDetails{
			EntityID: eid,
			Aliases:  cleanAliases,
		}
	}

	return res, nil
}

// Exposed devuelve el mapa de entidades expuestas para un asistente específico (fail-closed).
func (c *Conn) Exposed(ctx context.Context, assistant string) (map[string]bool, error) {
	raw, err := c.Command(ctx, map[string]any{
		"type": "homeassistant/expose_entity/list",
	})
	if err != nil {
		return nil, fmt.Errorf("ha: expose_entity/list: %w", err)
	}

	var resp struct {
		ExposedEntities map[string]map[string]any `json:"exposed_entities"`
	}
	if err := json.Unmarshal(raw, &resp); err != nil {
		return nil, fmt.Errorf("ha: decodificar expose_entity/list: %w", err)
	}

	asstMap, ok := resp.ExposedEntities[assistant]
	if !ok {
		return map[string]bool{}, nil
	}

	res := make(map[string]bool, len(asstMap))
	for eid, val := range asstMap {
		// Fail-closed: solo si es explícitamente true se considera expuesta
		if b, isBool := val.(bool); isBool && b {
			res[eid] = true
		} else {
			res[eid] = false
		}
	}

	return res, nil
}

// UnitSystem describe las unidades de medida configuradas en HA.
type UnitSystem struct {
	Temperature string `json:"temperature"`
	Length      string `json:"length"`
	Mass        string `json:"mass"`
	Pressure    string `json:"pressure"`
	Volume      string `json:"volume"`
}

// HAConfig contiene la configuración del core de HA.
type HAConfig struct {
	State      string     `json:"state"`
	Version    string     `json:"version"`
	TimeZone   string     `json:"time_zone"`
	UnitSystem UnitSystem `json:"unit_system"`
}

// GetConfig consulta y parsea la configuración del core de Home Assistant.
func (c *Conn) GetConfig(ctx context.Context) (*HAConfig, error) {
	raw, err := c.Command(ctx, map[string]any{"type": "get_config"})
	if err != nil {
		return nil, fmt.Errorf("ha: get_config: %w", err)
	}

	var cfg HAConfig
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return nil, fmt.Errorf("ha: decodificar get_config: %w", err)
	}
	return &cfg, nil
}
