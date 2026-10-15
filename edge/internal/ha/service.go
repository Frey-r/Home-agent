package ha

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
)

// ServiceTarget especifica los destinatarios de una llamada a servicio.
type ServiceTarget struct {
	EntityIDs []string `json:"entity_id,omitempty"`
	DeviceIDs []string `json:"device_id,omitempty"`
	AreaIDs   []string `json:"area_id,omitempty"`
}

// ServiceCall describe una invocación de servicio en Home Assistant.
type ServiceCall struct {
	Domain         string         `json:"domain"`
	Service        string         `json:"service"`
	Target         ServiceTarget  `json:"target,omitempty"`
	ServiceData    map[string]any `json:"service_data,omitempty"`
	ReturnResponse bool           `json:"return_response,omitempty"`
}

// ServiceResult almacena el identificador de contexto y la respuesta si la hubo.
type ServiceResult struct {
	ContextID string          `json:"context_id"`
	Response  json.RawMessage `json:"response,omitempty"`
}

// CallService ejecuta un servicio en HA. Nunca se reintenta automáticamente por seguridad.
func (c *Conn) CallService(ctx context.Context, call ServiceCall) (ServiceResult, error) {
	if call.Domain == "" {
		return ServiceResult{}, errors.New("ha: call_service requiere domain")
	}
	if call.Service == "" {
		return ServiceResult{}, errors.New("ha: call_service requiere service")
	}

	cmd := map[string]any{
		"type":    "call_service",
		"domain":  call.Domain,
		"service": call.Service,
	}

	target := map[string]any{}
	if len(call.Target.EntityIDs) > 0 {
		if len(call.Target.EntityIDs) == 1 {
			target["entity_id"] = call.Target.EntityIDs[0]
		} else {
			target["entity_id"] = call.Target.EntityIDs
		}
	}
	if len(call.Target.DeviceIDs) > 0 {
		if len(call.Target.DeviceIDs) == 1 {
			target["device_id"] = call.Target.DeviceIDs[0]
		} else {
			target["device_id"] = call.Target.DeviceIDs
		}
	}
	if len(call.Target.AreaIDs) > 0 {
		if len(call.Target.AreaIDs) == 1 {
			target["area_id"] = call.Target.AreaIDs[0]
		} else {
			target["area_id"] = call.Target.AreaIDs
		}
	}
	if len(target) > 0 {
		cmd["target"] = target
	}

	if len(call.ServiceData) > 0 {
		cmd["service_data"] = call.ServiceData
	}
	if call.ReturnResponse {
		cmd["return_response"] = true
	}

	raw, err := c.Command(ctx, cmd)
	if err != nil {
		return ServiceResult{}, err
	}

	var res struct {
		Context struct {
			ID string `json:"id"`
		} `json:"context"`
		Response json.RawMessage `json:"response"`
	}
	if err := json.Unmarshal(raw, &res); err != nil {
		return ServiceResult{}, fmt.Errorf("ha: decodificar respuesta de call_service: %w", err)
	}

	return ServiceResult{
		ContextID: res.Context.ID,
		Response:  res.Response,
	}, nil
}
