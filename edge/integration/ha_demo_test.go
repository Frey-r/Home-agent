//go:build integration

package integration

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"domotica/edge/internal/ha"
)

func TestRealHA_Demo(t *testing.T) {
	url, okURL := os.LookupEnv("DOMOTICA_HA_URL")
	token, okTok := os.LookupEnv("DOMOTICA_HA_TOKEN")
	if !okURL || !okTok {
		t.Fatal("faltan DOMOTICA_HA_URL/DOMOTICA_HA_TOKEN: levanta dev/docker-compose.yml y ejecuta ha-bootstrap (ver dev/README.md)")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	c, err := ha.Connect(ctx, url, token)
	if err != nil {
		t.Fatalf("Connect() error: %v", err)
	}
	defer c.Close()

	states, err := c.GetStates(ctx)
	if err != nil {
		t.Fatalf("GetStates() error: %v", err)
	}

	domains := map[string]int{}
	for _, s := range states {
		d, _, _ := strings.Cut(s.EntityID, ".")
		domains[d]++
	}
	t.Logf("HA %s: %d entidades, dominios: %v", c.HAVersion(), len(states), domains)
	for _, want := range []string{"light", "lock", "cover", "climate"} {
		if domains[want] == 0 {
			t.Errorf("el HA demo debería exponer entidades del dominio %q", want)
		}
	}
}
