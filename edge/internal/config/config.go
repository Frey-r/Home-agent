// Package config carga y valida la configuración de edge-core según el modo
// de ejecución (dev, addon, docker).
package config

import (
	"fmt"
	"net/url"
	"os"
)

// Mode es el modo de ejecución de edge-core.
type Mode string

const (
	ModeDev    Mode = "dev"
	ModeAddon  Mode = "addon"
	ModeDocker Mode = "docker"
)

const (
	// DefaultSupervisorURL es el endpoint WS de HA visto desde una app de HA.
	DefaultSupervisorURL = "ws://supervisor/core/websocket"
	// DefaultDataDir es el directorio de datos en modos dev y docker.
	DefaultDataDir = ".data"
	// DefaultAddonDataDir es el volumen persistente de una app de HA.
	DefaultAddonDataDir = "/data"
)

// Variables de entorno reconocidas.
const (
	EnvMode            = "DOMOTICA_MODE"
	EnvHAURL           = "DOMOTICA_HA_URL"
	EnvHAToken         = "DOMOTICA_HA_TOKEN"
	EnvDataDir         = "DOMOTICA_DATA_DIR"
	EnvLLMConfig       = "DOMOTICA_LLM_CONFIG"
	EnvSupervisorToken = "SUPERVISOR_TOKEN"
)

// Config es la configuración validada de edge-core.
type Config struct {
	Mode          Mode
	HAURL         string
	HAToken       string
	DataDir       string
	LLMConfigPath string
}

// LookupFunc tiene la firma de os.LookupEnv; se inyecta para testear.
type LookupFunc func(key string) (string, bool)

// Load lee la configuración desde variables de entorno y la valida.
func Load(lookup LookupFunc) (Config, error) {
	get := func(key string) string {
		v, _ := lookup(key)
		return v
	}

	cfg := Config{
		Mode:          Mode(get(EnvMode)),
		HAURL:         get(EnvHAURL),
		HAToken:       get(EnvHAToken),
		DataDir:       get(EnvDataDir),
		LLMConfigPath: get(EnvLLMConfig),
	}
	if cfg.Mode == "" {
		cfg.Mode = ModeDev
	}

	switch cfg.Mode {
	case ModeDev, ModeDocker:
		if cfg.HAURL == "" {
			return Config{}, missing(EnvHAURL)
		}
		if cfg.HAToken == "" {
			return Config{}, missing(EnvHAToken)
		}
		if cfg.DataDir == "" {
			cfg.DataDir = DefaultDataDir
		}
	case ModeAddon:
		if cfg.HAURL == "" {
			cfg.HAURL = DefaultSupervisorURL
		}
		if cfg.HAToken == "" {
			cfg.HAToken = get(EnvSupervisorToken)
		}
		if cfg.HAToken == "" {
			return Config{}, missing(EnvSupervisorToken)
		}
		if cfg.DataDir == "" {
			cfg.DataDir = DefaultAddonDataDir
		}
	default:
		return Config{}, fmt.Errorf("config: %s=%q inválido (valores: dev, addon, docker)", EnvMode, cfg.Mode)
	}

	if err := validateWSURL(cfg.HAURL); err != nil {
		return Config{}, fmt.Errorf("config: %s: %w", EnvHAURL, err)
	}
	if cfg.LLMConfigPath != "" {
		if _, err := os.Stat(cfg.LLMConfigPath); err != nil {
			return Config{}, fmt.Errorf("config: %s: %w", EnvLLMConfig, err)
		}
	}
	return cfg, nil
}

func missing(key string) error {
	return fmt.Errorf("config: falta la variable %s", key)
}

func validateWSURL(raw string) error {
	u, err := url.Parse(raw)
	if err != nil {
		return err
	}
	if u.Scheme != "ws" && u.Scheme != "wss" {
		return fmt.Errorf("esquema %q no soportado (se requiere ws o wss)", u.Scheme)
	}
	if u.Host == "" {
		return fmt.Errorf("falta el host en %q", raw)
	}
	return nil
}
