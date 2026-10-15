package llmkit

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/Frey-r/Home-agent/contracts"
)

// AnthropicConfig contiene la configuración para instanciar el cliente Anthropic.
type AnthropicConfig struct {
	BaseURL    string // Por defecto: https://api.anthropic.com/v1
	APIKey     string
	Model      string // Por defecto: claude-3-5-sonnet-latest
	HTTPClient *http.Client
}

// AnthropicClient implementa la interfaz Client para la REST API de Anthropic Messages.
type AnthropicClient struct {
	baseURL    string
	apiKey     string
	model      string
	httpClient *http.Client
}

// NewAnthropicClient crea un nuevo adaptador Anthropic para formato neutro.
func NewAnthropicClient(cfg AnthropicConfig) (*AnthropicClient, error) {
	baseURL := strings.TrimRight(cfg.BaseURL, "/")
	if baseURL == "" {
		baseURL = "https://api.anthropic.com/v1"
	}
	if cfg.Model == "" {
		cfg.Model = "claude-3-5-sonnet-latest"
	}
	httpClient := cfg.HTTPClient
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 60 * time.Second}
	}

	return &AnthropicClient{
		baseURL:    baseURL,
		apiKey:     cfg.APIKey,
		model:      cfg.Model,
		httpClient: httpClient,
	}, nil
}

// Complete ejecuta una llamada no-streaming a Messages API y la traduce a contracts.LLMResponse.
func (c *AnthropicClient) Complete(ctx context.Context, req contracts.LLMRequest) (contracts.LLMResponse, error) {
	payload, err := c.buildPayload(req, false)
	if err != nil {
		return contracts.LLMResponse{}, err
	}

	respBody, err := c.doRequest(ctx, payload)
	if err != nil {
		return contracts.LLMResponse{}, err
	}

	return c.parseResponse(respBody)
}

// Stream ejecuta una llamada streaming con SSE y traduce los eventos a contracts.LLMStreamEvent.
func (c *AnthropicClient) Stream(ctx context.Context, req contracts.LLMRequest) (<-chan contracts.LLMStreamEvent, <-chan error) {
	eventCh := make(chan contracts.LLMStreamEvent, 100)
	errCh := make(chan error, 1)

	payload, err := c.buildPayload(req, true)
	if err != nil {
		errCh <- err
		close(eventCh)
		close(errCh)
		return eventCh, errCh
	}

	go func() {
		defer close(eventCh)
		defer close(errCh)

		reqHTTP, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/messages", bytes.NewReader(payload))
		if err != nil {
			errCh <- fmt.Errorf("llmkit: crear peticion streaming Anthropic: %w", err)
			return
		}
		c.setHeaders(reqHTTP)

		resp, err := c.httpClient.Do(reqHTTP)
		if err != nil {
			errCh <- fmt.Errorf("llmkit: error en peticion HTTP Anthropic: %w", err)
			return
		}
		defer resp.Body.Close()

		if resp.StatusCode >= 400 {
			body, _ := io.ReadAll(resp.Body)
			errCh <- c.parseAPIError(resp.StatusCode, body)
			return
		}

		c.readSSEStream(ctx, resp.Body, eventCh, errCh)
	}()

	return eventCh, errCh
}

func (c *AnthropicClient) setHeaders(req *http.Request) {
	req.Header.Set("Content-Type", "application/json")
	if c.apiKey != "" {
		req.Header.Set("x-api-key", c.apiKey)
	}
	req.Header.Set("anthropic-version", "2023-06-01")
}

func (c *AnthropicClient) doRequest(ctx context.Context, payload []byte) ([]byte, error) {
	reqHTTP, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/messages", bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("llmkit: crear peticion HTTP Anthropic: %w", err)
	}
	c.setHeaders(reqHTTP)

	resp, err := c.httpClient.Do(reqHTTP)
	if err != nil {
		return nil, fmt.Errorf("llmkit: error en peticion HTTP Anthropic: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("llmkit: error leyendo respuesta Anthropic: %w", err)
	}

	if resp.StatusCode >= 400 {
		return nil, c.parseAPIError(resp.StatusCode, body)
	}

	return body, nil
}

func (c *AnthropicClient) parseAPIError(status int, body []byte) error {
	var errResp struct {
		Error struct {
			Type    string `json:"type"`
			Message string `json:"message"`
		} `json:"error"`
	}
	msg := ""
	if json.Unmarshal(body, &errResp) == nil && errResp.Error.Message != "" {
		msg = errResp.Error.Message
	}
	return &APIError{
		StatusCode: status,
		Message:    msg,
		RawBody:    string(body),
	}
}

