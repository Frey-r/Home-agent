package entities

import (
	"encoding/json"
	"math/rand/v2"
	"os"
	"path/filepath"
	"sort"
	"testing"

	"github.com/Frey-r/Home-agent/contracts"
	"github.com/Frey-r/Home-agent/edge/internal/ha"
)

func loadDemoFixtures(t *testing.T) ([]ha.State, *ha.Registries, map[string]bool) {
	t.Helper()

	baseDir := filepath.Join("testdata", "demo-2026.10.1")

	// 1. States
	statesData, err := os.ReadFile(filepath.Join(baseDir, "states.json"))
	if err != nil {
		t.Fatalf("ReadFile states.json error: %v", err)
	}
	var rawStates []struct {
		EntityID    string         `json:"entity_id"`
		State       string         `json:"state"`
		Attributes  map[string]any `json:"attributes"`
		LastChanged string         `json:"last_changed"`
		LastUpdated string         `json:"last_updated"`
	}
	if err := json.Unmarshal(statesData, &rawStates); err != nil {
		t.Fatalf("Unmarshal states.json error: %v", err)
	}
	var states []ha.State
	for _, s := range rawStates {
		states = append(states, ha.State{
			EntityID:   s.EntityID,
			State:      s.State,
			Attributes: s.Attributes,
		})
	}

	// 2. Registries
	regs := &ha.Registries{
		Entities: make(map[string]ha.EntityRegistryEntry),
		Devices:  make(map[string]ha.DeviceRegistryEntry),
		Areas:    make(map[string]ha.AreaRegistryEntry),
	}

	if areasData, err := os.ReadFile(filepath.Join(baseDir, "areas.json")); err == nil {
		var areaList []ha.AreaRegistryEntry
		if err := json.Unmarshal(areasData, &areaList); err == nil {
			for _, a := range areaList {
				regs.Areas[a.ID] = a
			}
		}
	}

	if devicesData, err := os.ReadFile(filepath.Join(baseDir, "devices.json")); err == nil {
		var devList []ha.DeviceRegistryEntry
		if err := json.Unmarshal(devicesData, &devList); err == nil {
			for _, d := range devList {
				regs.Devices[d.ID] = d
			}
		}
	}

	if entitiesData, err := os.ReadFile(filepath.Join(baseDir, "entities.json")); err == nil {
		var entList []ha.EntityRegistryEntry
		if err := json.Unmarshal(entitiesData, &entList); err == nil {
			for _, e := range entList {
				regs.Entities[e.EntityID] = e
			}
		}
	}

	// 3. Exposed
	exposed := make(map[string]bool)
	if expData, err := os.ReadFile(filepath.Join(baseDir, "exposed.json")); err == nil {
		var expMap map[string]map[string]any
		if err := json.Unmarshal(expData, &expMap); err == nil {
			for eid, assistants := range expMap {
				if assistants != nil {
					if c, ok := assistants["conversation"].(bool); ok && c {
						exposed[eid] = true
					}
				}
			}
		}
	}

	return states, regs, exposed
}

func TestBuildAll_DemoGolden(t *testing.T) {
	validator, err := contracts.NewValidator()
	if err != nil {
		t.Fatalf("contracts.NewValidator error: %v", err)
	}

	states, regs, exposed := loadDemoFixtures(t)
	if len(states) == 0 {
		t.Fatal("no states loaded from fixtures")
	}

	var built []*contracts.Entity
	categoriesCount := make(map[string]int)

	for _, s := range states {
		ent, err := Build(s, regs, exposed)
		if err != nil {
			t.Errorf("Build(%s) returned error: %v", s.EntityID, err)
			continue
		}
		if ent == nil {
			continue // dominio omitido
		}

		if err := validator.Validate(ent); err != nil {
			t.Errorf("entidad %s (%s) no valida contra entity.v1.json: %v", ent.ID, ent.Category, err)
		}

		built = append(built, ent)
		categoriesCount[ent.Category]++
	}

	if len(built) < 30 {
		t.Fatalf("se esperaban al menos 30 entidades construidas, obtenidas %d", len(built))
	}

	t.Logf("Total entidades normalizadas construidas: %d", len(built))
	for cat, count := range categoriesCount {
		t.Logf("  categoría %-20s: %d", cat, count)
	}
}

func TestCapabilityShapeInvariant(t *testing.T) {
	states, regs, exposed := loadDemoFixtures(t)

	// Mapa: (categoría, acción) -> JSON del esquema de params
	shapes := make(map[string]string)

	for _, s := range states {
		ent, err := Build(s, regs, exposed)
		if err != nil || ent == nil {
			continue
		}

		for _, cap := range ent.Capabilities {
			key := ent.Category + "." + cap.Action
			paramsJSON, err := json.Marshal(cap.Params)
			if err != nil {
				t.Fatalf("marshal params error for %s: %v", key, err)
			}

			if existing, ok := shapes[key]; ok {
				if string(paramsJSON) != existing {
					t.Errorf("invariante de forma rota para %s:\noriginal: %s\nactual:   %s", key, existing, string(paramsJSON))
				}
			} else {
				shapes[key] = string(paramsJSON)
			}
		}
	}
}

func TestBuildAll_DeterministicUnderShuffle(t *testing.T) {
	states, regs, exposed := loadDemoFixtures(t)

	buildAndSort := func(st []ha.State) []string {
		var ids []string
		for _, s := range st {
			ent, err := Build(s, regs, exposed)
			if err != nil || ent == nil {
				continue
			}
			ids = append(ids, ent.ID)
		}
		sort.Strings(ids)
		return ids
	}

	baseline := buildAndSort(states)

	for round := 1; round <= 5; round++ {
		shuffled := append([]ha.State(nil), states...)
		rand.Shuffle(len(shuffled), func(i, j int) {
			shuffled[i], shuffled[j] = shuffled[j], shuffled[i]
		})

		got := buildAndSort(shuffled)
		if len(got) != len(baseline) {
			t.Fatalf("round %d: tamaño diferente: got %d, want %d", round, len(got), len(baseline))
		}
		for i := range got {
			if got[i] != baseline[i] {
				t.Fatalf("round %d item %d: got %s, want %s", round, i, got[i], baseline[i])
			}
		}
	}
}
