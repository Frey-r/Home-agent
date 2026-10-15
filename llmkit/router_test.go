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

func TestRouter_PrimarySuccess(t *testing.T) {
	srv1 := llmtest.New(t)
	srv2 := llmtest.New(t)

	srv1.RespondJSON(http.StatusOK, map[string]any{
		"choices": []map[string]any{
			{"message": map[string]any{"role": "assistant", "content": "Respuesta de primario"}},
		},
	})

	c1, _ := llmkit.NewOpenAIClient(llmkit.OpenAIConfig{BaseURL: srv1.URL + "/v1"})
	c2, _ := llmkit.NewOpenAIClient(llmkit.OpenAIConfig{BaseURL: srv2.URL + "/v1"})

	router, err := llmkit.NewRouter(llmkit.RouterConfig{
		Targets: []llmkit.RouteTarget{
			{Name: "primary", Client: c1},
			{Name: "secondary", Client: c2},
		},
	})
	if err != nil {
		t.Fatalf("NewRouter error: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	resp, err := router.Complete(ctx, contracts.LLMRequest{
		Messages: []contracts.Message{contracts.UserMessage("hola")},
	})
	if err != nil {
		t.Fatalf("Complete error: %v", err)
	}

	if resp.Text() != "Respuesta de primario" {
		t.Errorf("Text mismatch: got %q", resp.Text())
	}

	if len(srv2.Requests()) != 0 {
		t.Errorf("Secondary should not have been called, got %d requests", len(srv2.Requests()))
	}
}

func TestRouter_FallbackOn429(t *testing.T) {
	srv1 := llmtest.New(t)
	srv2 := llmtest.New(t)

	srv1.RespondJSON(http.StatusTooManyRequests, map[string]any{
		"error": map[string]any{"message": "Rate limit exceeded"},
	})
	srv2.RespondJSON(http.StatusOK, map[string]any{
		"choices": []map[string]any{
			{"message": map[string]any{"role": "assistant", "content": "Respuesta de fallback tras 429"}},
		},
	})

	c1, _ := llmkit.NewOpenAIClient(llmkit.OpenAIConfig{BaseURL: srv1.URL + "/v1"})
	c2, _ := llmkit.NewOpenAIClient(llmkit.OpenAIConfig{BaseURL: srv2.URL + "/v1"})

	router, _ := llmkit.NewRouter(llmkit.RouterConfig{
		Targets: []llmkit.RouteTarget{
			{Name: "nemotron-primary", Client: c1},
			{Name: "claude-fallback", Client: c2},
		},
	})

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	resp, err := router.Complete(ctx, contracts.LLMRequest{
		Messages: []contracts.Message{contracts.UserMessage("hola")},
	})
	if err != nil {
		t.Fatalf("Complete error: %v", err)
	}

	if resp.Text() != "Respuesta de fallback tras 429" {
		t.Errorf("Text mismatch: got %q", resp.Text())
	}
	if len(srv1.Requests()) != 1 {
		t.Errorf("Primary should have been called once, got %d", len(srv1.Requests()))
	}
	if len(srv2.Requests()) != 1 {
		t.Errorf("Secondary should have been called once, got %d", len(srv2.Requests()))
	}
}

func TestRouter_FallbackOn5xx(t *testing.T) {
	srv1 := llmtest.New(t)
	srv2 := llmtest.New(t)

	srv1.RespondJSON(http.StatusInternalServerError, map[string]any{
		"error": map[string]any{"message": "Internal error"},
	})
	srv2.RespondJSON(http.StatusOK, map[string]any{
		"choices": []map[string]any{
			{"message": map[string]any{"role": "assistant", "content": "Respuesta tras 500"}},
		},
	})

	c1, _ := llmkit.NewOpenAIClient(llmkit.OpenAIConfig{BaseURL: srv1.URL + "/v1"})
	c2, _ := llmkit.NewOpenAIClient(llmkit.OpenAIConfig{BaseURL: srv2.URL + "/v1"})

	router, _ := llmkit.NewRouter(llmkit.RouterConfig{
		Targets: []llmkit.RouteTarget{
			{Name: "primary", Client: c1},
			{Name: "fallback", Client: c2},
		},
	})

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	resp, err := router.Complete(ctx, contracts.LLMRequest{
		Messages: []contracts.Message{contracts.UserMessage("hola")},
	})
	if err != nil {
		t.Fatalf("Complete error: %v", err)
	}
	if resp.Text() != "Respuesta tras 500" {
		t.Errorf("Text mismatch: got %q", resp.Text())
	}
}

func TestRouter_FallbackOnTimeout(t *testing.T) {
	srv1 := llmtest.New(t)
	srv2 := llmtest.New(t)

	srv1.SetHandler(func(r *http.Request, body []byte) (int, http.Header, []byte) {
		time.Sleep(150 * time.Millisecond)
		return http.StatusOK, nil, []byte(`{"choices":[{"message":{"role":"assistant","content":"demasiado tarde"}}]}`)
	})
	srv2.RespondJSON(http.StatusOK, map[string]any{
		"choices": []map[string]any{
			{"message": map[string]any{"role": "assistant", "content": "Respuesta rápida de fallback"}},
		},
	})

	c1, _ := llmkit.NewOpenAIClient(llmkit.OpenAIConfig{BaseURL: srv1.URL + "/v1"})
	c2, _ := llmkit.NewOpenAIClient(llmkit.OpenAIConfig{BaseURL: srv2.URL + "/v1"})

	router, _ := llmkit.NewRouter(llmkit.RouterConfig{
		Targets: []llmkit.RouteTarget{
			{Name: "slow-primary", Client: c1, Timeout: 40 * time.Millisecond},
			{Name: "fast-fallback", Client: c2},
		},
	})

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	resp, err := router.Complete(ctx, contracts.LLMRequest{
		Messages: []contracts.Message{contracts.UserMessage("hola")},
	})
	if err != nil {
		t.Fatalf("Complete error: %v", err)
	}
	if resp.Text() != "Respuesta rápida de fallback" {
		t.Errorf("Text mismatch: got %q", resp.Text())
	}
}

