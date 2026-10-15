//go:build integration

package integration

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/Frey-r/Home-agent/contracts"
	"github.com/Frey-r/Home-agent/edge/internal/entities"
	"github.com/Frey-r/Home-agent/edge/internal/ha"
)

func getRealHAEnv(t *testing.T) (string, string) {
	url, okURL := os.LookupEnv("DOMOTICA_HA_URL")
	token, okTok := os.LookupEnv("DOMOTICA_HA_TOKEN")
	if !okURL || !okTok {
		t.Fatal("faltan DOMOTICA_HA_URL/DOMOTICA_HA_TOKEN: levanta dev/docker-compose.yml y ejecuta ha-bootstrap")
	}
	return url, token
}

// TestRealHA_EntitiesValidateAndCover valida que el 100% de las entidades normalizadas
// desde una instancia real de HA cumplan estrictamente con contracts/schemas/entity.v1.json.
func TestRealHA_EntitiesValidateAndCover(t *testing.T) {
	url, token := getRealHAEnv(t)

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	conn, err := ha.Connect(ctx, url, token)
	if err != nil {
		t.Fatalf("Connect error: %v", err)
	}
	defer conn.Close()

	if err := conn.WaitStarted(ctx, 500*time.Millisecond); err != nil {
		t.Fatalf("WaitStarted error: %v", err)
	}

	dir := entities.NewDirectory()
	syncer := entities.NewSyncer(dir)

	syncCtx, syncCancel := context.WithCancel(ctx)
	defer syncCancel()

	go func() {
		_ = syncer.Run(syncCtx, conn)
	}()

	if err := syncer.WaitSynced(ctx, 10*time.Second); err != nil {
		t.Fatalf("WaitSynced error: %v", err)
	}

	allEntities := dir.List(entities.Filter{})
	if len(allEntities) == 0 {
		t.Fatal("el directorio está vacío tras sincronizar con HA real")
	}

	validator, err := contracts.NewValidator()
	if err != nil {
		t.Fatalf("NewValidator error: %v", err)
	}

	categoriesFound := make(map[string]int)
	for _, ent := range allEntities {
		categoriesFound[ent.Category]++

		raw, err := json.Marshal(ent)
		if err != nil {
			t.Fatalf("Marshal entity %s error: %v", ent.ID, err)
		}

		if err := validator.ValidateEntity(raw); err != nil {
			t.Errorf("entidad %s (%s) falló validación de esquema: %v\nJSON: %s", ent.ID, ent.Category, err, string(raw))
		}
	}

	t.Logf("Total entidades normalizadas en HA real: %d", len(allEntities))
	for cat, count := range categoriesFound {
		t.Logf("  categoría %-20s: %d", cat, count)
	}

	// Verificar presencia de categorías clave de actuadores y sensores en el demo
	essential := []string{"light", "switch", "cover", "lock", "climate", "media_player", "sensor", "binary_sensor"}
	for _, cat := range essential {
		if categoriesFound[cat] == 0 {
			t.Errorf("categoría esperada %q no fue encontrada en HA real", cat)
		}
	}
}

