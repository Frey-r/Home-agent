package main

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"domotica/edge/internal/config"
	"domotica/edge/internal/ha"
	"domotica/edge/internal/ha/hatest"
)

func lookupFrom(env map[string]string) config.LookupFunc {
	return func(key string) (string, bool) {
		v, ok := env[key]
		return v, ok
	}
}

func TestRun(t *testing.T) {
	states := []any{
		hatest.State("light.living", "on", nil),
		hatest.State("lock.front_door", "locked", nil),
		hatest.State("cover.garage_door", "closed", map[string]any{"device_class": "garage"}),
	}

	devEnv := func(u, d string) map[string]string {
		return map[string]string{"DOMOTICA_MODE": "dev", "DOMOTICA_HA_URL": u, "DOMOTICA_HA_TOKEN": "test-token", "DOMOTICA_DATA_DIR": d}
	}

	tests := []struct {
		name       string
		args       []string
		env        func(srvURL, dataDir string) map[string]string
		configSeq  []any     // respuestas sucesivas de get_config (nil = RUNNING)
		wantErr    string    // subcadena esperada en el error
		wantErrIs  error     // errors.Is esperado
		wantLogs   []string  // subcadenas esperadas en el log
		wantDBFile bool
	}{
		{
			// Regresión CI M0: no sincronizar una casa a medias mientras HA arranca.
			name: "espera a que HA termine de arrancar antes de sincronizar",
			args: []string{"run", "--once"},
			env:  devEnv,
			configSeq: []any{
				hatest.Config("STARTING", "2026.10.0"),
				hatest.Config("RUNNING", "2026.10.0"),
			},
			wantLogs: []string{"esperando a que Home Assistant termine de arrancar", "entities=3"},
		},
		{
			name: "run --once conecta y loguea entidades",
			args: []string{"run", "--once"},
			env: func(u, d string) map[string]string {
				return map[string]string{"DOMOTICA_MODE": "dev", "DOMOTICA_HA_URL": u, "DOMOTICA_HA_TOKEN": "test-token", "DOMOTICA_DATA_DIR": d}
			},
			wantLogs:   []string{"entities=3", "ha_version=2026.10.0"},
			wantDBFile: true,
		},
		{
			name: "config inválida falla rápido",
			args: []string{"run", "--once"},
			env: func(u, d string) map[string]string {
				return map[string]string{"DOMOTICA_HA_URL": u, "DOMOTICA_DATA_DIR": d}
			},
			wantErr: "DOMOTICA_HA_TOKEN",
		},
		{
			name: "token rechazado por HA",
			args: []string{"run", "--once"},
			env: func(u, d string) map[string]string {
				return map[string]string{"DOMOTICA_HA_URL": u, "DOMOTICA_HA_TOKEN": "malo", "DOMOTICA_DATA_DIR": d}
			},
			wantErrIs: ha.ErrAuthInvalid,
		},
		{
			name:    "sin subcomando",
			args:    nil,
			env:     func(u, d string) map[string]string { return nil },
			wantErr: "uso",
		},
		{
			name:    "subcomando desconocido",
			args:    []string{"volar"},
			env:     func(u, d string) map[string]string { return nil },
			wantErr: "volar",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts := hatest.Options{States: states, HAVersion: "2026.10.0"}
			if tt.configSeq != nil {
				opts.ResultSeq = map[string][]any{"get_config": tt.configSeq}
			}
			srv := hatest.New(t, opts)
			dataDir := filepath.Join(t.TempDir(), "data") // no existe: run debe crearlo
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			var logs bytes.Buffer
			err := run(ctx, tt.args, lookupFrom(tt.env(srv.URL, dataDir)), &logs)

			switch {
			case tt.wantErr != "" || tt.wantErrIs != nil:
				if err == nil {
					t.Fatalf("run() error = nil; logs:\n%s", logs.String())
				}
				if tt.wantErr != "" && !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("run() error = %q, debe contener %q", err, tt.wantErr)
				}
				if tt.wantErrIs != nil && !errors.Is(err, tt.wantErrIs) {
					t.Fatalf("run() error = %v, want errors.Is %v", err, tt.wantErrIs)
				}
				return
			case err != nil:
				t.Fatalf("run() error inesperado: %v; logs:\n%s", err, logs.String())
			}

			for _, want := range tt.wantLogs {
				if !strings.Contains(logs.String(), want) {
					t.Errorf("logs no contienen %q:\n%s", want, logs.String())
				}
			}
			if tt.wantDBFile {
				if _, err := os.Stat(filepath.Join(dataDir, "domotica.db")); err != nil {
					t.Errorf("no se creó domotica.db: %v", err)
				}
			}
		})
	}
}
