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

// OpenAIConfig contiene la configuración para instanciar el cliente OpenAI.
type OpenAIConfig struct {
	BaseURL    string // Por defecto: https://api.openai.com/v1
	APIKey     string
	Model      string
	HTTPClient *http.Client
}

// OpenAIClient implementa la interfaz Client para la REST API de OpenAI Chat Completions.
// Es compatible con OpenAI, Azure OpenAI, NVIDIA NIM (Nemotron), Ollama local, Groq, DeepSeek, etc.
type OpenAIClient struct {
	baseURL    string
	apiKey     string
	model      string
	httpClient *http.Client
}

// NewOpenAIClient crea un nuevo adaptador OpenAI para formato neutro.
func NewOpenAIClient(cfg OpenAIConfig) (*OpenAIClient, error) {
	baseURL := strings.TrimRight(cfg.BaseURL, "/")
	if baseURL == "" {
		baseURL = "https://api.openai.com/v1"
	}
	if cfg.Model == "" {
		cfg.Model = "gpt-4o"
	}
	httpClient := cfg.HTTPClient
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 60 * time.Second}
	}

	return &OpenAIClient{
		baseURL:    baseURL,
		apiKey:     cfg.APIKey,
		model:      cfg.Model,
		httpClient: httpClient,
	}, nil
}

