# Plan: M0 Fundaciones (monorepo, config, store, cliente HA base, entorno dev)

- **ID del Plan**: `2026-10-15_m0_fundaciones`
- **Estado**: Completado
- **Fecha de Creación**: 2026-10-15
- **Última Actualización**: 2026-10-15
- **ADR Relacionado**:
  - [ADR-0005](../adr/0005-integracion-ha-via-websocket-api.md): WebSocket API de HA.
  - [ADR-0006](../adr/0006-persistencia-edge-sqlite-modernc.md): SQLite con modernc.
- **Roadmap**: [2026-10-15_roadmap_producto.md](2026-10-15_roadmap_producto.md)

---

## 1. Objetivo y Alcance

Dejar listas las bases del edge en Go para que los milestones siguientes se dediquen solo a funcionalidades.

El criterio de aceptación es que `edge-core run --once`, en modo `dev`, haga lo siguiente:

1. Cargue y valide la configuración desde variables de entorno.
2. Abra el SQLite en `DOMOTICA_DATA_DIR` en modo WAL y aplique las migraciones embebidas.
3. Se autentique contra un HA real del `dev/docker-compose.yml` (con `demo:`) y registre en el log la versión de HA y la cantidad de entidades.

Además, `dev/scripts/test.ps1` debe pasar en verde con `-race`.

**Dentro del alcance**:
- `go.work`
- El módulo `edge`
- Los paquetes `config`, `store` y `ha` (auth, `get_states` y comando genérico)
- `cmd/edge-core` (subcomando `run`)
- `cmd/ha-bootstrap` (onboarding de HA y token de larga duración para dev)
- El compose de HA
- El script de tests
- Un workflow de CI
- `.gitignore`

**Fuera de alcance**: todo lo que no sea el cliente HA base.

| Pieza | Milestone |
|---|---|
| Suscripciones a eventos, `call_service`, registros, reconexión | M1 |
| Entidades normalizadas, módulo `contracts` | M1 |
| `llmkit` | M2a |
| Modo `addon` con `options.json` | M6 |

El módulo `contracts` se crea en M1, junto con `entity.v1.json`, para no dejar un módulo vacío.

---

## 2. Requisitos Previos y Contexto Técnico

- **Componentes afectados**:
  - `edge/internal/{config,store,ha}`
  - `edge/internal/ha/hatest` (fake de HA reutilizable en tests)
  - `edge/cmd/{edge-core,ha-bootstrap}`
  - `edge/integration/`
  - `dev/`
  - `.github/workflows/`
- **Contratos y protocolos**:
  - **WebSocket API de HA, autenticación**:
    1. El servidor envía `auth_required`.
    2. El cliente responde con `auth` y su `access_token`.
    3. El servidor contesta `auth_ok` o `auth_invalid`.
  - **WebSocket API de HA, comandos**: cada comando lleva un `id` incremental. El servidor responde `{id, type:"result", success, result|error}`.
  - **Onboarding de HA para dev**:
    1. `POST /api/onboarding/users` devuelve un `auth_code`.
    2. `POST /auth/token` cambia ese código por un `access_token`.
    3. El comando WS `auth/long_lived_access_token` genera el token de larga duración.
- **Servicios de Cloudflare involucrados**: ninguno; todo es local.
- **Impacto comercial**: ninguno sobre el OPEX. Es infraestructura de calidad (Mandato I).
- **Dependencias nuevas**:
  - `modernc.org/sqlite`: SQLite en Go puro.
  - `github.com/coder/websocket`: cliente WebSocket mantenido; también se usa en el fake de los tests.
  - Todo lo demás es biblioteca estándar: `log/slog`, `flag`, `embed`, `testing/fstest`.
- **Configuración en modo dev** (variables de entorno):

  | Variable | Uso |
  |---|---|
  | `DOMOTICA_MODE` | `dev`, `addon` o `docker` |
  | `DOMOTICA_HA_URL` | URL de HA, con esquema `ws` o `wss` |
  | `DOMOTICA_HA_TOKEN` | Token de acceso a HA |
  | `DOMOTICA_DATA_DIR` | Directorio de datos; por defecto `./.data` |
  | `DOMOTICA_LLM_CONFIG` | Opcional; si se define, el archivo debe existir |

  En modo `addon`, la URL por defecto es `ws://supervisor/core/websocket` y el token se toma de `SUPERVISOR_TOKEN`. La lectura de `options.json` queda para M6.