// TestRealHA_ChangeReflectedUnder1s valida que una mutación física en un actuador
// se refleje en el directorio en menos de 1 segundo a través del WebSocket reactivo.
func TestRealHA_ChangeReflectedUnder1s(t *testing.T) {
	url, token := getRealHAEnv(t)

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	conn, err := ha.Connect(ctx, url, token)
	if err != nil {
		t.Fatalf("Connect error: %v", err)
	}
	defer conn.Close()

	if err := conn.WaitStarted(ctx, 500*time.Millisecond); err != nil {
		t.Fatalf("WaitStarted error: %v", err)
	}

	dir := entities.NewDirectory()
	syncer := entities.NewSyncer(dir)

	syncCtx, syncCancel := context.WithCancel(ctx)
	defer syncCancel()

	go func() {
		_ = syncer.Run(syncCtx, conn)
	}()

	if err := syncer.WaitSynced(ctx, 10*time.Second); err != nil {
		t.Fatalf("WaitSynced error: %v", err)
	}

	// Buscar una luz disponible en el directorio
	lights := dir.List(entities.Filter{Category: "light"})
	if len(lights) == 0 {
		t.Fatal("no se encontraron luces en el directorio de HA real")
	}

	target := lights[0]
	rawEntityID := strings.TrimPrefix(target.ID, "ha:")

	// Determinar acción de cambio
	currentPower, _ := target.State["power"].(string)
	targetService := "turn_on"
	targetPower := "on"
	if currentPower == "on" {
		targetService = "turn_off"
		targetPower = "off"
	}

	t.Logf("Probando cambio reactivo en %s (power actual: %s -> objetivo: %s)", target.ID, currentPower, targetPower)

	start := time.Now()
	_, err = conn.CallService(ctx, ha.ServiceCall{
		Domain:  "light",
		Service: targetService,
		Target: ha.ServiceTarget{
			EntityIDs: []string{rawEntityID},
		},
	})
	if err != nil {
		t.Fatalf("CallService error: %v", err)
	}

	// Esperar que el cambio se refleje en el directorio en memoria
	var reflectedIn time.Duration
	var matched bool
	deadline := time.Now().Add(1 * time.Second)
	for time.Now().Before(deadline) {
		ent, ok := dir.Get(target.ID)
		if ok {
			p, _ := ent.State["power"].(string)
			if p == targetPower {
				reflectedIn = time.Since(start)
				matched = true
				break
			}
		}
		time.Sleep(10 * time.Millisecond)
	}

	if !matched {
		ent, _ := dir.Get(target.ID)
		curr := "<nil>"
		if ent != nil {
			p, _ := ent.State["power"].(string)
			curr = p
		}
		t.Fatalf("cambio no reflejado en < 1s (power actual en dir: %s, esperado: %s)", curr, targetPower)
	}

	t.Logf("Cambio de estado reflejado con éxito en: %v (< 1s)", reflectedIn)

	// Restaurar estado original
	restoreService := "turn_on"
	if currentPower != "on" {
		restoreService = "turn_off"
	}
	_, _ = conn.CallService(ctx, ha.ServiceCall{
		Domain:  "light",
		Service: restoreService,
		Target: ha.ServiceTarget{
			EntityIDs: []string{rawEntityID},
		},
	})
}

// TestRealHA_ReconnectResync valida que ante una caída forzada de conexión,
// el supervisor y syncer reconecten y restauren el directorio automáticamente.
func TestRealHA_ReconnectResync(t *testing.T) {
	url, token := getRealHAEnv(t)

	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
	defer cancel()

	dir := entities.NewDirectory()
	syncer := entities.NewSyncer(dir)

	reconnectedCh := make(chan struct{}, 2)
	sup := &ha.Supervisor{
		URL:         url,
		Token:       token,
		WaitStarted: 500 * time.Millisecond,
		Heartbeat:   5 * time.Second,
		PingTimeout: 2 * time.Second,
		HealthyTime: 1 * time.Second,
		Backoff: ha.Backoff{
			Initial: 100 * time.Millisecond,
			Max:     500 * time.Millisecond,
		},
		OnConnected: func() {
			select {
			case reconnectedCh <- struct{}{}:
			default:
			}
		},
	}

	var activeConn *ha.Conn
	session := func(sctx context.Context, conn *ha.Conn) error {
		activeConn = conn
		return syncer.Run(sctx, conn)
	}

	supCtx, supCancel := context.WithCancel(ctx)
	defer supCancel()

	go func() {
		_ = sup.Run(supCtx, session)
	}()

	// 1. Esperar primera conexión y sincronización
	select {
	case <-reconnectedCh:
	case <-time.After(5 * time.Second):
		t.Fatal("timeout esperando primera conexión")
	}

	if err := syncer.WaitSynced(ctx, 5*time.Second); err != nil {
		t.Fatalf("WaitSynced error: %v", err)
	}

	initialCount := dir.Count()
	if initialCount == 0 {
		t.Fatal("directorio vacío tras primera conexión")
	}

	// 2. Provocar caída forzada cerrando la conexión activa
	if activeConn != nil {
		_ = activeConn.Close()
	}

	// 3. Esperar que el supervisor reconecte
	select {
	case <-reconnectedCh:
	case <-time.After(8 * time.Second):
		t.Fatal("supervisor no reconectó tras caída de conexión")
	}

	// 4. Verificar que el directorio se mantiene íntegro tras resincronización
	time.Sleep(500 * time.Millisecond)
	if dir.Count() != initialCount {
		t.Fatalf("recuento de entidades post-reconexión (%d) difiere del inicial (%d)", dir.Count(), initialCount)
	}
}

