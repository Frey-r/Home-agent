package llmkit_test

import (
	"context"
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/Frey-r/Home-agent/contracts"
	"github.com/Frey-r/Home-agent/llmkit"
	"github.com/Frey-r/Home-agent/llmkit/llmtest"
)

func TestOpenAI_SingleTurn(t *testing.T) {
	srv := llmtest.New(t)
	srv.RespondJSON(http.StatusOK, map[string]any{
		"id":      "chatcmpl-test-1",
		"object":  "chat.completion",
		"created": 1700000000,
		"model":   "gpt-4o",
		"choices": []map[string]any{
			{
				"index": 0,
				"message": map[string]any{
					"role":    "assistant",
					"content": "¡Hola! ¿En qué puedo ayudarte?",
				},
				"finish_reason": "stop",
			},
		},
		"usage": map[string]any{
			"prompt_tokens":     15,
			"completion_tokens": 8,
			"total_tokens":      23,
			"prompt_tokens_details": map[string]any{
				"cached_tokens": 5,
			},
		},
	})

	client, err := llmkit.NewOpenAIClient(llmkit.OpenAIConfig{
		BaseURL: srv.URL + "/v1",
		APIKey:  "test-api-key",
		Model:   "gpt-4o",
	})
	if err != nil {
		t.Fatalf("NewOpenAIClient error: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	resp, err := client.Complete(ctx, contracts.LLMRequest{
		Messages: []contracts.Message{
			contracts.UserMessage("Hola"),
		},
	})
	if err != nil {
		t.Fatalf("Complete error: %v", err)
	}

	if resp.Text() != "¡Hola! ¿En qué puedo ayudarte?" {
		t.Errorf("Text: got %q, want '¡Hola! ¿En qué puedo ayudarte?'", resp.Text())
	}
	if resp.StopReason != contracts.StopReasonEnd {
		t.Errorf("StopReason: got %s, want end", resp.StopReason)
	}
	if resp.Usage.InputTokens != 15 || resp.Usage.OutputTokens != 8 || resp.Usage.CacheReadTokens != 5 {
		t.Errorf("Usage mismatch: %+v", resp.Usage)
	}

	// Verificar la petición registrada
	req, ok := srv.LastRequest()
	if !ok {
		t.Fatal("no request recorded")
	}
	if req.Header.Get("Authorization") != "Bearer test-api-key" {
		t.Errorf("Authorization header missing or invalid: %s", req.Header.Get("Authorization"))
	}
}

func TestOpenAI_MultiTurn_SystemPrompt(t *testing.T) {
	srv := llmtest.New(t)
	srv.RespondJSON(http.StatusOK, map[string]any{
		"choices": []map[string]any{
			{
				"message": map[string]any{
					"role":    "assistant",
					"content": "Soy un asistente del hogar.",
				},
				"finish_reason": "stop",
			},
		},
		"usage": map[string]any{
			"prompt_tokens":     30,
			"completion_tokens": 7,
		},
	})

	client, _ := llmkit.NewOpenAIClient(llmkit.OpenAIConfig{
		BaseURL: srv.URL + "/v1",
		APIKey:  "test-key",
		Model:   "gpt-4o",
	})

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	_, err := client.Complete(ctx, contracts.LLMRequest{
		System: "Instrucción de sistema secreta",
		Messages: []contracts.Message{
			contracts.UserMessage("¿Quién eres?"),
		},
	})
	if err != nil {
		t.Fatalf("Complete error: %v", err)
	}

	req, _ := srv.LastRequest()
	var payload struct {
		Messages []struct {
			Role    string `json:"role"`
			Content string `json:"content"`
		} `json:"messages"`
	}
	if err := req.UnmarshalBody(&payload); err != nil {
		t.Fatalf("UnmarshalBody error: %v", err)
	}

	if len(payload.Messages) != 2 {
		t.Fatalf("Messages len = %d, want 2 (system + user)", len(payload.Messages))
	}
	if payload.Messages[0].Role != "system" || payload.Messages[0].Content != "Instrucción de sistema secreta" {
		t.Errorf("System message not correctly mapped: %+v", payload.Messages[0])
	}
	if payload.Messages[1].Role != "user" || payload.Messages[1].Content != "¿Quién eres?" {
		t.Errorf("User message not correctly mapped: %+v", payload.Messages[1])
	}
}

