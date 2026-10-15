package entities

import (
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/Frey-r/Home-agent/contracts"
)

func boolPtr(b bool) *bool {
	return &b
}

func TestDirectory_NoBumpOnIdentical(t *testing.T) {
	d := NewDirectory()

	ent1 := &contracts.Entity{
		ID:        "ha:light.living",
		Name:      "Luz Living",
		Category:  "light",
		Available: boolPtr(true),
		State:     map[string]any{"power": "on"},
	}

	// Primer insert: version 1
	updated := d.Update(ent1)
	if !updated {
		t.Fatal("expected update=true on initial insert")
	}
	if v := d.Version(); v != 1 {
		t.Fatalf("expected version 1, got %d", v)
	}

	// Insert idéntico: no debe incrementar versión
	entIdentical := &contracts.Entity{
		ID:        "ha:light.living",
		Name:      "Luz Living",
		Category:  "light",
		Available: boolPtr(true),
		State:     map[string]any{"power": "on"},
	}
	updated = d.Update(entIdentical)
	if updated {
		t.Error("expected update=false on identical entity")
	}
	if v := d.Version(); v != 1 {
		t.Errorf("expected version to remain 1, got %d", v)
	}

	// Insert modificado: incrementa a versión 2
	entModified := &contracts.Entity{
		ID:        "ha:light.living",
		Name:      "Luz Living",
		Category:  "light",
		Available: boolPtr(true),
		State:     map[string]any{"power": "off"},
	}
	updated = d.Update(entModified)
	if !updated {
		t.Error("expected update=true on modified entity")
	}
	if v := d.Version(); v != 2 {
		t.Errorf("expected version 2, got %d", v)
	}
}

func TestDirectory_WatchCoalesces(t *testing.T) {
	d := NewDirectory()
	watchCh := d.Watch()

	// 10 actualizaciones rápidas no deben bloquear al escritor
	for i := 1; i <= 10; i++ {
		d.Update(&contracts.Entity{
			ID:        "ha:light.living",
			Name:      "Luz Living",
			Category:  "light",
			Available: boolPtr(true),
			State:     map[string]any{"power": fmt.Sprintf("state_%d", i)},
		})
	}

	// Debe haber una notificación en el canal
	select {
	case v := <-watchCh:
		if v != d.Version() && v < 1 {
			t.Errorf("watch version out of bounds: got %d", v)
		}
	case <-time.After(1 * time.Second):
		t.Fatal("watch channel did not receive notification")
	}
}

func TestDirectory_ConcurrentReaders(t *testing.T) {
	d := NewDirectory()

	for i := 0; i < 20; i++ {
		d.Update(&contracts.Entity{
			ID:        fmt.Sprintf("ha:light.lamp_%d", i),
			Name:      fmt.Sprintf("Lámpara %d", i),
			Category:  "light",
			Available: boolPtr(true),
			State:     map[string]any{"power": "on"},
		})
	}

	var wg sync.WaitGroup
	stop := make(chan struct{})

	// 10 lectores concurrentes
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for {
				select {
				case <-stop:
					return
				default:
					list := d.List(Filter{Category: "light"})
					if len(list) == 0 {
						t.Errorf("reader %d saw empty list", id)
					}
					_, _ = d.Get("ha:light.lamp_5")
				}
			}
		}(i)
	}

	// 2 escritores concurrentes
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func(wID int) {
			defer wg.Done()
			for j := 0; j < 50; j++ {
				d.Update(&contracts.Entity{
					ID:        fmt.Sprintf("ha:light.lamp_%d", j%20),
					Name:      fmt.Sprintf("Lámpara %d (rev %d)", j%20, j),
					Category:  "light",
					Available: boolPtr(true),
					State:     map[string]any{"power": fmt.Sprintf("val_%d_%d", wID, j)},
				})
				time.Sleep(1 * time.Millisecond)
			}
		}(i)
	}

	time.Sleep(100 * time.Millisecond)
	close(stop)
	wg.Wait()
}
