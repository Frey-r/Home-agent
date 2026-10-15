package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Frey-r/Home-agent/edge/internal/ha"
)

var supportedDomains = map[string]bool{
	"light":               true,
	"switch":              true,
	"cover":               true,
	"lock":                true,
	"alarm_control_panel": true,
	"climate":             true,
	"media_player":        true,
	"sensor":              true,
	"binary_sensor":       true,
	"scene":               true,
	"script":              true,
	"todo":                true,
	"weather":             true,
}

func main() {
	haURL := flag.String("url", "http://localhost:8123", "URL base de Home Assistant")
	token := flag.String("token", "", "Token de larga duración (o usa DOMOTICA_HA_TOKEN)")
	outDir := flag.String("out", filepath.Join("edge", "internal", "entities", "testdata", "demo-2026.10.1"), "Directorio de destino para fixtures")
	flag.Parse()

	if *token == "" {
		*token = os.Getenv("DOMOTICA_HA_TOKEN")
	}
	if *token == "" {
		envPath := filepath.Join("dev", ".env")
		if data, err := os.ReadFile(envPath); err == nil {
			for _, line := range strings.Split(string(data), "\n") {
				line = strings.TrimSpace(line)
				if strings.HasPrefix(line, "DOMOTICA_HA_TOKEN=") {
					*token = strings.TrimPrefix(line, "DOMOTICA_HA_TOKEN=")
				}
				if *haURL == "http://localhost:8123" && strings.HasPrefix(line, "DOMOTICA_HA_URL=") {
					*haURL = strings.TrimPrefix(line, "DOMOTICA_HA_URL=")
				}
			}
		}
	}

	if *token == "" {
		fmt.Fprintln(os.Stderr, "ha-capture: falta el token de Home Assistant (especificar -token o DOMOTICA_HA_TOKEN)")
		os.Exit(1)
	}

	ws := strings.TrimRight(*haURL, "/")
	if !strings.HasSuffix(ws, "/api/websocket") {
		switch {
		case strings.HasPrefix(ws, "https://"):
			ws = "wss://" + strings.TrimPrefix(ws, "https://") + "/api/websocket"
		case strings.HasPrefix(ws, "http://"):
			ws = "ws://" + strings.TrimPrefix(ws, "http://") + "/api/websocket"
		default:
			ws = ws + "/api/websocket"
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	conn, err := ha.Connect(ctx, ws, *token)
	if err != nil {
		fmt.Fprintf(os.Stderr, "ha-capture: conexión fallida: %v\n", err)
		os.Exit(1)
	}
	defer conn.Close()

	if err := runCapture(ctx, conn, *outDir); err != nil {
		fmt.Fprintf(os.Stderr, "ha-capture: error: %v\n", err)
		os.Exit(1)
	}
	slog.Info("captura completada exitosamente", "dir", *outDir)
}

func runCapture(ctx context.Context, conn *ha.Conn, outDir string) error {
	cfg, err := conn.GetConfig(ctx)
	if err != nil {
		return fmt.Errorf("obtener config de HA: %w", err)
	}
	if cfg.LocationName != "Casa Dev" {
		return fmt.Errorf("rechazado: ha-capture solo puede ejecutarse contra la instancia de demo (esperado location_name='Casa Dev', recibido %q)", cfg.LocationName)
	}

	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return fmt.Errorf("crear directorio %s: %w", outDir, err)
	}

	// 1. Estados
	if rawStates, err := conn.Command(ctx, map[string]any{"type": "get_states"}); err == nil {
		var states []map[string]any
		if err := json.Unmarshal(rawStates, &states); err == nil {
			var filtered []map[string]any
			for _, s := range states {
				entityID, _ := s["entity_id"].(string)
				parts := strings.SplitN(entityID, ".", 2)
				if len(parts) != 2 || !supportedDomains[parts[0]] {
					continue
				}
				// Sanitizar atributos secretos
				if attrs, ok := s["attributes"].(map[string]any); ok {
					delete(attrs, "access_token")
					delete(attrs, "entity_picture")
					for k := range attrs {
						kl := strings.ToLower(k)
						if strings.Contains(kl, "token") || strings.Contains(kl, "password") || strings.Contains(kl, "secret") {
							delete(attrs, k)
						}
					}
				}
				filtered = append(filtered, s)
			}
			if err := writeJSON(filepath.Join(outDir, "states.json"), filtered); err != nil {
				return err
			}
		}
	}

	// 2. Registros (Áreas, Dispositivos, Entidades)
	if rawAreas, err := conn.Command(ctx, map[string]any{"type": "config/area_registry/list"}); err == nil {
		var areas any
		_ = json.Unmarshal(rawAreas, &areas)
		_ = writeJSON(filepath.Join(outDir, "areas.json"), areas)
	}
	if rawDevices, err := conn.Command(ctx, map[string]any{"type": "config/device_registry/list"}); err == nil {
		var devices any
		_ = json.Unmarshal(rawDevices, &devices)
		_ = writeJSON(filepath.Join(outDir, "devices.json"), devices)
	}
	if rawEntities, err := conn.Command(ctx, map[string]any{"type": "config/entity_registry/list"}); err == nil {
		var entities any
		_ = json.Unmarshal(rawEntities, &entities)
		_ = writeJSON(filepath.Join(outDir, "entities.json"), entities)
	}

	// 3. Expuestos y Servicios
	if rawExposed, err := conn.Command(ctx, map[string]any{"type": "homeassistant/expose_entity/list"}); err == nil {
		var exposed any
		_ = json.Unmarshal(rawExposed, &exposed)
		_ = writeJSON(filepath.Join(outDir, "exposed.json"), exposed)
	}
	if rawServices, err := conn.Command(ctx, map[string]any{"type": "get_services"}); err == nil {
		var services any
		_ = json.Unmarshal(rawServices, &services)
		_ = writeJSON(filepath.Join(outDir, "services.json"), services)
	}

	// 4. Config
	_ = writeJSON(filepath.Join(outDir, "config.json"), cfg)

	return nil
}

func writeJSON(path string, data any) error {
	b, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return err
	}
	b = append(b, '\n')
	return os.WriteFile(path, b, 0o644)
}
