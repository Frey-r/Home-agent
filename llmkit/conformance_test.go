package llmkit_test

import (
	"context"
	"fmt"
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/Frey-r/Home-agent/contracts"
	"github.com/Frey-r/Home-agent/llmkit"
	"github.com/Frey-r/Home-agent/llmkit/llmtest"
	"go.uber.org/goleak"
)

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}

type adapterHarness struct {
	name             string
	newClient        func(t *testing.T, srv *llmtest.Server) llmkit.Client
	mockTextResp     func(srv *llmtest.Server, text string, usage contracts.LLMUsage, stop contracts.StopReason)
	mockToolCallResp func(srv *llmtest.Server, calls []contracts.ContentBlock, usage contracts.LLMUsage)
	mockStreamText   func(srv *llmtest.Server, deltas []string, usage contracts.LLMUsage)
	mockStreamTool   func(srv *llmtest.Server, id, name string, argChunks []string, usage contracts.LLMUsage)
}

func getHarnesses() []adapterHarness {
	return []adapterHarness{
		{
			name: "OpenAI",
			newClient: func(t *testing.T, srv *llmtest.Server) llmkit.Client {
				c, err := llmkit.NewOpenAIClient(llmkit.OpenAIConfig{
					BaseURL: srv.URL + "/v1",
					APIKey:  "test-openai-key",
					Model:   "gpt-4o",
				})
				if err != nil {
					t.Fatalf("NewOpenAIClient: %v", err)
				}
				return c
			},
			mockTextResp: func(srv *llmtest.Server, text string, usage contracts.LLMUsage, stop contracts.StopReason) {
				finishReason := "stop"
				switch stop {
				case contracts.StopReasonMaxTokens:
					finishReason = "length"
				case contracts.StopReasonRefusal:
					finishReason = "content_filter"
				}
				srv.RespondJSON(http.StatusOK, map[string]any{
					"choices": []map[string]any{
						{
							"message": map[string]any{
								"role":    "assistant",
								"content": text,
							},
							"finish_reason": finishReason,
						},
					},
					"usage": map[string]any{
						"prompt_tokens":     usage.InputTokens,
						"completion_tokens": usage.OutputTokens,
						"prompt_tokens_details": map[string]any{
							"cached_tokens": usage.CacheReadTokens,
						},
					},
				})
			},
			mockToolCallResp: func(srv *llmtest.Server, calls []contracts.ContentBlock, usage contracts.LLMUsage) {
				var tcList []map[string]any
				for _, c := range calls {
					argsStr, _ := c.ArgumentsJSON()
					tcList = append(tcList, map[string]any{
						"id":   c.ID,
						"type": "function",
						"function": map[string]any{
							"name":      c.Name,
							"arguments": argsStr,
						},
					})
				}
				srv.RespondJSON(http.StatusOK, map[string]any{
					"choices": []map[string]any{
						{
							"message": map[string]any{
								"role":       "assistant",
								"tool_calls": tcList,
							},
							"finish_reason": "tool_calls",
						},
					},
					"usage": map[string]any{
						"prompt_tokens":     usage.InputTokens,
						"completion_tokens": usage.OutputTokens,
					},
				})
			},
			mockStreamText: func(srv *llmtest.Server, deltas []string, usage contracts.LLMUsage) {
				var events []string
				for _, d := range deltas {
					events = append(events, fmt.Sprintf("data: {\"choices\":[{\"delta\":{\"content\":%q}}]}\n\n", d))
				}
				events = append(events, fmt.Sprintf("data: {\"choices\":[{\"finish_reason\":\"stop\"}],\"usage\":{\"prompt_tokens\":%d,\"completion_tokens\":%d,\"prompt_tokens_details\":{\"cached_tokens\":%d}}}\n\n",
					usage.InputTokens, usage.OutputTokens, usage.CacheReadTokens))
				events = append(events, "data: [DONE]\n\n")
				srv.RespondSSE(events)
			},
			mockStreamTool: func(srv *llmtest.Server, id, name string, argChunks []string, usage contracts.LLMUsage) {
				var events []string
				for i, chunk := range argChunks {
					if i == 0 {
						events = append(events, fmt.Sprintf("data: {\"choices\":[{\"delta\":{\"tool_calls\":[{\"index\":0,\"id\":%q,\"function\":{\"name\":%q,\"arguments\":%q}}]}}]}\n\n", id, name, chunk))
					} else {
						events = append(events, fmt.Sprintf("data: {\"choices\":[{\"delta\":{\"tool_calls\":[{\"index\":0,\"function\":{\"arguments\":%q}}]}}]}\n\n", chunk))
					}
				}
				events = append(events, fmt.Sprintf("data: {\"choices\":[{\"finish_reason\":\"tool_calls\"}],\"usage\":{\"prompt_tokens\":%d,\"completion_tokens\":%d}}\n\n",
					usage.InputTokens, usage.OutputTokens))
				events = append(events, "data: [DONE]\n\n")
				srv.RespondSSE(events)
			},
		},
		{
			name: "Anthropic",
			newClient: func(t *testing.T, srv *llmtest.Server) llmkit.Client {
				c, err := llmkit.NewAnthropicClient(llmkit.AnthropicConfig{
					BaseURL: srv.URL + "/v1",
					APIKey:  "test-anthropic-key",
					Model:   "claude-3-5-sonnet-latest",
				})
				if err != nil {
					t.Fatalf("NewAnthropicClient: %v", err)
				}
				return c
			},
			mockTextResp: func(srv *llmtest.Server, text string, usage contracts.LLMUsage, stop contracts.StopReason) {
				stopReason := "end_turn"
				switch stop {
				case contracts.StopReasonMaxTokens:
					stopReason = "max_tokens"
				}
				srv.RespondJSON(http.StatusOK, map[string]any{
					"role":        "assistant",
					"stop_reason": stopReason,
					"content": []map[string]any{
						{"type": "text", "text": text},
					},
					"usage": map[string]any{
						"input_tokens":                usage.InputTokens,
						"output_tokens":               usage.OutputTokens,
						"cache_read_input_tokens":     usage.CacheReadTokens,
						"cache_creation_input_tokens": usage.CacheWriteTokens,
					},
				})
			},
			mockToolCallResp: func(srv *llmtest.Server, calls []contracts.ContentBlock, usage contracts.LLMUsage) {
				var content []map[string]any
				for _, c := range calls {
					var input any = c.Arguments
					if str, ok := c.Arguments.(string); ok {
						input = map[string]any{"raw": str}
					}
					content = append(content, map[string]any{
						"type":  "tool_use",
						"id":    c.ID,
						"name":  c.Name,
						"input": input,
					})
				}
				srv.RespondJSON(http.StatusOK, map[string]any{
					"role":        "assistant",
					"stop_reason": "tool_use",
					"content":     content,
					"usage": map[string]any{
						"input_tokens":  usage.InputTokens,
						"output_tokens": usage.OutputTokens,
					},
				})
			},
			mockStreamText: func(srv *llmtest.Server, deltas []string, usage contracts.LLMUsage) {
				var events []string
				events = append(events, fmt.Sprintf("event: message_start\ndata: {\"type\":\"message_start\",\"message\":{\"usage\":{\"input_tokens\":%d,\"cache_read_input_tokens\":%d}}}\n\n",
					usage.InputTokens, usage.CacheReadTokens))
				for _, d := range deltas {
					events = append(events, fmt.Sprintf("event: content_block_delta\ndata: {\"type\":\"content_block_delta\",\"index\":0,\"delta\":{\"type\":\"text_delta\",\"text\":%q}}\n\n", d))
				}
				events = append(events, fmt.Sprintf("event: message_delta\ndata: {\"type\":\"message_delta\",\"delta\":{\"stop_reason\":\"end_turn\"},\"usage\":{\"output_tokens\":%d}}\n\n", usage.OutputTokens))
				events = append(events, "event: message_stop\ndata: {\"type\":\"message_stop\"}\n\n")
				srv.RespondSSE(events)
			},
			mockStreamTool: func(srv *llmtest.Server, id, name string, argChunks []string, usage contracts.LLMUsage) {
				var events []string
				events = append(events, fmt.Sprintf("event: message_start\ndata: {\"type\":\"message_start\",\"message\":{\"usage\":{\"input_tokens\":%d}}}\n\n", usage.InputTokens))
				events = append(events, fmt.Sprintf("event: content_block_start\ndata: {\"type\":\"content_block_start\",\"index\":0,\"content_block\":{\"type\":\"tool_use\",\"id\":%q,\"name\":%q}}\n\n", id, name))
				for _, chunk := range argChunks {
					events = append(events, fmt.Sprintf("event: content_block_delta\ndata: {\"type\":\"content_block_delta\",\"index\":0,\"delta\":{\"type\":\"input_json_delta\",\"partial_json\":%q}}\n\n", chunk))
				}
				events = append(events, fmt.Sprintf("event: message_delta\ndata: {\"type\":\"message_delta\",\"delta\":{\"stop_reason\":\"tool_use\"},\"usage\":{\"output_tokens\":%d}}\n\n", usage.OutputTokens))
				events = append(events, "event: message_stop\ndata: {\"type\":\"message_stop\"}\n\n")
				srv.RespondSSE(events)
			},
		},
	}
}

