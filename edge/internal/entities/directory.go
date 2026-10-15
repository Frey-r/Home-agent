package entities

import (
	"reflect"
	"sort"
	"sync"

	"github.com/Frey-r/Home-agent/contracts"
)

// Filter define los criterios de filtrado para consultar entidades del directorio.
type Filter struct {
	Category    string
	ExposedOnly bool
	AreaID      string
}

// Directory es el almacén concurrente en memoria de entidades normalizadas v1 (ADR-0007).
type Directory struct {
	mu       sync.RWMutex
	version  uint64
	entities map[string]*contracts.Entity
	watchers []chan uint64
}

// NewDirectory crea una nueva instancia vacía de Directory.
func NewDirectory() *Directory {
	return &Directory{
		entities: make(map[string]*contracts.Entity),
	}
}

// Version devuelve la versión actual del directorio. Cada mutación real incrementa este contador.
func (d *Directory) Version() uint64 {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.version
}

// Count devuelve el número total de entidades en el directorio.
func (d *Directory) Count() int {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return len(d.entities)
}

// Get obtiene una entidad por su ID normalizado (ha:<dominio>.<objeto>).
func (d *Directory) Get(id string) (*contracts.Entity, bool) {
	d.mu.RLock()
	defer d.mu.RUnlock()
	ent, ok := d.entities[id]
	return ent, ok
}

// List devuelve todas las entidades que coincidan con el filtro dado, ordenadas por ID de forma determinista.
func (d *Directory) List(filter Filter) []*contracts.Entity {
	d.mu.RLock()
	defer d.mu.RUnlock()

	var result []*contracts.Entity
	for _, ent := range d.entities {
		if filter.Category != "" && ent.Category != filter.Category {
			continue
		}
		if filter.ExposedOnly && !ent.Exposed {
			continue
		}
		if filter.AreaID != "" {
			if ent.Area == nil || ent.Area.ID != filter.AreaID {
				continue
			}
		}
		result = append(result, ent)
	}

	sort.Slice(result, func(i, j int) bool {
		return result[i].ID < result[j].ID
	})

	return result
}

// Snapshot devuelve la versión y una copia del slice ordenado de entidades actuales.
func (d *Directory) Snapshot() (uint64, []*contracts.Entity) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	result := make([]*contracts.Entity, 0, len(d.entities))
	for _, ent := range d.entities {
		result = append(result, ent)
	}

	sort.Slice(result, func(i, j int) bool {
		return result[i].ID < result[j].ID
	})

	return d.version, result
}

// Watch devuelve un canal coalescente con buffer 1 que notifica cuando la versión del directorio cambia.
func (d *Directory) Watch() <-chan uint64 {
	d.mu.Lock()
	defer d.mu.Unlock()

	ch := make(chan uint64, 1)
	d.watchers = append(d.watchers, ch)
	return ch
}

func (d *Directory) notifyWatchersLocked() {
	v := d.version
	for _, ch := range d.watchers {
		select {
		case ch <- v:
		default:
			// Coalescente: el receptor ya tiene una notificación pendiente
		}
	}
}

// Update inserta o actualiza una entidad. Si la entidad es idéntica a la existente,
// no incrementa la versión y devuelve false.
func (d *Directory) Update(ent *contracts.Entity) bool {
	if ent == nil {
		return false
	}

	d.mu.Lock()
	defer d.mu.Unlock()

	if existing, ok := d.entities[ent.ID]; ok {
		if reflect.DeepEqual(existing, ent) {
			return false
		}
	}

	d.entities[ent.ID] = ent
	d.version++
	d.notifyWatchersLocked()
	return true
}

// Remove elimina una entidad del directorio por su ID. Devuelve true si la entidad existía.
func (d *Directory) Remove(id string) bool {
	d.mu.Lock()
	defer d.mu.Unlock()

	if _, ok := d.entities[id]; !ok {
		return false
	}

	delete(d.entities, id)
	d.version++
	d.notifyWatchersLocked()
	return true
}

// Apply reemplaza atómicamente todas las entidades del directorio, útil para snapshots iniciales y resync.
func (d *Directory) Apply(entities []*contracts.Entity) {
	d.mu.Lock()
	defer d.mu.Unlock()

	newMap := make(map[string]*contracts.Entity, len(entities))
	for _, ent := range entities {
		if ent != nil {
			newMap[ent.ID] = ent
		}
	}

	d.entities = newMap
	d.version++
	d.notifyWatchersLocked()
}