---

## 3. Estrategia de Pruebas Primero (TDD)

Todos los tests son table-driven cuando aplica y se validan con `go test -v -race ./...`.

- [ ] **`TestLoad`** (`edge/internal/config/config_test.go`)
  - Modo dev con todos los datos → carga bien.
  - `DATA_DIR` vacío → usa `./.data` por defecto.
  - Falta `HA_URL` en dev → error.
  - Falta el token → error.
  - `MODE` inválido → error.
  - URL con esquema distinto de `ws` o `wss` → error.
  - URL que no se puede parsear → error.
  - `LLM_CONFIG` apunta a un archivo inexistente → error.
  - `LLM_CONFIG` apunta a un archivo existente → carga bien.
  - Modo `addon`: usa la URL del supervisor y `SUPERVISOR_TOKEN`.
  - Modo `addon` sin `SUPERVISOR_TOKEN` → error.
  - Los errores dicen qué variable falló.
- [ ] **`TestOpen_AppliesPragmas`** (`edge/internal/store/store_test.go`)
  - Verifica `journal_mode=wal`, `foreign_keys=1` y `busy_timeout=5000`.
- [ ] **`TestMigrate`**, con migraciones de prueba en `fstest.MapFS`:
  - Aplica en orden y registra las versiones.
  - Es idempotente: correrlo dos veces deja el mismo estado y no da error.
  - Una migración que falla hace rollback: la versión no avanza y la tabla parcial no existe.
  - Un nombre de archivo inválido → error.
  - Una versión duplicada → error.
- [ ] **`TestOpen_EmbeddedMigrations`**: `Open` aplica `0001_init.sql`, de modo que existen `schema_migrations` y `meta`.
- [ ] **`TestMeta_SetGet`**: ida y vuelta, sobrescritura y clave inexistente (devuelve `ErrNotFound`).
- [ ] **`TestStore_ConcurrentWrites`**: 20 goroutines × 25 escrituras sin errores ni `SQLITE_BUSY`, y todas las claves quedan presentes.
- [ ] **`TestConnect`** (`edge/internal/ha/client_test.go`, contra el fake `hatest`):
  - `auth_ok` → conecta y expone la versión de HA.
  - `auth_invalid` → `ErrAuthInvalid`.
  - El servidor cierra antes del handshake → error.
  - Llega un mensaje inesperado en lugar de `auth_required` → error.
  - El servidor no envía nada y el contexto vence → error de contexto, rápido y sin colgarse.
- [ ] **`TestGetStates_Parses`**: convierte el fixture de estados en `[]State`, con `entity_id`, `state`, `attributes` y timestamps.
- [ ] **`TestCommand_ErrorResult`**: si el servidor responde `success:false`, se devuelve `*CommandError` con su código y mensaje.
- [ ] **`TestConcurrentRequests_CorrelateByID`**: el fake responde en orden inverso y cada llamador recibe su propia respuesta.
- [ ] **`TestServerDropsMidRequest_ReturnsError`**: si el fake corta la conexión con un comando pendiente, el comando devuelve error en lugar de colgarse, y las llamadas posteriores fallan con `ErrClosed`.
- [ ] **`TestRun_Once_LogsEntityCount`** (`edge/cmd/edge-core/run_test.go`): contra el fake HA, verifica que el log contenga `ha_version` y `entities=N` y que se cree `domotica.db` en `DATA_DIR`.
- [ ] **`TestRun_InvalidConfig_ReturnsError`**: el arranque falla rápido y con un mensaje claro.
- [ ] **`TestBootstrap`** (`edge/cmd/ha-bootstrap/bootstrap_test.go`, contra un fake de onboarding + `hatest`): sigue el flujo usuario → auth_code → token → token de larga duración. Si HA ya está onboardeado, devuelve un error claro.
- [ ] **`TestRealHA_Demo`** (`edge/integration/ha_demo_test.go`, tag `integration`): contra el HA del compose, verifica que el `get_states` incluya los dominios `light`, `lock`, `cover` y `climate`.