func TestConformance_SingleTurn(t *testing.T) {
	for _, h := range getHarnesses() {
		t.Run(h.name, func(t *testing.T) {
			srv := llmtest.New(t)
			h.mockTextResp(srv, "Hola mundo", contracts.LLMUsage{InputTokens: 10, OutputTokens: 5, CacheReadTokens: 2}, contracts.StopReasonEnd)
			client := h.newClient(t, srv)

			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()

			resp, err := client.Complete(ctx, contracts.LLMRequest{
				Messages: []contracts.Message{contracts.UserMessage("Di hola")},
			})
			if err != nil {
				t.Fatalf("[%s] Complete error: %v", h.name, err)
			}

			if resp.Text() != "Hola mundo" {
				t.Errorf("[%s] Text mismatch: got %q", h.name, resp.Text())
			}
			if resp.StopReason != contracts.StopReasonEnd {
				t.Errorf("[%s] StopReason mismatch: got %s", h.name, resp.StopReason)
			}
			if resp.Usage.InputTokens != 10 || resp.Usage.OutputTokens != 5 || resp.Usage.CacheReadTokens != 2 {
				t.Errorf("[%s] Usage mismatch: %+v", h.name, resp.Usage)
			}
		})
	}
}

func TestConformance_MultiTurn_SystemPrompt(t *testing.T) {
	for _, h := range getHarnesses() {
		t.Run(h.name, func(t *testing.T) {
			srv := llmtest.New(t)
			h.mockTextResp(srv, "Respuesta multi-turn", contracts.LLMUsage{InputTokens: 30, OutputTokens: 10}, contracts.StopReasonEnd)
			client := h.newClient(t, srv)

			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()

			resp, err := client.Complete(ctx, contracts.LLMRequest{
				System: "Eres un asistente del hogar.",
				Messages: []contracts.Message{
					contracts.UserMessage("Mensaje 1"),
					contracts.AssistantMessage("Respuesta 1"),
					contracts.UserMessage("Mensaje 2"),
				},
			})
			if err != nil {
				t.Fatalf("[%s] Complete error: %v", h.name, err)
			}
			if resp.Text() != "Respuesta multi-turn" {
				t.Errorf("[%s] Text mismatch: got %q", h.name, resp.Text())
			}

			req, ok := srv.LastRequest()
			if !ok {
				t.Fatalf("[%s] no request recorded", h.name)
			}
			if len(req.Body) == 0 {
				t.Fatalf("[%s] empty body recorded", h.name)
			}
		})
	}
}