func TestRouter_NoFallbackOn400(t *testing.T) {
	srv1 := llmtest.New(t)
	srv2 := llmtest.New(t)

	srv1.RespondJSON(http.StatusBadRequest, map[string]any{
		"error": map[string]any{"message": "Invalid parameter: max_tokens must be positive"},
	})

	c1, _ := llmkit.NewOpenAIClient(llmkit.OpenAIConfig{BaseURL: srv1.URL + "/v1"})
	c2, _ := llmkit.NewOpenAIClient(llmkit.OpenAIConfig{BaseURL: srv2.URL + "/v1"})

	router, _ := llmkit.NewRouter(llmkit.RouterConfig{
		Targets: []llmkit.RouteTarget{
			{Name: "primary", Client: c1},
			{Name: "fallback", Client: c2},
		},
	})

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	_, err := router.Complete(ctx, contracts.LLMRequest{
		Messages: []contracts.Message{contracts.UserMessage("hola")},
	})
	if err == nil {
		t.Fatal("expected error on 400, got nil")
	}

	var apiErr *llmkit.APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("expected *llmkit.APIError, got %T: %v", err, err)
	}
	if apiErr.StatusCode != 400 {
		t.Errorf("expected status 400, got %d", apiErr.StatusCode)
	}

	if len(srv2.Requests()) != 0 {
		t.Errorf("Secondary should NOT have been called on 400 client error, got %d calls", len(srv2.Requests()))
	}
}

func TestRouter_AllFailReturnsCompositeError(t *testing.T) {
	srv1 := llmtest.New(t)
	srv2 := llmtest.New(t)

	srv1.RespondJSON(http.StatusTooManyRequests, map[string]any{
		"error": map[string]any{"message": "Primary rate limited"},
	})
	srv2.RespondJSON(http.StatusBadGateway, map[string]any{
		"error": map[string]any{"message": "Secondary 502"},
	})

	c1, _ := llmkit.NewOpenAIClient(llmkit.OpenAIConfig{BaseURL: srv1.URL + "/v1"})
	c2, _ := llmkit.NewOpenAIClient(llmkit.OpenAIConfig{BaseURL: srv2.URL + "/v1"})

	router, _ := llmkit.NewRouter(llmkit.RouterConfig{
		Targets: []llmkit.RouteTarget{
			{Name: "primary", Client: c1},
			{Name: "fallback", Client: c2},
		},
	})

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	_, err := router.Complete(ctx, contracts.LLMRequest{
		Messages: []contracts.Message{contracts.UserMessage("hola")},
	})
	if err == nil {
		t.Fatal("expected composite error, got nil")
	}

	var fbErr *llmkit.FallbackError
	if !errors.As(err, &fbErr) {
		t.Fatalf("expected *llmkit.FallbackError, got %T: %v", err, err)
	}
	if len(fbErr.Errors) != 2 {
		t.Errorf("expected 2 target errors, got %d", len(fbErr.Errors))
	}
}

func TestRouter_Streaming_Fallback(t *testing.T) {
	srv1 := llmtest.New(t)
	srv2 := llmtest.New(t)

	// srv1 falla con 429
	srv1.RespondJSON(http.StatusTooManyRequests, map[string]any{
		"error": map[string]any{"message": "stream rate limit"},
	})
	// srv2 responde con SSE
	srv2.RespondSSE([]string{
		"data: {\"choices\":[{\"delta\":{\"content\":\"Stream de fallback\"}}]}\n\n",
		"data: {\"choices\":[{\"finish_reason\":\"stop\"}]}\n\n",
		"data: [DONE]\n\n",
	})

	c1, _ := llmkit.NewOpenAIClient(llmkit.OpenAIConfig{BaseURL: srv1.URL + "/v1"})
	c2, _ := llmkit.NewOpenAIClient(llmkit.OpenAIConfig{BaseURL: srv2.URL + "/v1"})

	router, _ := llmkit.NewRouter(llmkit.RouterConfig{
		Targets: []llmkit.RouteTarget{
			{Name: "primary", Client: c1},
			{Name: "fallback", Client: c2},
		},
	})

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	eventCh, errCh := router.Stream(ctx, contracts.LLMRequest{
		Messages: []contracts.Message{contracts.UserMessage("hola")},
	})

	var textAcc string
	for eventCh != nil || errCh != nil {
		select {
		case ev, ok := <-eventCh:
			if !ok {
				eventCh = nil
				continue
			}
			if ev.Type == contracts.StreamEventTypeTextDelta {
				textAcc += ev.Text
			}
		case err, ok := <-errCh:
			if !ok {
				errCh = nil
				continue
			}
			if err != nil {
				t.Fatalf("unexpected stream error: %v", err)
			}
		}
	}

	if textAcc != "Stream de fallback" {
		t.Errorf("textAcc: got %q, want 'Stream de fallback'", textAcc)
	}
}