func TestOpenAI_ToolCalling_SimpleAndParallel(t *testing.T) {
	srv := llmtest.New(t)
	srv.RespondJSON(http.StatusOK, map[string]any{
		"choices": []map[string]any{
			{
				"message": map[string]any{
					"role": "assistant",
					"tool_calls": []map[string]any{
						{
							"id":   "call_1",
							"type": "function",
							"function": map[string]any{
								"name":      "light_set_power",
								"arguments": "{\"state\":\"on\"}",
							},
						},
						{
							"id":   "call_2",
							"type": "function",
							"function": map[string]any{
								"name":      "cover_open",
								"arguments": "{\"position\":100}",
							},
						},
					},
				},
				"finish_reason": "tool_calls",
			},
		},
		"usage": map[string]any{
			"prompt_tokens":     45,
			"completion_tokens": 20,
		},
	})

	client, _ := llmkit.NewOpenAIClient(llmkit.OpenAIConfig{
		BaseURL: srv.URL + "/v1",
		APIKey:  "key",
		Model:   "gpt-4o",
	})

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	resp, err := client.Complete(ctx, contracts.LLMRequest{
		Messages: []contracts.Message{
			contracts.UserMessage("Enciende la luz y abre la persiana"),
		},
		Tools: []contracts.LLMTool{
			{
				Name:        "light_set_power",
				Description: "luz",
				Parameters:  map[string]any{"type": "object"},
			},
		},
	})
	if err != nil {
		t.Fatalf("Complete error: %v", err)
	}

	if resp.StopReason != contracts.StopReasonToolCalls {
		t.Errorf("StopReason: got %s, want tool_calls", resp.StopReason)
	}

	calls := resp.ToolCalls()
	if len(calls) != 2 {
		t.Fatalf("ToolCalls count: got %d, want 2", len(calls))
	}
	if calls[0].ID != "call_1" || calls[0].Name != "light_set_power" {
		t.Errorf("Call 0 mismatch: %+v", calls[0])
	}
	if calls[1].ID != "call_2" || calls[1].Name != "cover_open" {
		t.Errorf("Call 1 mismatch: %+v", calls[1])
	}
}