func TestConformance_ToolCalling_SimpleAndParallel(t *testing.T) {
	for _, h := range getHarnesses() {
		t.Run(h.name, func(t *testing.T) {
			srv := llmtest.New(t)
			expectedCalls := []contracts.ContentBlock{
				contracts.ToolCallBlock("call_1", "light_turn_on", map[string]any{"entity_id": "light.living"}),
				contracts.ToolCallBlock("call_2", "climate_set_temp", map[string]any{"temperature": 21}),
			}
			h.mockToolCallResp(srv, expectedCalls, contracts.LLMUsage{InputTokens: 40, OutputTokens: 20})
			client := h.newClient(t, srv)

			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()

			resp, err := client.Complete(ctx, contracts.LLMRequest{
				Messages: []contracts.Message{contracts.UserMessage("enciende la luz y pon el clima a 21")},
				Tools: []contracts.LLMTool{
					{Name: "light_turn_on", Description: "luz", Parameters: map[string]any{"type": "object"}},
					{Name: "climate_set_temp", Description: "clima", Parameters: map[string]any{"type": "object"}},
				},
			})
			if err != nil {
				t.Fatalf("[%s] Complete error: %v", h.name, err)
			}

			if resp.StopReason != contracts.StopReasonToolCalls {
				t.Errorf("[%s] StopReason: got %s, want tool_calls", h.name, resp.StopReason)
			}
			calls := resp.ToolCalls()
			if len(calls) != 2 {
				t.Fatalf("[%s] ToolCalls count: got %d, want 2", h.name, len(calls))
			}
			if calls[0].Name != "light_turn_on" || calls[1].Name != "climate_set_temp" {
				t.Errorf("[%s] Calls mismatch: %+v", h.name, calls)
			}
		})
	}
}