// TestRealHA_RestartResync valida la resiliencia completa ante un reinicio del contenedor
// de Home Assistant (docker restart domotica-ha).
func TestRealHA_RestartResync(t *testing.T) {
	if _, err := exec.LookPath("docker"); err != nil {
		t.Skip("docker no disponible en el entorno")
	}

	url, token := getRealHAEnv(t)

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	dir := entities.NewDirectory()
	syncer := entities.NewSyncer(dir)

	connectCount := 0
	connectedCh := make(chan struct{}, 5)
	sup := &ha.Supervisor{
		URL:         url,
		Token:       token,
		WaitStarted: 500 * time.Millisecond,
		Heartbeat:   5 * time.Second,
		PingTimeout: 2 * time.Second,
		HealthyTime: 2 * time.Second,
		Backoff: ha.Backoff{
			Initial: 200 * time.Millisecond,
			Max:     1 * time.Second,
		},
		OnConnected: func() {
			connectCount++
			select {
			case connectedCh <- struct{}{}:
			default:
			}
		},
	}

	session := func(sctx context.Context, conn *ha.Conn) error {
		return syncer.Run(sctx, conn)
	}

	supCtx, supCancel := context.WithCancel(ctx)
	defer supCancel()

	go func() {
		_ = sup.Run(supCtx, session)
	}()

	// 1. Esperar arranque inicial
	select {
	case <-connectedCh:
	case <-time.After(10 * time.Second):
		t.Fatal("timeout esperando conexión inicial antes del reinicio")
	}

	if err := syncer.WaitSynced(ctx, 10*time.Second); err != nil {
		t.Fatalf("WaitSynced inicial error: %v", err)
	}

	initialCount := dir.Count()
	t.Logf("Entidades iniciales antes de docker restart: %d", initialCount)

	// 2. Reiniciar contenedor de Home Assistant
	t.Log("Ejecutando docker restart domotica-ha...")
	cmd := exec.CommandContext(ctx, "docker", "restart", "domotica-ha")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("docker restart error: %v, salida: %s", err, string(out))
	}
	t.Log("Contenedor reiniciado, esperando que el supervisor detecte y reconecte...")

	// 3. Esperar que el supervisor reconecte tras la recuperación del contenedor
	reconnected := false
	deadline := time.Now().Add(45 * time.Second)
	for time.Now().Before(deadline) {
		select {
		case <-connectedCh:
			reconnected = true
		default:
		}
		if reconnected && syncer.Connected() && dir.Count() >= initialCount {
			break
		}
		time.Sleep(500 * time.Millisecond)
	}

	if !reconnected || !syncer.Connected() {
		t.Fatalf("el supervisor no logró reconectar y resincronizar tras el reinicio del contenedor (connected=%v, count=%d)", syncer.Connected(), dir.Count())
	}

	t.Logf("Reconexión y resincronización completada con éxito tras reinicio de HA. Entidades: %d", dir.Count())
}
