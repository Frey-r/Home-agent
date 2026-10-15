// Package llmkit proporciona una capa de abstracción desacoplada para interactuar con
// modelos de lenguaje (LLM) tratando a los proveedores como commodities intercambiables.
package llmkit

import (
	"context"
	"fmt"

	"github.com/Frey-r/Home-agent/contracts"
)

// Client define la interfaz de inferencia unificada en formato neutro.
type Client interface {
	// Complete ejecuta una invocación bloqueante y devuelve la respuesta neutra completa.
	Complete(ctx context.Context, req contracts.LLMRequest) (contracts.LLMResponse, error)

	// Stream ejecuta una invocación con streaming SSE y emite eventos incrementales normalizados.
	Stream(ctx context.Context, req contracts.LLMRequest) (<-chan contracts.LLMStreamEvent, <-chan error)
}

// APIError representa un error devuelto por la API HTTP del proveedor de LLM.
type APIError struct {
	StatusCode int    `json:"status_code"`
	Message    string `json:"message"`
	RawBody    string `json:"raw_body,omitempty"`
}

func (e *APIError) Error() string {
	if e.Message != "" {
		return fmt.Sprintf("llmkit: status %d: %s", e.StatusCode, e.Message)
	}
	return fmt.Sprintf("llmkit: error HTTP %d: %s", e.StatusCode, e.RawBody)
}

// IsRateLimit indica si el error corresponde a límite de cuota o saturación (HTTP 429).
func (e *APIError) IsRateLimit() bool {
	return e.StatusCode == 429
}

// IsServerError indica si el error corresponde a fallo del proveedor (HTTP 5xx).
func (e *APIError) IsServerError() bool {
	return e.StatusCode >= 500 && e.StatusCode <= 599
}

// IsClientError indica si el error corresponde a error del cliente (HTTP 4xx excepto 429).
func (e *APIError) IsClientError() bool {
	return e.StatusCode >= 400 && e.StatusCode <= 499 && e.StatusCode != 429
}