---

## 4. Fases de Ejecución y Tareas

### Fase 1: Pruebas Iniciales (Rojo)
- [x] Crear `go.work` y `edge/go.mod`, con las dependencias `modernc.org/sqlite` v1.60.1 y `coder/websocket` v1.8.15.
- [x] Escribir los tests de `config`, con stubs mínimos de firmas para que compilen, y comprobar que fallan.
- [x] Escribir los tests de `store` y comprobar que fallan.
- [x] Escribir el fake `hatest` y los tests de `ha`, y comprobar que fallan.
- [x] Escribir los tests de `cmd/edge-core` y `cmd/ha-bootstrap`, y comprobar que fallan.

### Fase 2: Implementación Mínima (Verde)
- [x] Implementar `config.Load`.
- [x] Implementar `store.Open`, `migrate`, `SetMeta`/`GetMeta` y `0001_init.sql`.
- [x] Implementar `ha.Connect`, `Conn.Command`, `Conn.GetStates` y `Conn.Close`.
- [x] Implementar el subcomando `run` de `edge-core` (con `--once`).
- [x] Implementar `ha-bootstrap`.
- [x] Correr `dev/scripts/test.ps1` y confirmar que todo queda en verde con `-race`.

### Fase 3: Refactorización y Calidad
- [x] `go vet` limpio y revisión de errores descartados (todos los `_ =` llevan comentario de justificación).
- [x] Refactor: `ha.Command` rechaza comandos sin `type` (antes entraba en pánico con `nil`). Lo cubre `TestCommand_InvalidInput`, que se escribió primero y falló en rojo.
- [x] Crear `dev/docker-compose.yml` (HA **2026.10.1** fijado, `demo:`, tz `America/Santiago`) y `dev/ha-config/configuration.yaml`.
- [x] Ejecutar `ha-bootstrap`, que genera `dev/.env` (gitignored). Funcionó al primer intento contra el HA real, incluidos los pasos opcionales de onboarding.
- [x] Hacer pasar `TestRealHA_Demo` con `-tags integration`: 123 entidades en 40 dominios.
- [x] Smoke manual con `CGO_ENABLED=0`: `edge-core run --once` registró en el log `ha_version=2026.10.1 entities=123`.
- [x] Cross-compile estático para linux/amd64 (10,9 MB) y linux/arm64 (10,4 MB).
- [x] Crear `.github/workflows/edge.yml` con dos jobs: `test` (vet, `-race`, build multi-arch) e `integration` (HA en Docker, `ha-bootstrap`, `-tags integration`). **No se ha ejecutado**, porque el repo todavía no tiene remoto.

### Fase 4: Documentación y Cierre
- [x] Crear `dev/README.md` explicando cómo levantar HA, hacer el bootstrap y correr los tests.
- [x] Actualizar este plan y el roadmap, y marcar M0 como completado en `docs/plans/README.md`.

---

## 5. Bitácora de Progreso y Memoria de Sesiones

- **[2026-10-15]**: Plan creado. Toolchain verificado: Go 1.27.0 y gcc 16.2 (WinLibs UCRT); `-race` funciona.
- **[2026-10-15]**: M0 completado. Commits: `test(edge)` (rojo) → `feat(edge)` (verde) → `refactor(edge)` → `chore(dev)` (cierre).
  - **Desviaciones respecto al plan**:
    - El módulo `contracts` se crea en M1, junto con `entity.v1.json`, para no dejar un módulo vacío.
    - `golangci-lint` no está configurado; se usa `go vet`, que AGENTS.md §4 permite. Queda pendiente agregarlo en CI cuando haya remoto.
    - `config` agrega `DefaultAddonDataDir=/data` para el modo `addon`.
  - **Notas para M1**:
    - El demo de HA expone `calendar`, `todo` y `weather`, que sirven para el brief de M4.
    - El cliente ya tiene `Command` genérico. M1 suma la suscripción a eventos (`readLoop` hoy ignora todo lo que no sea `result`), `call_service`, los registros y la reconexión con resync.
  - **Entorno**: Docker Desktop mostró un aviso de WSL al arrancar, pero WSL 2 (`Ubuntu`, `docker-desktop`) quedó en `Running` y los contenedores funcionan.