// Complete ejecuta una llamada no-streaming a Chat Completions y la traduce a contracts.LLMResponse.
func (c *OpenAIClient) Complete(ctx context.Context, req contracts.LLMRequest) (contracts.LLMResponse, error) {
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
func (c *OpenAIClient) Stream(ctx context.Context, req contracts.LLMRequest) (<-chan contracts.LLMStreamEvent, <-chan error) {
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

		reqHTTP, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/chat/completions", bytes.NewReader(payload))
		if err != nil {
			errCh <- fmt.Errorf("llmkit: crear peticion streaming: %w", err)
			return
		}
		c.setHeaders(reqHTTP)

		resp, err := c.httpClient.Do(reqHTTP)
		if err != nil {
			errCh <- fmt.Errorf("llmkit: error en peticion HTTP: %w", err)
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

func (c *OpenAIClient) setHeaders(req *http.Request) {
	req.Header.Set("Content-Type", "application/json")
	if c.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.apiKey)
	}
}

func (c *OpenAIClient) doRequest(ctx context.Context, payload []byte) ([]byte, error) {
	reqHTTP, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/chat/completions", bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("llmkit: crear peticion HTTP: %w", err)
	}
	c.setHeaders(reqHTTP)

	resp, err := c.httpClient.Do(reqHTTP)
	if err != nil {
		return nil, fmt.Errorf("llmkit: error en peticion HTTP: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("llmkit: error leyendo respuesta: %w", err)
	}

	if resp.StatusCode >= 400 {
		return nil, c.parseAPIError(resp.StatusCode, body)
	}

	return body, nil
}

func (c *OpenAIClient) parseAPIError(status int, body []byte) error {
	var errResp struct {
		Error struct {
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

func (c *OpenAIClient) buildPayload(req contracts.LLMRequest, stream bool) ([]byte, error) {
	var messages []map[string]any

	if req.System != "" {
		messages = append(messages, map[string]any{
			"role":    "system",
			"content": req.System,
		})
	}

	for _, m := range req.Messages {
		switch m.Role {
		case contracts.RoleSystem:
			messages = append(messages, map[string]any{
				"role":    "system",
				"content": m.TextContent(),
			})
		case contracts.RoleUser:
			for _, b := range m.Content {
				switch b.Type {
				case contracts.ContentTypeToolResult:
					messages = append(messages, map[string]any{
						"role":         "tool",
						"tool_call_id": b.ToolCallID,
						"content":      b.Content,
					})
				case contracts.ContentTypeText:
					messages = append(messages, map[string]any{
						"role":    "user",
						"content": b.Text,
					})
				}
			}
		case contracts.RoleAssistant:
			msg := map[string]any{"role": "assistant"}
			text := m.TextContent()
			if text != "" {
				msg["content"] = text
			}
			toolCalls := m.ToolCalls()
			if len(toolCalls) > 0 {
				var calls []map[string]any
				for _, tc := range toolCalls {
					argsStr, _ := tc.ArgumentsJSON()
					calls = append(calls, map[string]any{
						"id":   tc.ID,
						"type": "function",
						"function": map[string]any{
							"name":      tc.Name,
							"arguments": argsStr,
						},
					})
				}
				msg["tool_calls"] = calls
			}
			messages = append(messages, msg)
		}
	}

	payload := map[string]any{
		"model":    c.model,
		"messages": messages,
	}

	if len(req.Tools) > 0 {
		var tools []map[string]any
		for _, t := range req.Tools {
			tools = append(tools, map[string]any{
				"type": "function",
				"function": map[string]any{
					"name":        t.Name,
					"description": t.Description,
					"parameters":  t.Parameters,
				},
			})
		}
		payload["tools"] = tools
	}

	if req.Options.Temperature != nil {
		payload["temperature"] = *req.Options.Temperature
	}
	if req.Options.MaxTokens != nil {
		payload["max_tokens"] = *req.Options.MaxTokens
	}
	if req.Options.ToolChoice != nil {
		payload["tool_choice"] = req.Options.ToolChoice
	}

	if stream {
		payload["stream"] = true
		payload["stream_options"] = map[string]any{
			"include_usage": true,
		}
	}

	return json.Marshal(payload)
}

type openAIChoice struct {
	Index   int `json:"index"`
	Message struct {
		Role      string `json:"role"`
		Content   string `json:"content"`
		ToolCalls []struct {
			ID       string `json:"id"`
			Type     string `json:"type"`
			Function struct {
				Name      string `json:"name"`
				Arguments string `json:"arguments"`
			} `json:"function"`
		} `json:"tool_calls"`
	} `json:"message"`
	FinishReason string `json:"finish_reason"`
}

type openAIUsage struct {
	PromptTokens        int `json:"prompt_tokens"`
	CompletionTokens    int `json:"completion_tokens"`
	PromptTokensDetails struct {
		CachedTokens int `json:"cached_tokens"`
	} `json:"prompt_tokens_details"`
}

type openAIResponse struct {
	Choices []openAIChoice `json:"choices"`
	Usage   openAIUsage    `json:"usage"`
}

func (c *OpenAIClient) parseResponse(body []byte) (contracts.LLMResponse, error) {
	var resp openAIResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return contracts.LLMResponse{}, fmt.Errorf("llmkit: decodificar respuesta OpenAI: %w", err)
	}

	if len(resp.Choices) == 0 {
		return contracts.LLMResponse{}, fmt.Errorf("llmkit: respuesta sin opciones (choices)")
	}

	choice := resp.Choices[0]
	var blocks []contracts.ContentBlock

	if choice.Message.Content != "" {
		blocks = append(blocks, contracts.TextBlock(choice.Message.Content))
	}

	for _, tc := range choice.Message.ToolCalls {
		blocks = append(blocks, contracts.ToolCallBlock(tc.ID, tc.Function.Name, tc.Function.Arguments))
	}

	stopReason := mapOpenAIFinishReason(choice.FinishReason)

	return contracts.LLMResponse{
		Message: contracts.Message{
			Role:    contracts.RoleAssistant,
			Content: blocks,
		},
		StopReason: stopReason,
		Usage: contracts.LLMUsage{
			InputTokens:     resp.Usage.PromptTokens,
			OutputTokens:    resp.Usage.CompletionTokens,
			CacheReadTokens: resp.Usage.PromptTokensDetails.CachedTokens,
		},
	}, nil
}

func mapOpenAIFinishReason(fr string) contracts.StopReason {
	switch fr {
	case "stop":
		return contracts.StopReasonEnd
	case "tool_calls":
		return contracts.StopReasonToolCalls
	case "length":
		return contracts.StopReasonMaxTokens
	case "content_filter":
		return contracts.StopReasonRefusal
	default:
		return contracts.StopReasonEnd
	}
}

type openAIStreamChunk struct {
	Choices []struct {
		Index int `json:"index"`
		Delta struct {
			Role      string `json:"role"`
			Content   string `json:"content"`
			ToolCalls []struct {
				Index    int    `json:"index"`
				ID       string `json:"id"`
				Type     string `json:"type"`
				Function struct {
					Name      string `json:"name"`
					Arguments string `json:"arguments"`
				} `json:"function"`
			} `json:"tool_calls"`
		} `json:"delta"`
		FinishReason string `json:"finish_reason"`
	} `json:"choices"`
	Usage *openAIUsage `json:"usage"`
}

func (c *OpenAIClient) readSSEStream(ctx context.Context, body io.Reader, eventCh chan<- contracts.LLMStreamEvent, errCh chan<- error) {
	scanner := bufio.NewScanner(body)
	var finalStopReason contracts.StopReason
	var finalUsage contracts.LLMUsage
	hasFinalStopReason := false

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

		var chunk openAIStreamChunk
		if err := json.Unmarshal([]byte(data), &chunk); err != nil {
			continue
		}

		if chunk.Usage != nil {
			finalUsage.InputTokens = chunk.Usage.PromptTokens
			finalUsage.OutputTokens = chunk.Usage.CompletionTokens
			finalUsage.CacheReadTokens = chunk.Usage.PromptTokensDetails.CachedTokens
		}

		for _, choice := range chunk.Choices {
			if choice.FinishReason != "" {
				finalStopReason = mapOpenAIFinishReason(choice.FinishReason)
				hasFinalStopReason = true
			}

			if choice.Delta.Content != "" {
				eventCh <- contracts.LLMStreamEvent{
					Type: contracts.StreamEventTypeTextDelta,
					Text: choice.Delta.Content,
				}
			}

			for _, tc := range choice.Delta.ToolCalls {
				eventCh <- contracts.LLMStreamEvent{
					Type:           contracts.StreamEventTypeToolCallDelta,
					Index:          tc.Index,
					ToolCallID:     tc.ID,
					ToolName:       tc.Function.Name,
					ArgumentsDelta: tc.Function.Arguments,
				}
			}
		}
	}

	if err := scanner.Err(); err != nil && err != io.EOF {
		errCh <- fmt.Errorf("llmkit: error en stream SSE: %w", err)
		return
	}

	if hasFinalStopReason || finalUsage.InputTokens > 0 || finalUsage.OutputTokens > 0 {
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