func (c *AnthropicClient) buildPayload(req contracts.LLMRequest, stream bool) ([]byte, error) {
	// Anthropic maneja el system prompt como campo de primer nivel, no como turno en messages.
	systemPrompt := req.System
	var messages []map[string]any

	for _, m := range req.Messages {
		switch m.Role {
		case contracts.RoleSystem:
			if systemPrompt != "" {
				systemPrompt += "\n\n" + m.TextContent()
			} else {
				systemPrompt = m.TextContent()
			}
		case contracts.RoleUser:
			var blocks []map[string]any
			for _, b := range m.Content {
				switch b.Type {
				case contracts.ContentTypeText:
					blocks = append(blocks, map[string]any{
						"type": "text",
						"text": b.Text,
					})
				case contracts.ContentTypeToolResult:
					res := map[string]any{
						"type":         "tool_result",
						"tool_use_id": b.ToolCallID,
						"content":      b.Content,
					}
					if b.IsError {
						res["is_error"] = true
					}
					blocks = append(blocks, res)
				case contracts.ContentTypeImage:
					blocks = append(blocks, map[string]any{
						"type": "image",
						"source": map[string]any{
							"type":       "base64",
							"media_type": b.MediaType,
							"data":       b.Data,
						},
					})
				}
			}
			if len(blocks) > 0 {
				messages = append(messages, map[string]any{
					"role":    "user",
					"content": blocks,
				})
			}
		case contracts.RoleAssistant:
			var blocks []map[string]any
			for _, b := range m.Content {
				switch b.Type {
				case contracts.ContentTypeText:
					if b.Text != "" {
						blocks = append(blocks, map[string]any{
							"type": "text",
							"text": b.Text,
						})
					}
				case contracts.ContentTypeToolCall:
					var inputObj any
					switch v := b.Arguments.(type) {
					case string:
						var parsed any
						if err := json.Unmarshal([]byte(v), &parsed); err == nil {
							inputObj = parsed
						} else {
							inputObj = v
						}
					case nil:
						inputObj = map[string]any{}
					default:
						inputObj = v
					}
					blocks = append(blocks, map[string]any{
						"type":  "tool_use",
						"id":    b.ID,
						"name":  b.Name,
						"input": inputObj,
					})
				}
			}
			if len(blocks) > 0 {
				messages = append(messages, map[string]any{
					"role":    "assistant",
					"content": blocks,
				})
			}
		}
	}

	maxTokens := 4096
	if req.Options.MaxTokens != nil && *req.Options.MaxTokens > 0 {
		maxTokens = *req.Options.MaxTokens
	}

	payload := map[string]any{
		"model":      c.model,
		"messages":   messages,
		"max_tokens": maxTokens,
	}

	if systemPrompt != "" {
		payload["system"] = systemPrompt
	}

	if len(req.Tools) > 0 {
		var tools []map[string]any
		for _, t := range req.Tools {
			schema := t.Parameters
			if schema == nil {
				schema = map[string]any{"type": "object"}
			}
			tools = append(tools, map[string]any{
				"name":         t.Name,
				"description":  t.Description,
				"input_schema": schema,
			})
		}
		payload["tools"] = tools
	}

	if req.Options.Temperature != nil {
		payload["temperature"] = *req.Options.Temperature
	}
	if req.Options.ToolChoice != nil {
		payload["tool_choice"] = req.Options.ToolChoice
	}

	if stream {
		payload["stream"] = true
	}

	return json.Marshal(payload)
}

type anthropicResponse struct {
	ID      string `json:"id"`
	Type    string `json:"type"`
	Role    string `json:"role"`
	Content []struct {
		Type  string         `json:"type"`
		Text  string         `json:"text"`
		ID    string         `json:"id"`
		Name  string         `json:"name"`
		Input map[string]any `json:"input"`
	} `json:"content"`
	Model      string `json:"model"`
	StopReason string `json:"stop_reason"`
	Usage      struct {
		InputTokens              int `json:"input_tokens"`
		OutputTokens             int `json:"output_tokens"`
		CacheCreationInputTokens int `json:"cache_creation_input_tokens"`
		CacheReadInputTokens     int `json:"cache_read_input_tokens"`
	} `json:"usage"`
}

func (c *AnthropicClient) parseResponse(body []byte) (contracts.LLMResponse, error) {
	var resp anthropicResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return contracts.LLMResponse{}, fmt.Errorf("llmkit: decodificar respuesta Anthropic: %w", err)
	}

	var blocks []contracts.ContentBlock
	for _, item := range resp.Content {
		switch item.Type {
		case "text":
			blocks = append(blocks, contracts.TextBlock(item.Text))
		case "tool_use":
			blocks = append(blocks, contracts.ToolCallBlock(item.ID, item.Name, item.Input))
		}
	}

	stopReason := mapAnthropicStopReason(resp.StopReason)

	return contracts.LLMResponse{
		Message: contracts.Message{
			Role:    contracts.RoleAssistant,
			Content: blocks,
		},
		StopReason: stopReason,
		Usage: contracts.LLMUsage{
			InputTokens:      resp.Usage.InputTokens,
			OutputTokens:     resp.Usage.OutputTokens,
			CacheReadTokens:  resp.Usage.CacheReadInputTokens,
			CacheWriteTokens: resp.Usage.CacheCreationInputTokens,
		},
	}, nil
}