func TestConformance_ToolResult_Resume(t *testing.T) {
	for _, h := range getHarnesses() {
		t.Run(h.name, func(t *testing.T) {
			srv := llmtest.New(t)
			h.mockTextResp(srv, "Luz encendida exitosamente.", contracts.LLMUsage{InputTokens: 50, OutputTokens: 8}, contracts.StopReasonEnd)
			client := h.newClient(t, srv)

			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()

			resp, err := client.Complete(ctx, contracts.LLMRequest{
				Messages: []contracts.Message{
					contracts.UserMessage("Enciende la luz"),
					{
						Role:    contracts.RoleAssistant,
						Content: []contracts.ContentBlock{contracts.ToolCallBlock("c_1", "light_turn_on", map[string]any{"entity_id": "light.living"})},
					},
					{
						Role:    contracts.RoleUser,
						Content: []contracts.ContentBlock{contracts.ToolResultBlock("c_1", "{\"status\":\"ok\"}", false)},
					},
				},
			})
			if err != nil {
				t.Fatalf("[%s] Complete error: %v", h.name, err)
			}
			if resp.Text() != "Luz encendida exitosamente." {
				t.Errorf("[%s] Text: got %q", h.name, resp.Text())
			}
		})
	}
}

func TestConformance_Streaming_Text(t *testing.T) {
	for _, h := range getHarnesses() {
		t.Run(h.name, func(t *testing.T) {
			srv := llmtest.New(t)
			deltas := []string{"Hola ", "desde ", "streaming!"}
			h.mockStreamText(srv, deltas, contracts.LLMUsage{InputTokens: 12, OutputTokens: 6, CacheReadTokens: 3})
			client := h.newClient(t, srv)

			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()

			eventCh, errCh := client.Stream(ctx, contracts.LLMRequest{
				Messages: []contracts.Message{contracts.UserMessage("Hola")},
			})

			var textAcc string
			var finalEnd *contracts.LLMStreamEvent

			for eventCh != nil || errCh != nil {
				select {
				case ev, ok := <-eventCh:
					if !ok {
						eventCh = nil
						continue
					}
					if ev.Type == contracts.StreamEventTypeTextDelta {
						textAcc += ev.Text
					} else if ev.Type == contracts.StreamEventTypeEnd {
						finalEnd = &ev
					}
				case err, ok := <-errCh:
					if !ok {
						errCh = nil
						continue
					}
					if err != nil {
						t.Fatalf("[%s] Stream error: %v", h.name, err)
					}
				}
			}

			if textAcc != "Hola desde streaming!" {
				t.Errorf("[%s] textAcc: got %q, want 'Hola desde streaming!'", h.name, textAcc)
			}
			if finalEnd == nil {
				t.Fatalf("[%s] expected finalEnd event", h.name)
			}
			if finalEnd.StopReason != contracts.StopReasonEnd {
				t.Errorf("[%s] StopReason: got %s, want end", h.name, finalEnd.StopReason)
			}
			if finalEnd.Usage.InputTokens != 12 || finalEnd.Usage.OutputTokens != 6 {
				t.Errorf("[%s] Usage mismatch: %+v", h.name, finalEnd.Usage)
			}
		})
	}
}