func TestOpenAI_ToolResult_Resume(t *testing.T) {
	srv := llmtest.New(t)
	srv.RespondJSON(http.StatusOK, map[string]any{
		"choices": []map[string]any{
			{
				"message": map[string]any{
					"role":    "assistant",
					"content": "Luz encendida correctamente.",
				},
				"finish_reason": "stop",
			},
		},
		"usage": map[string]any{"prompt_tokens": 60, "completion_tokens": 6},
	})

	client, _ := llmkit.NewOpenAIClient(llmkit.OpenAIConfig{
		BaseURL: srv.URL + "/v1",
		APIKey:  "key",
		Model:   "gpt-4o",
	})

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	resp, err := client.Complete(ctx, contracts.LLMRequest{
		Messages: []contracts.Message{
			contracts.UserMessage("Enciende la luz"),
			{
				Role: contracts.RoleAssistant,
				Content: []contracts.ContentBlock{
					contracts.ToolCallBlock("call_1", "light_set_power", "{\"state\":\"on\"}"),
				},
			},
			{
				Role: contracts.RoleUser,
				Content: []contracts.ContentBlock{
					contracts.ToolResultBlock("call_1", "{\"status\":\"ok\"}", false),
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("Complete error: %v", err)
	}

	if resp.Text() != "Luz encendida correctamente." {
		t.Errorf("Text: got %q", resp.Text())
	}

	// Verificar que el tool result fue mapeado a role: tool con tool_call_id
	req, _ := srv.LastRequest()
	var payload struct {
		Messages []struct {
			Role       string `json:"role"`
			Content    string `json:"content"`
			ToolCallID string `json:"tool_call_id,omitempty"`
		} `json:"messages"`
	}
	_ = req.UnmarshalBody(&payload)

	var foundToolRole bool
	for _, m := range payload.Messages {
		if m.Role == "tool" && m.ToolCallID == "call_1" && m.Content == "{\"status\":\"ok\"}" {
			foundToolRole = true
			break
		}
	}
	if !foundToolRole {
		t.Errorf("tool_result message not found in payload: %+v", payload.Messages)
	}
}

func TestOpenAI_Streaming_Text(t *testing.T) {
	srv := llmtest.New(t)
	srv.RespondSSE([]string{
		"data: {\"choices\":[{\"delta\":{\"content\":\"Hola\"}}]}\n\n",
		"data: {\"choices\":[{\"delta\":{\"content\":\" mundo\"}}]}\n\n",
		"data: {\"choices\":[{\"delta\":{},\"finish_reason\":\"stop\"}],\"usage\":{\"prompt_tokens\":10,\"completion_tokens\":5}}\n\n",
		"data: [DONE]\n\n",
	})

	client, _ := llmkit.NewOpenAIClient(llmkit.OpenAIConfig{
		BaseURL: srv.URL + "/v1",
		APIKey:  "key",
		Model:   "gpt-4o",
	})

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	eventCh, errCh := client.Stream(ctx, contracts.LLMRequest{
		Messages: []contracts.Message{contracts.UserMessage("hola")},
	})

	var accumulatedText string
	var finalEnd *contracts.LLMStreamEvent

	for eventCh != nil || errCh != nil {
		select {
		case ev, ok := <-eventCh:
			if !ok {
				eventCh = nil
				continue
			}
			if ev.Type == contracts.StreamEventTypeTextDelta {
				accumulatedText += ev.Text
			} else if ev.Type == contracts.StreamEventTypeEnd {
				finalEnd = &ev
			}
		case err, ok := <-errCh:
			if !ok {
				errCh = nil
				continue
			}
			if err != nil {
				t.Fatalf("Stream error: %v", err)
			}
		}
	}

	if accumulatedText != "Hola mundo" {
		t.Errorf("accumulatedText: got %q, want 'Hola mundo'", accumulatedText)
	}
	if finalEnd == nil {
		t.Fatal("expected final end stream event")
	}
	if finalEnd.StopReason != contracts.StopReasonEnd {
		t.Errorf("finalEnd StopReason: got %s, want end", finalEnd.StopReason)
	}
	if finalEnd.Usage.InputTokens != 10 || finalEnd.Usage.OutputTokens != 5 {
		t.Errorf("finalEnd Usage: %+v", finalEnd.Usage)
	}
}

func TestOpenAI_Streaming_ToolCalls(t *testing.T) {
	srv := llmtest.New(t)
	srv.RespondSSE([]string{
		"data: {\"choices\":[{\"delta\":{\"tool_calls\":[{\"index\":0,\"id\":\"call_99\",\"type\":\"function\",\"function\":{\"name\":\"light_turn_on\",\"arguments\":\"\"}}]}}]}\n\n",
		"data: {\"choices\":[{\"delta\":{\"tool_calls\":[{\"index\":0,\"function\":{\"arguments\":\"{\\\"brightness\\\":\"}}]}}]}\n\n",
		"data: {\"choices\":[{\"delta\":{\"tool_calls\":[{\"index\":0,\"function\":{\"arguments\":\"80}\"}}]}}]}\n\n",
		"data: {\"choices\":[{\"delta\":{},\"finish_reason\":\"tool_calls\"}],\"usage\":{\"prompt_tokens\":20,\"completion_tokens\":10}}\n\n",
		"data: [DONE]\n\n",
	})

	client, _ := llmkit.NewOpenAIClient(llmkit.OpenAIConfig{
		BaseURL: srv.URL + "/v1",
		APIKey:  "key",
		Model:   "gpt-4o",
	})

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	eventCh, errCh := client.Stream(ctx, contracts.LLMRequest{
		Messages: []contracts.Message{contracts.UserMessage("luz al 80%")},
	})

	var assembledArgs string
	var toolName string
	var callID string
	var finalEnd *contracts.LLMStreamEvent

	for eventCh != nil || errCh != nil {
		select {
		case ev, ok := <-eventCh:
			if !ok {
				eventCh = nil
				continue
			}
			if ev.Type == contracts.StreamEventTypeToolCallDelta {
				if ev.ToolName != "" {
					toolName = ev.ToolName
				}
				if ev.ToolCallID != "" {
					callID = ev.ToolCallID
				}
				assembledArgs += ev.ArgumentsDelta
			} else if ev.Type == contracts.StreamEventTypeEnd {
				finalEnd = &ev
			}
		case err, ok := <-errCh:
			if !ok {
				errCh = nil
				continue
			}
			if err != nil {
				t.Fatalf("Stream error: %v", err)
			}
		}
	}

	if callID != "call_99" {
		t.Errorf("callID: got %q, want call_99", callID)
	}
	if toolName != "light_turn_on" {
		t.Errorf("toolName: got %q, want light_turn_on", toolName)
	}
	if assembledArgs != "{\"brightness\":80}" {
		t.Errorf("assembledArgs: got %q, want '{\"brightness\":80}'", assembledArgs)
	}
	if finalEnd == nil || finalEnd.StopReason != contracts.StopReasonToolCalls {
		t.Errorf("finalEnd stop reason: got %+v", finalEnd)
	}
}

func TestOpenAI_HTTPError_429_5xx(t *testing.T) {
	srv := llmtest.New(t)

	client, _ := llmkit.NewOpenAIClient(llmkit.OpenAIConfig{
		BaseURL: srv.URL + "/v1",
		APIKey:  "key",
		Model:   "gpt-4o",
	})

	t.Run("429 rate limit error", func(t *testing.T) {
		srv.SetHandler(func(r *http.Request, body []byte) (int, http.Header, []byte) {
			h := make(http.Header)
			h.Set("Retry-After", "3")
			return http.StatusTooManyRequests, h, []byte(`{"error":{"message":"Rate limit exceeded","type":"tokens"}}`)
		})

		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()

		_, err := client.Complete(ctx, contracts.LLMRequest{
			Messages: []contracts.Message{contracts.UserMessage("hola")},
		})
		if err == nil {
			t.Fatal("expected 429 error, got nil")
		}

		var apiErr *llmkit.APIError
		if !errors.As(err, &apiErr) {
			t.Fatalf("expected *llmkit.APIError, got %T: %v", err, err)
		}
		if apiErr.StatusCode != 429 {
			t.Errorf("StatusCode: got %d, want 429", apiErr.StatusCode)
		}
		if !apiErr.IsRateLimit() {
			t.Errorf("IsRateLimit should be true")
		}
	})

	t.Run("500 server error", func(t *testing.T) {
		srv.SetHandler(func(r *http.Request, body []byte) (int, http.Header, []byte) {
			return http.StatusInternalServerError, nil, []byte(`{"error":{"message":"Internal error"}}`)
		})

		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()

		_, err := client.Complete(ctx, contracts.LLMRequest{
			Messages: []contracts.Message{contracts.UserMessage("hola")},
		})
		if err == nil {
			t.Fatal("expected 500 error, got nil")
		}

		var apiErr *llmkit.APIError
		if !errors.As(err, &apiErr) {
			t.Fatalf("expected *llmkit.APIError, got %T: %v", err, err)
		}
		if !apiErr.IsServerError() {
			t.Errorf("IsServerError should be true")
		}
	})
}
