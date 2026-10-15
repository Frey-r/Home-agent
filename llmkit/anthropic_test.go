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

func TestAnthropic_SingleTurn(t *testing.T) {
	srv := llmtest.New(t)
	srv.RespondJSON(http.StatusOK, map[string]any{
		"id":           "msg_test_1",
		"type":         "message",
		"role":         "assistant",
		"model":        "claude-3-5-sonnet-latest",
		"stop_reason":  "end_turn",
		"stop_sequence": nil,
		"content": []map[string]any{
			{
				"type": "text",
				"text": "¡Hola! ¿Cómo estás?",
			},
		},
		"usage": map[string]any{
			"input_tokens":                18,
			"output_tokens":               9,
			"cache_creation_input_tokens": 10,
			"cache_read_input_tokens":     4,
		},
	})

	client, err := llmkit.NewAnthropicClient(llmkit.AnthropicConfig{
		BaseURL: srv.URL + "/v1",
		APIKey:  "anthropic-test-key",
		Model:   "claude-3-5-sonnet-latest",
	})
	if err != nil {
		t.Fatalf("NewAnthropicClient error: %v", err)
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

	if resp.Text() != "¡Hola! ¿Cómo estás?" {
		t.Errorf("Text: got %q, want '¡Hola! ¿Cómo estás?'", resp.Text())
	}
	if resp.StopReason != contracts.StopReasonEnd {
		t.Errorf("StopReason: got %s, want end", resp.StopReason)
	}
	if resp.Usage.InputTokens != 18 || resp.Usage.OutputTokens != 9 || resp.Usage.CacheReadTokens != 4 || resp.Usage.CacheWriteTokens != 10 {
		t.Errorf("Usage mismatch: %+v", resp.Usage)
	}

	// Verificar cabeceras registradas
	req, ok := srv.LastRequest()
	if !ok {
		t.Fatal("no request recorded")
	}
	if req.Header.Get("x-api-key") != "anthropic-test-key" {
		t.Errorf("x-api-key header missing or invalid: %s", req.Header.Get("x-api-key"))
	}
	if req.Header.Get("anthropic-version") != "2023-06-01" {
		t.Errorf("anthropic-version header missing: %s", req.Header.Get("anthropic-version"))
	}
}

func TestAnthropic_MultiTurn_SystemPrompt(t *testing.T) {
	srv := llmtest.New(t)
	srv.RespondJSON(http.StatusOK, map[string]any{
		"role":        "assistant",
		"stop_reason": "end_turn",
		"content": []map[string]any{
			{"type": "text", "text": "Entendido."},
		},
		"usage": map[string]any{"input_tokens": 25, "output_tokens": 5},
	})

	client, _ := llmkit.NewAnthropicClient(llmkit.AnthropicConfig{
		BaseURL: srv.URL + "/v1",
		APIKey:  "key",
		Model:   "claude-3-5-sonnet-latest",
	})

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	_, err := client.Complete(ctx, contracts.LLMRequest{
		System: "Eres Claude, un asistente doméstico.",
		Messages: []contracts.Message{
			contracts.UserMessage("Hola"),
		},
	})
	if err != nil {
		t.Fatalf("Complete error: %v", err)
	}

	req, _ := srv.LastRequest()
	var payload struct {
		System   string `json:"system"`
		Messages []struct {
			Role string `json:"role"`
		} `json:"messages"`
	}
	_ = req.UnmarshalBody(&payload)

	if payload.System != "Eres Claude, un asistente doméstico." {
		t.Errorf("System prompt must be at top-level in Anthropic payload: got %q", payload.System)
	}
	if len(payload.Messages) != 1 || payload.Messages[0].Role != "user" {
		t.Errorf("Messages must not contain system turn: %+v", payload.Messages)
	}
}

func TestAnthropic_ToolCalling_SimpleAndParallel(t *testing.T) {
	srv := llmtest.New(t)
	srv.RespondJSON(http.StatusOK, map[string]any{
		"role":        "assistant",
		"stop_reason": "tool_use",
		"content": []map[string]any{
			{
				"type": "text",
				"text": "Ejecutando comandos.",
			},
			{
				"type":  "tool_use",
				"id":    "toolu_1",
				"name":  "light_set_power",
				"input": map[string]any{"state": "on"},
			},
			{
				"type":  "tool_use",
				"id":    "toolu_2",
				"name":  "cover_close",
				"input": map[string]any{"position": 0},
			},
		},
		"usage": map[string]any{"input_tokens": 50, "output_tokens": 30},
	})

	client, _ := llmkit.NewAnthropicClient(llmkit.AnthropicConfig{
		BaseURL: srv.URL + "/v1",
		APIKey:  "key",
		Model:   "claude-3-5-sonnet-latest",
	})

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	resp, err := client.Complete(ctx, contracts.LLMRequest{
		Messages: []contracts.Message{contracts.UserMessage("Luz y persiana")},
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
	if calls[0].ID != "toolu_1" || calls[0].Name != "light_set_power" {
		t.Errorf("Call 0 mismatch: %+v", calls[0])
	}
	if calls[1].ID != "toolu_2" || calls[1].Name != "cover_close" {
		t.Errorf("Call 1 mismatch: %+v", calls[1])
	}

	// Verificar mapeo de herramientas a input_schema
	req, _ := srv.LastRequest()
	var payload struct {
		Tools []struct {
			Name        string         `json:"name"`
			Description string         `json:"description"`
			InputSchema map[string]any `json:"input_schema"`
		} `json:"tools"`
	}
	_ = req.UnmarshalBody(&payload)

	if len(payload.Tools) != 1 || payload.Tools[0].Name != "light_set_power" || payload.Tools[0].InputSchema == nil {
		t.Errorf("Tools not mapped to input_schema: %+v", payload.Tools)
	}
}

func TestAnthropic_ToolResult_Resume(t *testing.T) {
	srv := llmtest.New(t)
	srv.RespondJSON(http.StatusOK, map[string]any{
		"role":        "assistant",
		"stop_reason": "end_turn",
		"content": []map[string]any{
			{"type": "text", "text": "Listo, persiana cerrada."},
		},
		"usage": map[string]any{"input_tokens": 70, "output_tokens": 10},
	})

	client, _ := llmkit.NewAnthropicClient(llmkit.AnthropicConfig{
		BaseURL: srv.URL + "/v1",
		APIKey:  "key",
		Model:   "claude-3-5-sonnet-latest",
	})

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	resp, err := client.Complete(ctx, contracts.LLMRequest{
		Messages: []contracts.Message{
			contracts.UserMessage("Cierra persiana"),
			{
				Role: contracts.RoleAssistant,
				Content: []contracts.ContentBlock{
					contracts.ToolCallBlock("toolu_9", "cover_close", map[string]any{"position": 0}),
				},
			},
			{
				Role: contracts.RoleUser,
				Content: []contracts.ContentBlock{
					contracts.ToolResultBlock("toolu_9", "{\"status\":\"closed\"}", false),
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("Complete error: %v", err)
	}

	if resp.Text() != "Listo, persiana cerrada." {
		t.Errorf("Text: got %q", resp.Text())
	}

	// Verificar que el tool result va dentro de un bloque tool_result en turno user
	req, _ := srv.LastRequest()
	var payload struct {
		Messages []struct {
			Role    string `json:"role"`
			Content []struct {
				Type      string `json:"type"`
				ToolUseID string `json:"tool_use_id"`
				Content   string `json:"content"`
			} `json:"content"`
		} `json:"messages"`
	}
	_ = req.UnmarshalBody(&payload)

	lastMsg := payload.Messages[len(payload.Messages)-1]
	if lastMsg.Role != "user" || len(lastMsg.Content) == 0 || lastMsg.Content[0].Type != "tool_result" {
		t.Errorf("tool_result block not correctly serialized: %+v", lastMsg)
	}
	if lastMsg.Content[0].ToolUseID != "toolu_9" {
		t.Errorf("tool_use_id mismatch: %s", lastMsg.Content[0].ToolUseID)
	}
}

func TestAnthropic_Streaming_TextAndToolCalls(t *testing.T) {
	srv := llmtest.New(t)
	srv.RespondSSE([]string{
		"event: message_start\ndata: {\"type\":\"message_start\",\"message\":{\"id\":\"msg_sse\",\"role\":\"assistant\",\"usage\":{\"input_tokens\":25,\"output_tokens\":1,\"cache_read_input_tokens\":5}}}\n\n",
		"event: content_block_start\ndata: {\"type\":\"content_block_start\",\"index\":0,\"content_block\":{\"type\":\"text\",\"text\":\"\"}}\n\n",
		"event: content_block_delta\ndata: {\"type\":\"content_block_delta\",\"index\":0,\"delta\":{\"type\":\"text_delta\",\"text\":\"Accionando\"}}\n\n",
		"event: content_block_delta\ndata: {\"type\":\"content_block_delta\",\"index\":0,\"delta\":{\"type\":\"text_delta\",\"text\":\" dispositivo.\"}}\n\n",
		"event: content_block_stop\ndata: {\"type\":\"content_block_stop\",\"index\":0}\n\n",
		"event: content_block_start\ndata: {\"type\":\"content_block_start\",\"index\":1,\"content_block\":{\"type\":\"tool_use\",\"id\":\"toolu_s1\",\"name\":\"switch_turn_on\"}}\n\n",
		"event: content_block_delta\ndata: {\"type\":\"content_block_delta\",\"index\":1,\"delta\":{\"type\":\"input_json_delta\",\"partial_json\":\"{\\\"id\\\":\"}}\n\n",
		"event: content_block_delta\ndata: {\"type\":\"content_block_delta\",\"index\":1,\"delta\":{\"type\":\"input_json_delta\",\"partial_json\":\"\\\"switch.fan\\\"}\"}}\n\n",
		"event: content_block_stop\ndata: {\"type\":\"content_block_stop\",\"index\":1}\n\n",
		"event: message_delta\ndata: {\"type\":\"message_delta\",\"delta\":{\"stop_reason\":\"tool_use\"},\"usage\":{\"output_tokens\":15}}\n\n",
		"event: message_stop\ndata: {\"type\":\"message_stop\"}\n\n",
	})

	client, _ := llmkit.NewAnthropicClient(llmkit.AnthropicConfig{
		BaseURL: srv.URL + "/v1",
		APIKey:  "key",
		Model:   "claude-3-5-sonnet-latest",
	})

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	eventCh, errCh := client.Stream(ctx, contracts.LLMRequest{
		Messages: []contracts.Message{contracts.UserMessage("enciende ventilador")},
	})

	var textAcc string
	var toolName string
	var toolArgs string
	var finalEnd *contracts.LLMStreamEvent

	for eventCh != nil || errCh != nil {
		select {
		case ev, ok := <-eventCh:
			if !ok {
				eventCh = nil
				continue
			}
			switch ev.Type {
			case contracts.StreamEventTypeTextDelta:
				textAcc += ev.Text
			case contracts.StreamEventTypeToolCallDelta:
				if ev.ToolName != "" {
					toolName = ev.ToolName
				}
				toolArgs += ev.ArgumentsDelta
			case contracts.StreamEventTypeEnd:
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

	if textAcc != "Accionando dispositivo." {
		t.Errorf("textAcc: got %q", textAcc)
	}
	if toolName != "switch_turn_on" {
		t.Errorf("toolName: got %q", toolName)
	}
	if toolArgs != "{\"id\":\"switch.fan\"}" {
		t.Errorf("toolArgs: got %q", toolArgs)
	}
	if finalEnd == nil {
		t.Fatal("expected final end event")
	}
	if finalEnd.StopReason != contracts.StopReasonToolCalls {
		t.Errorf("finalEnd stop reason: got %s, want tool_calls", finalEnd.StopReason)
	}
	if finalEnd.Usage.InputTokens != 25 || finalEnd.Usage.OutputTokens != 15 || finalEnd.Usage.CacheReadTokens != 5 {
		t.Errorf("finalEnd usage mismatch: %+v", finalEnd.Usage)
	}
}

func TestAnthropic_HTTPError_429_5xx(t *testing.T) {
	srv := llmtest.New(t)

	client, _ := llmkit.NewAnthropicClient(llmkit.AnthropicConfig{
		BaseURL: srv.URL + "/v1",
		APIKey:  "key",
		Model:   "claude-3-5-sonnet-latest",
	})

	t.Run("429 rate limit error", func(t *testing.T) {
		srv.SetHandler(func(r *http.Request, body []byte) (int, http.Header, []byte) {
			return http.StatusTooManyRequests, nil, []byte(`{"type":"error","error":{"type":"rate_limit_error","message":"Number of request tokens has exceeded your daily rate limit."}}`)
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
		if apiErr.StatusCode != 429 || !apiErr.IsRateLimit() {
			t.Errorf("APIError mismatch: %+v", apiErr)
		}
	})

	t.Run("529 overloaded error", func(t *testing.T) {
		srv.SetHandler(func(r *http.Request, body []byte) (int, http.Header, []byte) {
			return 529, nil, []byte(`{"type":"error","error":{"type":"overloaded_error","message":"Anthropic is temporarily overloaded."}}`)
		})

		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()

		_, err := client.Complete(ctx, contracts.LLMRequest{
			Messages: []contracts.Message{contracts.UserMessage("hola")},
		})
		if err == nil {
			t.Fatal("expected 529 error, got nil")
		}

		var apiErr *llmkit.APIError
		if !errors.As(err, &apiErr) {
			t.Fatalf("expected *llmkit.APIError, got %T: %v", err, err)
		}
		if !apiErr.IsServerError() {
			t.Errorf("529 should be treated as IsServerError()")
		}
	})
}
