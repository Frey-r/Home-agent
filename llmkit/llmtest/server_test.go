package llmtest_test

import (
	"bufio"
	"bytes"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/Frey-r/Home-agent/llmkit/llmtest"
)

func TestServer_JSONResponseAndRecording(t *testing.T) {
	srv := llmtest.New(t)

	srv.RespondJSON(http.StatusOK, map[string]string{
		"status": "success",
	})

	reqBody := `{"prompt": "hello"}`
	resp, err := http.Post(srv.URL+"/v1/chat/completions", "application/json", bytes.NewBufferString(reqBody))
	if err != nil {
		t.Fatalf("http.Post error: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("StatusCode: got %d, want 200", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("ReadAll error: %v", err)
	}

	if !strings.Contains(string(body), "success") {
		t.Errorf("body does not contain 'success': %s", string(body))
	}

	reqs := srv.Requests()
	if len(reqs) != 1 {
		t.Fatalf("Requests len: got %d, want 1", len(reqs))
	}
	if reqs[0].Path != "/v1/chat/completions" {
		t.Errorf("Path: got %s, want /v1/chat/completions", reqs[0].Path)
	}
	if string(reqs[0].Body) != reqBody {
		t.Errorf("Body: got %s, want %s", string(reqs[0].Body), reqBody)
	}

	last, ok := srv.LastRequest()
	if !ok || string(last.Body) != reqBody {
		t.Errorf("LastRequest mismatch: %+v", last)
	}
}

func TestServer_SSEResponse(t *testing.T) {
	srv := llmtest.New(t)

	events := []string{
		"data: {\"chunk\": 1}\n\n",
		"data: {\"chunk\": 2}\n\n",
		"data: [DONE]\n\n",
	}
	srv.RespondSSE(events)

	resp, err := http.Get(srv.URL + "/stream")
	if err != nil {
		t.Fatalf("http.Get error: %v", err)
	}
	defer resp.Body.Close()

	if resp.Header.Get("Content-Type") != "text/event-stream" {
		t.Errorf("Content-Type: got %s, want text/event-stream", resp.Header.Get("Content-Type"))
	}

	scanner := bufio.NewScanner(resp.Body)
	var lines []string
	for scanner.Scan() {
		line := scanner.Text()
		if line != "" {
			lines = append(lines, line)
		}
	}

	if len(lines) != 3 {
		t.Fatalf("SSE lines count: got %d, want 3 (%v)", len(lines), lines)
	}
	if lines[0] != "data: {\"chunk\": 1}" {
		t.Errorf("lines[0] = %q", lines[0])
	}
}
