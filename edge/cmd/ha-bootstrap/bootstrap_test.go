package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"domotica/edge/internal/ha/hatest"
)

// fakeOnboarding simula los endpoints HTTP de onboarding/auth de HA y monta el
// WS falso en /api/websocket.
type fakeOnboarding struct {
	userDone      bool
	tokenStatus   int   // 0 = 200
	configSeq     []any // respuestas sucesivas de get_config (nil = RUNNING)
	gotUser       map[string]any
	gotTokenForm  map[string]string
	bearerOnSteps []string
}

func (f *fakeOnboarding) server(t *testing.T) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/onboarding", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, []map[string]any{
			{"step": "user", "done": f.userDone},
			{"step": "core_config", "done": false},
			{"step": "analytics", "done": false},
			{"step": "integration", "done": false},
		})
	})
	mux.HandleFunc("POST /api/onboarding/users", func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&f.gotUser); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		writeJSON(w, map[string]any{"auth_code": "CODE-1"})
	})
	mux.HandleFunc("POST /auth/token", func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		f.gotTokenForm = map[string]string{
			"grant_type": r.PostForm.Get("grant_type"),
			"code":       r.PostForm.Get("code"),
			"client_id":  r.PostForm.Get("client_id"),
		}
		if f.tokenStatus != 0 {
			http.Error(w, `{"error":"invalid_request"}`, f.tokenStatus)
			return
		}
		writeJSON(w, map[string]any{"access_token": "ACCESS-1", "token_type": "Bearer", "refresh_token": "R-1", "expires_in": 1800})
	})
	for _, step := range []string{"core_config", "analytics", "integration"} {
		mux.HandleFunc("POST /api/onboarding/"+step, func(w http.ResponseWriter, r *http.Request) {
			f.bearerOnSteps = append(f.bearerOnSteps, r.Header.Get("Authorization"))
			writeJSON(w, map[string]any{})
		})
	}
	opts := hatest.Options{
		Token:   "ACCESS-1",
		Results: map[string]any{"auth/long_lived_access_token": "LLAT-123"},
	}
	if f.configSeq != nil {
		opts.ResultSeq = map[string][]any{"get_config": f.configSeq}
	}
	mux.Handle("/api/websocket", hatest.NewHandler(t, opts))
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

func TestBootstrap(t *testing.T) {
	starting := hatest.Config("STARTING", "2026.10.1")
	running := hatest.Config("RUNNING", "2026.10.1")

	tests := []struct {
		name      string
		fake      fakeOnboarding
		timeout   time.Duration
		wantToken string
		wantErr   string
	}{
		{name: "flujo completo", wantToken: "LLAT-123"},
		// Regresión CI M0: la API de onboarding responde antes de que HA
		// termine de cargar integraciones.
		{name: "espera a que HA termine de arrancar", fake: fakeOnboarding{configSeq: []any{starting, starting, running}}, wantToken: "LLAT-123"},
		{name: "HA nunca termina de arrancar", fake: fakeOnboarding{configSeq: []any{starting}}, timeout: 300 * time.Millisecond, wantErr: "arrancar"},
		{name: "ya onboardeado", fake: fakeOnboarding{userDone: true}, wantErr: "ya fue configurado"},
		{name: "token endpoint falla", fake: fakeOnboarding{tokenStatus: http.StatusBadRequest}, wantErr: "auth/token"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fake := tt.fake
			srv := fake.server(t)
			timeout := tt.timeout
			if timeout == 0 {
				timeout = 5 * time.Second
			}
			ctx, cancel := context.WithTimeout(context.Background(), timeout)
			defer cancel()

			got, err := bootstrap(ctx, srv.URL, "dev", "dev-pass", srv.Client(), 10*time.Millisecond)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("bootstrap() error = %v, debe contener %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("bootstrap() error: %v", err)
			}
			if got != tt.wantToken {
				t.Fatalf("token = %q, want %q", got, tt.wantToken)
			}
			if fake.gotUser["username"] != "dev" || fake.gotUser["password"] != "dev-pass" || fake.gotUser["client_id"] != srv.URL+"/" {
				t.Fatalf("payload de usuario = %v", fake.gotUser)
			}
			wantForm := map[string]string{"grant_type": "authorization_code", "code": "CODE-1", "client_id": srv.URL + "/"}
			for k, v := range wantForm {
				if fake.gotTokenForm[k] != v {
					t.Fatalf("form /auth/token[%s] = %q, want %q", k, fake.gotTokenForm[k], v)
				}
			}
			for _, b := range fake.bearerOnSteps {
				if b != "Bearer ACCESS-1" {
					t.Fatalf("paso de onboarding sin bearer correcto: %q", b)
				}
			}
		})
	}
}

func TestWaitReady(t *testing.T) {
	tests := []struct {
		name      string
		failFirst int32 // cantidad de 503 antes de responder 200; -1 = siempre 503
		timeout   time.Duration
		wantErr   bool
	}{
		{name: "listo tras reintentos", failFirst: 2, timeout: 2 * time.Second},
		{name: "nunca listo", failFirst: -1, timeout: 150 * time.Millisecond, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var calls atomic.Int32
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				n := calls.Add(1)
				if tt.failFirst < 0 || n <= tt.failFirst {
					w.WriteHeader(http.StatusServiceUnavailable)
					return
				}
				writeJSON(w, []any{})
			}))
			t.Cleanup(srv.Close)
			ctx, cancel := context.WithTimeout(context.Background(), tt.timeout)
			defer cancel()

			err := waitReady(ctx, srv.URL, srv.Client(), 10*time.Millisecond)
			if tt.wantErr {
				if err == nil {
					t.Fatal("waitReady() error = nil, se esperaba timeout")
				}
				return
			}
			if err != nil {
				t.Fatalf("waitReady() error: %v", err)
			}
			if calls.Load() != tt.failFirst+1 {
				t.Fatalf("llamadas = %d, want %d", calls.Load(), tt.failFirst+1)
			}
		})
	}
}

func TestWriteEnvFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sub", ".env")
	if err := writeEnvFile(path, "http://localhost:8123", "LLAT-123"); err != nil {
		t.Fatalf("writeEnvFile() error: %v", err)
	}
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"DOMOTICA_MODE=dev\n",
		"DOMOTICA_HA_URL=ws://localhost:8123/api/websocket\n",
		"DOMOTICA_HA_TOKEN=LLAT-123\n",
	} {
		if !strings.Contains(string(b), want) {
			t.Errorf(".env no contiene %q:\n%s", want, b)
		}
	}
}

func TestWSURL(t *testing.T) {
	tests := []struct{ in, want string }{
		{"http://localhost:8123", "ws://localhost:8123/api/websocket"},
		{"https://ha.example.com/", "wss://ha.example.com/api/websocket"},
	}
	for _, tt := range tests {
		if got := wsURL(tt.in); got != tt.want {
			t.Errorf("wsURL(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}
