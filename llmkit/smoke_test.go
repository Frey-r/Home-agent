//go:build integration_llm

package llmkit_test

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/Frey-r/Home-agent/contracts"
	"github.com/Frey-r/Home-agent/llmkit"
)

// TestRealLLM_NemotronCloud ejecuta un test real contra NVIDIA NIM (Nemotron cloud)
// Requiere la variable de entorno NVIDIA_API_KEY o NEMOTRON_API_KEY.
func TestRealLLM_NemotronCloud(t *testing.T) {
	apiKey := os.Getenv("NVIDIA_API_KEY")
	if apiKey == "" {
		apiKey = os.Getenv("NEMOTRON_API_KEY")
	}
	if apiKey == "" {
		t.Skip("Omitiendo smoke test cloud: NVIDIA_API_KEY no configurada")
	}

	model := os.Getenv("NEMOTRON_MODEL")
	if model == "" {
		model = "nvidia/llama-3.1-nemotron-70b-instruct"
	}

	client, err := llmkit.NewOpenAIClient(llmkit.OpenAIConfig{
		BaseURL: "https://integrate.api.nvidia.com/v1",
		APIKey:  apiKey,
		Model:   model,
	})
	if err != nil {
		t.Fatalf("NewOpenAIClient error: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	t.Run("Complete", func(t *testing.T) {
		resp, err := client.Complete(ctx, contracts.LLMRequest{
			Messages: []contracts.Message{
				contracts.UserMessage("Di únicamente la palabra: CONFIRMADO"),
			},
		})
		if err != nil {
			t.Fatalf("Complete error: %v", err)
		}

		t.Logf("Nemotron cloud response: %s (usage: %+v)", resp.Text(), resp.Usage)
		if !strings.Contains(resp.Text(), "CONFIRMADO") {
			t.Logf("Aviso: el modelo respondió %q, esperado contener 'CONFIRMADO'", resp.Text())
		}
		if resp.Usage.InputTokens == 0 || resp.Usage.OutputTokens == 0 {
			t.Errorf("Usage tokens no reportados: %+v", resp.Usage)
		}
	})

	t.Run("Stream", func(t *testing.T) {
		eventCh, errCh := client.Stream(ctx, contracts.LLMRequest{
			Messages: []contracts.Message{
				contracts.UserMessage("Cuenta del 1 al 3."),
			},
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
					t.Fatalf("Stream error: %v", err)
				}
			}
		}

		t.Logf("Nemotron stream accumulado: %s", textAcc)
		if len(textAcc) == 0 {
			t.Errorf("Stream no emitió texto")
		}
	})
}

// TestRealLLM_OllamaLocal ejecuta un test real contra una instancia local de Ollama
// Requiere la variable de entorno OLLAMA_BASE_URL (ej: http://localhost:11434/v1).
func TestRealLLM_OllamaLocal(t *testing.T) {
	baseURL := os.Getenv("OLLAMA_BASE_URL")
	if baseURL == "" {
		t.Skip("Omitiendo smoke test local: OLLAMA_BASE_URL no configurada")
	}

	model := os.Getenv("OLLAMA_MODEL")
	if model == "" {
		model = "llama3.2"
	}

	client, err := llmkit.NewOpenAIClient(llmkit.OpenAIConfig{
		BaseURL: baseURL,
		Model:   model,
	})
	if err != nil {
		t.Fatalf("NewOpenAIClient error: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	resp, err := client.Complete(ctx, contracts.LLMRequest{
		Messages: []contracts.Message{
			contracts.UserMessage("Responde 'HOLA'"),
		},
	})
	if err != nil {
		t.Fatalf("Complete Ollama error: %v", err)
	}
	t.Logf("Ollama response: %s", resp.Text())
}
