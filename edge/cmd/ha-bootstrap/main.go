// Comando ha-bootstrap: herramienta de desarrollo que completa el onboarding
// de un Home Assistant recién creado (dev/docker-compose.yml) y genera un
// token de larga duración en dev/.env para edge-core.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"domotica/edge/internal/ha"
)

func main() {
	base := flag.String("url", "http://localhost:8123", "URL base de Home Assistant")
	user := flag.String("user", "dev", "usuario owner a crear")
	pass := flag.String("password", "dev-password", "contraseña del usuario owner")
	envPath := flag.String("env", filepath.Join("dev", ".env"), "archivo .env a escribir")
	wait := flag.Duration("wait", 3*time.Minute, "tiempo máximo de espera a que HA arranque")
	flag.Parse()

	ctx, cancel := context.WithTimeout(context.Background(), *wait+time.Minute)
	defer cancel()

	if err := runBootstrap(ctx, *base, *user, *pass, *envPath, *wait); err != nil {
		fmt.Fprintln(os.Stderr, "ha-bootstrap:", err)
		os.Exit(1)
	}
}

func runBootstrap(ctx context.Context, base, user, pass, envPath string, wait time.Duration) error {
	client := &http.Client{Timeout: 30 * time.Second}

	waitCtx, cancel := context.WithTimeout(ctx, wait)
	defer cancel()
	slog.Info("esperando a Home Assistant", "url", base)
	if err := waitReady(waitCtx, base, client, 2*time.Second); err != nil {
		return err
	}

	token, err := bootstrap(ctx, base, user, pass, client)
	if err != nil {
		return err
	}
	if err := writeEnvFile(envPath, base, token); err != nil {
		return err
	}
	slog.Info("listo: token de larga duración guardado", "env", envPath, "usuario", user)
	return nil
}

// waitReady espera a que GET /api/onboarding responda 200.
func waitReady(ctx context.Context, baseURL string, client *http.Client, interval time.Duration) error {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, baseURL+"/api/onboarding", nil)
		if err != nil {
			return err
		}
		resp, err := client.Do(req)
		if err == nil {
			_, _ = io.Copy(io.Discard, resp.Body) // drenar para reutilizar la conexión
			resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return nil
			}
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("Home Assistant no respondió en %s: %w", baseURL, ctx.Err())
		case <-ticker.C:
		}
	}
}

// bootstrap crea el usuario owner, obtiene un access token y con él crea un
// token de larga duración vía WebSocket.
func bootstrap(ctx context.Context, baseURL, username, password string, client *http.Client) (string, error) {
	baseURL = strings.TrimRight(baseURL, "/")
	clientID := baseURL + "/"

	var steps []struct {
		Step string `json:"step"`
		Done bool   `json:"done"`
	}
	if err := doJSON(ctx, client, http.MethodGet, baseURL+"/api/onboarding", "", nil, &steps); err != nil {
		return "", err
	}
	for _, s := range steps {
		if s.Step == "user" && s.Done {
			return "", errors.New("Home Assistant ya fue configurado (onboarding hecho): borra el estado de dev/ha-config o usa un token existente")
		}
	}

	var created struct {
		AuthCode string `json:"auth_code"`
	}
	if err := doJSON(ctx, client, http.MethodPost, baseURL+"/api/onboarding/users", "", map[string]any{
		"client_id": clientID,
		"name":      "Dev",
		"username":  username,
		"password":  password,
		"language":  "es",
	}, &created); err != nil {
		return "", err
	}

	access, err := exchangeCode(ctx, client, baseURL, clientID, created.AuthCode)
	if err != nil {
		return "", err
	}

	// Pasos restantes del onboarding: solo para que la UI quede usable.
	// No son necesarios para la API, por eso un fallo no aborta.
	for step, body := range map[string]any{
		"core_config": map[string]any{},
		"analytics":   map[string]any{},
		"integration": map[string]any{"client_id": clientID, "redirect_uri": clientID + "?auth_callback=1"},
	} {
		if err := doJSON(ctx, client, http.MethodPost, baseURL+"/api/onboarding/"+step, access, body, nil); err != nil {
			slog.Warn("paso de onboarding no completado", "step", step, "err", err)
		}
	}

	conn, err := ha.Connect(ctx, wsURL(baseURL), access)
	if err != nil {
		return "", err
	}
	defer conn.Close()
	raw, err := conn.Command(ctx, map[string]any{
		"type":        "auth/long_lived_access_token",
		"client_name": "edge-core-dev",
		"lifespan":    3650,
	})
	if err != nil {
		return "", fmt.Errorf("crear token de larga duración: %w", err)
	}
	var token string
	if err := json.Unmarshal(raw, &token); err != nil || token == "" {
		return "", fmt.Errorf("respuesta inesperada al crear token de larga duración: %s", raw)
	}
	return token, nil
}

func exchangeCode(ctx context.Context, client *http.Client, baseURL, clientID, code string) (string, error) {
	form := url.Values{"grant_type": {"authorization_code"}, "code": {code}, "client_id": {clientID}}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL+"/auth/token", strings.NewReader(form.Encode()))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	var tok struct {
		AccessToken string `json:"access_token"`
	}
	if err := send(client, req, &tok); err != nil {
		return "", err
	}
	if tok.AccessToken == "" {
		return "", errors.New("POST /auth/token: respuesta sin access_token")
	}
	return tok.AccessToken, nil
}

func doJSON(ctx context.Context, client *http.Client, method, url, bearer string, body, out any) error {
	var rd io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return err
		}
		rd = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, url, rd)
	if err != nil {
		return err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if bearer != "" {
		req.Header.Set("Authorization", "Bearer "+bearer)
	}
	return send(client, req, out)
}

func send(client *http.Client, req *http.Request, out any) error {
	what := req.Method + " " + req.URL.Path
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("%s: %w", what, err)
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("%s: %w", what, err)
	}
	if resp.StatusCode/100 != 2 {
		return fmt.Errorf("%s: HTTP %d: %s", what, resp.StatusCode, strings.TrimSpace(string(b)))
	}
	if out == nil {
		return nil
	}
	if err := json.Unmarshal(b, out); err != nil {
		return fmt.Errorf("%s: decodificar respuesta: %w", what, err)
	}
	return nil
}

// writeEnvFile escribe las variables de modo dev para edge-core.
func writeEnvFile(path, baseURL, token string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	content := fmt.Sprintf("# Generado por ha-bootstrap. No versionar.\nDOMOTICA_MODE=dev\nDOMOTICA_HA_URL=%s\nDOMOTICA_HA_TOKEN=%s\n",
		wsURL(baseURL), token)
	return os.WriteFile(path, []byte(content), 0o600)
}

// wsURL convierte la URL base HTTP(S) de HA en la del endpoint WebSocket.
func wsURL(baseURL string) string {
	u := strings.TrimRight(baseURL, "/")
	switch {
	case strings.HasPrefix(u, "https://"):
		u = "wss://" + strings.TrimPrefix(u, "https://")
	case strings.HasPrefix(u, "http://"):
		u = "ws://" + strings.TrimPrefix(u, "http://")
	}
	return u + "/api/websocket"
}
