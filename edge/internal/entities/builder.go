package entities

import (
	"strings"

	"github.com/Frey-r/Home-agent/contracts"
	"github.com/Frey-r/Home-agent/edge/internal/ha"
)

// DomainBuilder construye la categoría, el estado estructurado y las capacidades
// a partir del estado crudo de Home Assistant.
type DomainBuilder func(raw ha.State) (category string, state any, capabilities []contracts.Capability)

var adapters = map[string]DomainBuilder{
	"light":               buildLight,
	"switch":              buildSwitch,
	"cover":               buildCover,
	"lock":                buildLock,
	"alarm_control_panel": buildAlarm,
	"climate":             buildClimate,
}

// RegisterAdapter permite registrar o sobreescribir un adaptador para un dominio de HA.
func RegisterAdapter(domain string, b DomainBuilder) {
	adapters[domain] = b
}

// Build transforma un estado crudo de Home Assistant en una entidad normalizada v1 (ADR-0007).
// Si el dominio no está soportado en este hito, se omite silenciosamente devolviendo (nil, nil).
func Build(raw ha.State, reg *ha.Registries, exposed map[string]bool) (*contracts.Entity, error) {
	parts := strings.SplitN(raw.EntityID, ".", 2)
	if len(parts) != 2 {
		return nil, nil
	}
	domain := parts[0]

	adapter, ok := adapters[domain]
	if !ok {
		return nil, nil // Dominio no soportado: omitir en silencio
	}

	var entry *ha.EntityRegistryEntry
	var dev *ha.DeviceRegistryEntry

	if reg != nil {
		if e, found := reg.Entities[raw.EntityID]; found {
			entry = &e
			if entry.DeviceID != nil && *entry.DeviceID != "" {
				if d, foundDev := reg.Devices[*entry.DeviceID]; foundDev {
					dev = &d
				}
			}
		}
	}

	name := ResolveName(raw.EntityID, raw.Attributes, entry, dev)
	aliases := ResolveAliases(entry, name)
	area := ResolveArea(entry, reg)
	available := ResolveAvailability(raw.State)

	isExposed := false
	if exposed != nil {
		isExposed = exposed[raw.EntityID]
	}

	category, state, capabilities := adapter(raw)
	overallRisk := EntityRisk(capabilities)

	emptyParams := map[string]any{
		"type":                 "object",
		"properties":           map[string]any{},
		"additionalProperties": false,
	}
	for i := range capabilities {
		if capabilities[i].Params == nil {
			capabilities[i].Params = emptyParams
		}
	}

	if aliases == nil {
		aliases = []string{}
	}

	var devID *string
	if dev != nil {
		devID = &dev.ID
	}

	stateMap, ok := state.(map[string]any)
	if !ok {
		stateMap = make(map[string]any)
	}

	ent := &contracts.Entity{
		ID:           "ha:" + raw.EntityID,
		Name:         name,
		Aliases:      aliases,
		Category:     category,
		Area:         area,
		Risk:         overallRisk,
		Exposed:      isExposed,
		Available:    &available,
		State:        stateMap,
		Capabilities: capabilities,
		Source: contracts.Source{
			Kind:     "ha",
			EntityID: raw.EntityID,
			DeviceID: devID,
		},
	}

	return ent, nil
}
