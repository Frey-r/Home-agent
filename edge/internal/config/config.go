// Package config carga y valida la configuración de edge-core según el modo
// de ejecución (dev, addon, docker).
package config

import "errors"

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
	return Config{}, errors.New("config: no implementado")
}