func TestConformance_Streaming_ToolCalls(t *testing.T) {
	for _, h := range getHarnesses() {
		t.Run(h.name, func(t *testing.T) {
			srv := llmtest.New(t)
			chunks := []string{"{\"id\":", "\"light.living\",", "\"brightness\":100}"}
			h.mockStreamTool(srv, "tool_s9", "set_light", chunks, contracts.LLMUsage{InputTokens: 20, OutputTokens: 15})
			client := h.newClient(t, srv)

			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()

			eventCh, errCh := client.Stream(ctx, contracts.LLMRequest{
				Messages: []contracts.Message{contracts.UserMessage("Luz al 100")},
			})

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
					if ev.Type == contracts.StreamEventTypeToolCallDelta {
						if ev.ToolName != "" {
							toolName = ev.ToolName
						}
						toolArgs += ev.ArgumentsDelta
					} else if ev.Type == contracts.StreamEventTypeEnd {
						finalEnd = &ev
					}
				case err, ok := <-errCh:
					if !ok {
						errCh = nil
						continue
					}
					if err != nil {
						t.Fatalf("[%s] Stream error: %v", h.name, err)
					}
				}
			}

			if toolName != "set_light" {
				t.Errorf("[%s] toolName: got %q, want set_light", h.name, toolName)
			}
			expectedArgs := "{\"id\":\"light.living\",\"brightness\":100}"
			if toolArgs != expectedArgs {
				t.Errorf("[%s] toolArgs: got %q, want %q", h.name, toolArgs, expectedArgs)
			}
			if finalEnd == nil {
				t.Fatalf("[%s] expected finalEnd event", h.name)
			}
			if finalEnd.StopReason != contracts.StopReasonToolCalls {
				t.Errorf("[%s] StopReason: got %s, want tool_calls", h.name, finalEnd.StopReason)
			}
		})
	}
}

func TestConformance_StopReasons(t *testing.T) {
	reasons := []contracts.StopReason{
		contracts.StopReasonEnd,
		contracts.StopReasonMaxTokens,
	}

	for _, h := range getHarnesses() {
		for _, r := range reasons {
			testName := fmt.Sprintf("%s_%s", h.name, r)
			t.Run(testName, func(t *testing.T) {
				srv := llmtest.New(t)
				h.mockTextResp(srv, "Texto de parada", contracts.LLMUsage{InputTokens: 10, OutputTokens: 10}, r)
				client := h.newClient(t, srv)

				ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
				defer cancel()

				resp, err := client.Complete(ctx, contracts.LLMRequest{
					Messages: []contracts.Message{contracts.UserMessage("test")},
				})
				if err != nil {
					t.Fatalf("[%s] Complete error: %v", testName, err)
				}
				if resp.StopReason != r {
					t.Errorf("[%s] StopReason mismatch: got %s, want %s", testName, resp.StopReason, r)
				}
			})
		}
	}
}

func TestConformance_ContextCancel(t *testing.T) {
	for _, h := range getHarnesses() {
		t.Run(h.name, func(t *testing.T) {
			srv := llmtest.New(t)
			// Configurar handler bloqueante
			blockCh := make(chan struct{})
			srv.SetHandler(func(r *http.Request, body []byte) (int, http.Header, []byte) {
				<-blockCh
				return http.StatusOK, nil, []byte("{}")
			})
			t.Cleanup(func() {
				// Desbloquear por si acaso para no dejar goroutines colgadas en el server
				select {
				case <-blockCh:
				default:
					close(blockCh)
				}
			})

			client := h.newClient(t, srv)

			ctx, cancel := context.WithCancel(context.Background())
			var wg sync.WaitGroup
			wg.Add(1)

			go func() {
				defer wg.Done()
				time.Sleep(20 * time.Millisecond)
				cancel()
			}()

			_, err := client.Complete(ctx, contracts.LLMRequest{
				Messages: []contracts.Message{contracts.UserMessage("cancel me")},
			})
			wg.Wait()

			if err == nil {
				t.Fatalf("[%s] expected context cancellation error, got nil", h.name)
			}
		})
	}
}