func mapAnthropicStopReason(sr string) contracts.StopReason {
	switch sr {
	case "end_turn", "stop_sequence":
		return contracts.StopReasonEnd
	case "tool_use":
		return contracts.StopReasonToolCalls
	case "max_tokens":
		return contracts.StopReasonMaxTokens
	default:
		return contracts.StopReasonEnd
	}
}

type anthropicSSEEvent struct {
	Type         string `json:"type"`
	Index        int    `json:"index"`
	ContentBlock struct {
		Type string `json:"type"`
		ID   string `json:"id"`
		Name string `json:"name"`
		Text string `json:"text"`
	} `json:"content_block"`
	Delta struct {
		Type        string `json:"type"`
		Text        string `json:"text"`
		PartialJSON string `json:"partial_json"`
		StopReason  string `json:"stop_reason"`
	} `json:"delta"`
	Message struct {
		ID    string `json:"id"`
		Usage struct {
			InputTokens              int `json:"input_tokens"`
			OutputTokens             int `json:"output_tokens"`
			CacheCreationInputTokens int `json:"cache_creation_input_tokens"`
			CacheReadInputTokens     int `json:"cache_read_input_tokens"`
		} `json:"usage"`
	} `json:"message"`
	Usage struct {
		OutputTokens int `json:"output_tokens"`
	} `json:"usage"`
}

type anthropicToolTracker struct {
	id   string
	name string
}

func (c *AnthropicClient) readSSEStream(ctx context.Context, body io.Reader, eventCh chan<- contracts.LLMStreamEvent, errCh chan<- error) {
	scanner := bufio.NewScanner(body)
	var finalStopReason contracts.StopReason
	var finalUsage contracts.LLMUsage
	hasFinalStopReason := false
	emittedEnd := false

	toolBlocks := make(map[int]anthropicToolTracker)

	for scanner.Scan() {
		select {
		case <-ctx.Done():
			errCh <- ctx.Err()
			return
		default:
		}

		line := scanner.Text()
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		if !strings.HasPrefix(line, "data:") {
			continue
		}

		data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if data == "[DONE]" {
			break
		}

		var ev anthropicSSEEvent
		if err := json.Unmarshal([]byte(data), &ev); err != nil {
			continue
		}

		switch ev.Type {
		case "message_start":
			finalUsage.InputTokens = ev.Message.Usage.InputTokens
			finalUsage.CacheReadTokens = ev.Message.Usage.CacheReadInputTokens
			finalUsage.CacheWriteTokens = ev.Message.Usage.CacheCreationInputTokens

		case "content_block_start":
			if ev.ContentBlock.Type == "tool_use" {
				toolBlocks[ev.Index] = anthropicToolTracker{
					id:   ev.ContentBlock.ID,
					name: ev.ContentBlock.Name,
				}
			}

		case "content_block_delta":
			switch ev.Delta.Type {
			case "text_delta":
				if ev.Delta.Text != "" {
					eventCh <- contracts.LLMStreamEvent{
						Type: contracts.StreamEventTypeTextDelta,
						Text: ev.Delta.Text,
					}
				}
			case "input_json_delta":
				tb := toolBlocks[ev.Index]
				eventCh <- contracts.LLMStreamEvent{
					Type:           contracts.StreamEventTypeToolCallDelta,
					Index:          ev.Index,
					ToolCallID:     tb.id,
					ToolName:       tb.name,
					ArgumentsDelta: ev.Delta.PartialJSON,
				}
			}

		case "message_delta":
			if ev.Delta.StopReason != "" {
				finalStopReason = mapAnthropicStopReason(ev.Delta.StopReason)
				hasFinalStopReason = true
			}
			if ev.Usage.OutputTokens > 0 {
				finalUsage.OutputTokens = ev.Usage.OutputTokens
			}

		case "message_stop":
			if !emittedEnd {
				emittedEnd = true
				if !hasFinalStopReason {
					finalStopReason = contracts.StopReasonEnd
				}
				eventCh <- contracts.LLMStreamEvent{
					Type:       contracts.StreamEventTypeEnd,
					StopReason: finalStopReason,
					Usage:      finalUsage,
				}
			}
		}
	}

	if err := scanner.Err(); err != nil && err != io.EOF {
		errCh <- fmt.Errorf("llmkit: error en stream SSE Anthropic: %w", err)
		return
	}

	if !emittedEnd && (hasFinalStopReason || finalUsage.InputTokens > 0 || finalUsage.OutputTokens > 0) {
		if !hasFinalStopReason {
			finalStopReason = contracts.StopReasonEnd
		}
		eventCh <- contracts.LLMStreamEvent{
			Type:       contracts.StreamEventTypeEnd,
			StopReason: finalStopReason,
			Usage:      finalUsage,
		}
	}
}
