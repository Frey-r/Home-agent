// Comando ha-bootstrap: herramienta de desarrollo que completa el onboarding
// de un Home Assistant recién creado (dev/docker-compose.yml) y genera un
// token de larga duración en dev/.env para edge-core.
package main

import (
	"context"
	"errors"
	"net/http"
	"time"
)

func main() {}

func bootstrap(ctx context.Context, baseURL, username, password string, client *http.Client) (string, error) {
	return "", errors.New("ha-bootstrap: no implementado")
}

func waitReady(ctx context.Context, baseURL string, client *http.Client, interval time.Duration) error {
	return errors.New("ha-bootstrap: no implementado")
}

func writeEnvFile(path, baseURL, token string) error {
	return errors.New("ha-bootstrap: no implementado")
}

func wsURL(baseURL string) string { return "" }
