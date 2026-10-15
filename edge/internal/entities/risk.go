package entities

import "github.com/Frey-r/Home-agent/contracts"

// ActionRisk devuelve el nivel de riesgo canónico para una acción en una categoría dada,
// según las directrices de seguridad de ADR-0007 y el modelo de amenazas del hogar.
func ActionRisk(category, action string) contracts.Risk {
	switch category {
	case "light":
		return contracts.RiskLow
	case "switch":
		return contracts.RiskLow
	case "cover":
		if action == "stop" {
			return contracts.RiskLow
		}
		return contracts.RiskMedium
	case "lock":
		if action == "lock" {
			return contracts.RiskLow
		}
		return contracts.RiskHigh
	case "alarm_control_panel":
		if action == "disarm" {
			return contracts.RiskHigh
		}
		return contracts.RiskMedium
	case "climate":
		if action == "set_power" {
			return contracts.RiskLow
		}
		return contracts.RiskMedium
	case "media_player":
		return contracts.RiskLow
	case "scene":
		return contracts.RiskLow
	case "script":
		return contracts.RiskLow
	case "todo":
		return contracts.RiskLow
	case "weather":
		return contracts.RiskLow
	default:
		return contracts.RiskLow
	}
}

// EntityRisk calcula el riesgo agregado de una entidad como el máximo nivel de riesgo de sus capacidades.
func EntityRisk(caps []contracts.Capability) contracts.Risk {
	maxR := contracts.RiskLow
	for _, c := range caps {
		maxR = contracts.MaxRisk(maxR, c.Risk)
	}
	return maxR
}

