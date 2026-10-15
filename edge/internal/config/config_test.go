package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func lookupFrom(env map[string]string) LookupFunc {
	return func(key string) (string, bool) {
		v, ok := env[key]
		return v, ok
	}
}

func TestLoad(t *testing.T) {
	existingLLM := filepath.Join(t.TempDir(), "llm.yaml")
	if err := os.WriteFile(existingLLM, []byte("providers: {}\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name       string
		env        map[string]string
		want       Config
		wantErrVar string // si no es vacío, se espera error que mencione esta variable
	}{
		{
			name: "dev completo",
			env: map[string]string{
				"DOMOTICA_MODE":     "dev",
				"DOMOTICA_HA_URL":   "ws://localhost:8123/api/websocket",
				"DOMOTICA_HA_TOKEN": "tok",
				"DOMOTICA_DATA_DIR": "/tmp/domotica",
			},
			want: Config{Mode: ModeDev, HAURL: "ws://localhost:8123/api/websocket", HAToken: "tok", DataDir: "/tmp/domotica"},
		},
		{
			name: "modo por defecto es dev y data dir por defecto",
			env: map[string]string{
				"DOMOTICA_HA_URL":   "wss://ha.example/api/websocket",
				"DOMOTICA_HA_TOKEN": "tok",
			},
			want: Config{Mode: ModeDev, HAURL: "wss://ha.example/api/websocket", HAToken: "tok", DataDir: DefaultDataDir},
		},
		{
			name: "docker exige url y token explícitos",
			env: map[string]string{
				"DOMOTICA_MODE":     "docker",
				"DOMOTICA_HA_URL":   "ws://homeassistant:8123/api/websocket",
				"DOMOTICA_HA_TOKEN": "tok",
			},
			want: Config{Mode: ModeDocker, HAURL: "ws://homeassistant:8123/api/websocket", HAToken: "tok", DataDir: DefaultDataDir},
		},
		{
			name: "addon usa supervisor por defecto",
			env: map[string]string{
				"DOMOTICA_MODE":    "addon",
				"SUPERVISOR_TOKEN": "sup",
			},
			want: Config{Mode: ModeAddon, HAURL: DefaultSupervisorURL, HAToken: "sup", DataDir: DefaultAddonDataDir},
		},
		{
			name: "llm config existente",
			env: map[string]string{
				"DOMOTICA_HA_URL":     "ws://localhost:8123/api/websocket",
				"DOMOTICA_HA_TOKEN":   "tok",
				"DOMOTICA_LLM_CONFIG": existingLLM,
			},
			want: Config{Mode: ModeDev, HAURL: "ws://localhost:8123/api/websocket", HAToken: "tok", DataDir: DefaultDataDir, LLMConfigPath: existingLLM},
		},
		{
			name:       "falta url en dev",
			env:        map[string]string{"DOMOTICA_HA_TOKEN": "tok"},
			wantErrVar: "DOMOTICA_HA_URL",
		},
		{
			name:       "falta token en dev",
			env:        map[string]string{"DOMOTICA_HA_URL": "ws://localhost:8123/api/websocket"},
			wantErrVar: "DOMOTICA_HA_TOKEN",
		},
		{
			name:       "addon sin supervisor token",
			env:        map[string]string{"DOMOTICA_MODE": "addon"},
			wantErrVar: "SUPERVISOR_TOKEN",
		},
		{
			name: "modo inválido",
			env: map[string]string{
				"DOMOTICA_MODE":     "prod",
				"DOMOTICA_HA_URL":   "ws://localhost:8123/api/websocket",
				"DOMOTICA_HA_TOKEN": "tok",
			},
			wantErrVar: "DOMOTICA_MODE",
		},
		{
			name: "url con esquema http",
			env: map[string]string{
				"DOMOTICA_HA_URL":   "http://localhost:8123/api/websocket",
				"DOMOTICA_HA_TOKEN": "tok",
			},
			wantErrVar: "DOMOTICA_HA_URL",
		},
		{
			name: "url imposible de parsear",
			env: map[string]string{
				"DOMOTICA_HA_URL":   "ws://%zz",
				"DOMOTICA_HA_TOKEN": "tok",
			},
			wantErrVar: "DOMOTICA_HA_URL",
		},
		{
			name: "llm config inexistente",
			env: map[string]string{
				"DOMOTICA_HA_URL":     "ws://localhost:8123/api/websocket",
				"DOMOTICA_HA_TOKEN":   "tok",
				"DOMOTICA_LLM_CONFIG": filepath.Join(t.TempDir(), "no-existe.yaml"),
			},
			wantErrVar: "DOMOTICA_LLM_CONFIG",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Load(lookupFrom(tt.env))
			if tt.wantErrVar != "" {
				if err == nil {
					t.Fatalf("Load() error = nil, se esperaba error sobre %s", tt.wantErrVar)
				}
				if !strings.Contains(err.Error(), tt.wantErrVar) {
					t.Fatalf("Load() error = %q, debe mencionar %s", err, tt.wantErrVar)
				}
				return
			}
			if err != nil {
				t.Fatalf("Load() error inesperado: %v", err)
			}
			if got != tt.want {
				t.Fatalf("Load() = %+v, want %+v", got, tt.want)
			}
		})
	}
}
