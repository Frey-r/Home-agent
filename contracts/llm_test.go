package contracts_test

import (
	"encoding/json"
	"testing"

	"github.com/Frey-r/Home-agent/contracts"
)

func TestLLMSchema_Compiles(t *testing.T) {
	v, err := contracts.NewValidator()
	if err != nil {
		t.Fatalf("NewValidator falló al compilar esquemas: %v", err)
	}
	if v == nil {
		t.Fatal("Validator no debería ser nulo")
	}
}

func TestLLMSchema_ValidRequest(t *testing.T) {
	v, err := contracts.NewValidator()
	if err != nil {
		t.Fatalf("NewValidator error: %v", err)
	}

	temp := 0.7
	maxTokens := 1024

	req := contracts.LLMRequest{
		System: "Eres un asistente del hogar inteligente.",
		Messages: []contracts.Message{
			contracts.UserMessage("Enciende la luz del salón"),
			{
				Role: contracts.RoleAssistant,
				Content: []contracts.ContentBlock{
					contracts.ToolCallBlock("call_123", "light_set_power", map[string]any{"state": "on"}),
				},
			},
			{
				Role: contracts.RoleUser,
				Content: []contracts.ContentBlock{
					contracts.ToolResultBlock("call_123", "{\"status\": \"ok\"}", false),
				},
			},
			contracts.AssistantMessage("La luz del salón ya está encendida."),
		},
		Tools: []contracts.LLMTool{
			{
				Name:        "light_set_power",
				Description: "Controla el encendido de luces",
				Parameters: map[string]any{
					"type": "object",
					"properties": map[string]any{
						"state": map[string]any{
							"type": "string",
							"enum": []string{"on", "off"},
						},
					},
					"required": []string{"state"},
				},
			},
		},
		Options: contracts.LLMRequestOptions{
			Temperature: &temp,
			MaxTokens:   &maxTokens,
			ToolChoice:  "auto",
		},
	}

	if err := v.ValidateLLMRequest(req); err != nil {
		t.Fatalf("solicitud válida fue rechazada por el validador: %v", err)
	}
}

func TestLLMSchema_ValidResponse(t *testing.T) {
	v, err := contracts.NewValidator()
	if err != nil {
		t.Fatalf("NewValidator error: %v", err)
	}

	resp := contracts.LLMResponse{
		Message: contracts.Message{
			Role: contracts.RoleAssistant,
			Content: []contracts.ContentBlock{
				contracts.TextBlock("Voy a activar la luz."),
				contracts.ToolCallBlock("call_abc", "light_set_power", map[string]any{"state": "on"}),
			},
		},
		StopReason: contracts.StopReasonToolCalls,
		Usage: contracts.LLMUsage{
			InputTokens:      120,
			OutputTokens:     35,
			CacheReadTokens:  80,
			CacheWriteTokens: 0,
		},
	}

	if err := v.ValidateLLMResponse(resp); err != nil {
		t.Fatalf("respuesta válida fue rechazada por el validador: %v", err)
	}

	if resp.Text() != "Voy a activar la luz." {
		t.Errorf("Text() = %q, want 'Voy a activar la luz.'", resp.Text())
	}
	if len(resp.ToolCalls()) != 1 {
		t.Fatalf("ToolCalls() len = %d, want 1", len(resp.ToolCalls()))
	}
	argsJSON, err := resp.ToolCalls()[0].ArgumentsJSON()
	if err != nil {
		t.Fatalf("ArgumentsJSON() error: %v", err)
	}
	if argsJSON == "" {
		t.Error("ArgumentsJSON no debería estar vacío")
	}
}

func TestLLMSchema_InvalidFixtures(t *testing.T) {
	v, err := contracts.NewValidator()
	if err != nil {
		t.Fatalf("NewValidator error: %v", err)
	}

	tests := []struct {
		name    string
		payload string
	}{
		{
			name:    "solicitud sin mensajes",
			payload: `{"system": "test", "messages": []}`,
		},
		{
			name:    "mensaje con rol desconocido",
			payload: `{"messages": [{"role": "robot", "content": [{"type": "text", "text": "hola"}]}]}`,
		},
		{
			name:    "bloque de contenido con tipo inválido",
			payload: `{"messages": [{"role": "user", "content": [{"type": "audio", "data": "abc"}]}]}`,
		},
		{
			name:    "tool_call sin id ni name",
			payload: `{"messages": [{"role": "assistant", "content": [{"type": "tool_call"}]}]}`,
		},
		{
			name:    "respuesta sin stop_reason",
			payload: `{"message": {"role": "assistant", "content": [{"type": "text", "text": "ok"}]}, "usage": {"input_tokens": 10, "output_tokens": 5}}`,
		},
		{
			name:    "respuesta con usage negativo",
			payload: `{"message": {"role": "assistant", "content": [{"type": "text", "text": "ok"}]}, "stop_reason": "end", "usage": {"input_tokens": -5, "output_tokens": 5}}`,
		},
		{
			name:    "temperatura fuera de rango",
			payload: `{"messages": [{"role": "user", "content": [{"type": "text", "text": "hola"}]}], "options": {"temperature": 3.5}}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := v.ValidateLLM([]byte(tt.payload))
			if err == nil {
				t.Fatalf("se esperaba que el validador fallara para payload inválido: %s", tt.payload)
			}
		})
	}
}

func TestLLM_RoundTrip(t *testing.T) {
	req := contracts.LLMRequest{
		System: "Instrucción de prueba",
		Messages: []contracts.Message{
			contracts.UserMessage("¿Qué hora es?"),
			contracts.AssistantMessage("Son las 4 PM."),
		},
	}

	raw, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("Marshal error: %v", err)
	}

	var req2 contracts.LLMRequest
	if err := json.Unmarshal(raw, &req2); err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}

	if req2.System != req.System {
		t.Errorf("System: got %q, want %q", req2.System, req.System)
	}
	if len(req2.Messages) != 2 {
		t.Fatalf("Messages len: got %d, want 2", len(req2.Messages))
	}
	if req2.Messages[0].TextContent() != "¿Qué hora es?" {
		t.Errorf("Message[0] text: got %q, want '¿Qué hora es?'", req2.Messages[0].TextContent())
	}
}
