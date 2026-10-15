# Plan: Entidades Normalizadas v1 y Cliente HA Completo (Milestone M1)

- **ID del Plan**: `2026-10-15_m1_entidades`
- **Estado**: En Progreso
- **Fecha de Creación**: 2026-10-15
- **Última Actualización**: 2026-10-15
- **ADR Relacionado**: [ADR-0007](../adr/0007-contrato-entidad-v1-tools-por-capacidad.md)

---

## 1. Objetivo y Alcance

Llevar a `edge-core` de un prototipo básico que lee estados a un sistema integral que **comprende la topología de la casa y la mantiene sincronizada en vivo (< 1 s)** sobre el WebSocket de Home Assistant.

### Qué se construye en M1:
1. **Contrato y validador de Entidades Normalizadas v1**: tipado estricto por categoría con `google/jsonschema-go`, capacidades con esquemas restringidos compatibles con OpenAI Strict y Anthropic, y clasificación de riesgo fail-closed.
2. **Cliente HA robusto y extendido**: soporte para subscripción a eventos, pipeline de registros (`entity_registry`, `device_registry`, `area_registry`), detalles de entidades vía `get_entries`, lista de exposición a Assist (`expose_entity/list`), llamada a servicios con `call_service` y heartbeat periódico.
3. **Supervisor de conexión**: reconexión automática con backoff exponencial y jitter, detección de desconexión sin loop caliente y resincronización limpia.
4. **Directorio y Sincronizador (`Directory` y `Syncer`)**: directorio en memoria inmutable para lecturas concurrentes, resolución de colisiones durante snapshots, debounce de eventos de registro y sondeo de exposición.
5. **Adaptadores para 13 dominios**: `light`, `switch`, `cover`, `lock`, `alarm_control_panel`, `climate`, `media_player`, `sensor`, `binary_sensor`, `scene`, `script`, `todo`, `weather`.
6. **CLI y comandos de inspección**: subcomando `edge-core entities dump [--exposed-only] [--category X]`, y actualización en vivo en `edge-core run`.
7. **Simulador `hatest` v2**: pruebas controladas de desconexión, latencia, desbordamiento y ráfagas de eventos.
8. **Fixtures sanitizados y HA sembrado**: generación idempotente de escenarios de prueba con `ha-bootstrap seed`.

### Fuera de alcance (diferido a M2+):
- Dominios adicionales: fan, vacuum, humidifier, water_heater, valve, siren, button, number, select, `input_*`.
- Persistencia de entidades en SQLite (directorio vive 100% en memoria RAM en M1).
- Modos avanzados de clima (swing, humedad objetivo), efectos de luces, tilt de covers, o sound mode de media.
- Integración con LLM y ejecución de tools (M2a/M2b).

---

## 2. Requisitos Previos y Contexto Técnico

- **Componentes**: `contracts/` (nuevo módulo Go), `edge/internal/ha/`, `edge/internal/entities/`, `edge/cmd/edge-core/`, `edge/cmd/ha-bootstrap/`, `edge/cmd/ha-capture/`.
- **Contratos de Datos**: `contracts/schemas/entity.v1.json`.
- **Servicios Cloudflare Involucrados**: Ninguno en M1 (100% Edge Local, Cero OPEX).
- **Dependencias**:
  - `github.com/google/jsonschema-go` v0.4.3
  - `go.uber.org/goleak` v1.3.0 (solo tests)
- **Toolchain**: Go 1.27.0, GCC WinLibs 16.2 para `-race`.

---

## 3. Estrategia de Pruebas Primero (TDD)

### Módulo `contracts`
- [x] `TestSchemas_Compile`: compilación de esquemas JSON embebidos.
- [x] `TestEntitySchema_ValidFixtures`: fixture válido para cada uno de los 13 dominios.
- [x] `TestEntitySchema_InvalidFixtures`: tabla de casos inválidos (id sin `ha:`, riesgo inválido, luz con `unlock`, `oneOf`, `additionalProperties: true`, alias `null`, etc.).
- [x] `TestValidator_ErrorHasPointer`: validación de punteros JSON en errores de validación.
- [x] `TestValidateParams`: validación de parámetros de capacidad contra el esquema definido.

### Paquete `ha` y simulador `hatest` v2
- [x] `TestSubscribeEvents_DeliversInOrder`: entrega secuencial de eventos por suscripción.
- [x] `TestSubscribeEvents_FanIn`: múltiples suscriptores simultáneos.
- [x] `TestSubscribeEvents_EventInSameFrameAsResult`: manejo de evento recibido en el mismo frame que la respuesta a la suscripción.
- [x] `TestReadLoop_CoalescedArray`: soporte de mensajes WebSocket agrupados en array JSON.
- [x] `TestConnect_NegotiatesCoalescing`: negociación del header `coalesce_messages`.
- [x] `TestSubscription_SlowConsumerOverflow`: detección de consumidor lento con `ErrSlowConsumer`.
- [x] `TestSubscription_ClosedOnDrop`: cierre limpio de suscripción ante desconexión.
- [x] `TestSubscription_CloseRacesWithEvents`: concurrencia segura entre cierre y emisión.
- [x] `TestConn_DoneOnDrop`: canal `Done()` notifica desconexión del peer.
- [x] `TestPing`: latido y detección de estado de conexión.
- [x] `TestCallService_Payload`: serialización correcta de `ServiceCall`.
- [x] `TestCallService_ReturnResponse`: soporte para llamadas con retorno de datos.
- [x] `TestCallService_Errors`: propagación de errores de servicio de HA.
- [x] `TestCallService_InvalidInput`: validación de campos obligatorios (`domain`, `service`).
- [x] `TestRegistries_Parses`: pipeline de áreas, dispositivos y entidades.
- [x] `TestRegistries_AllOrNothing`: fallo total si algún registro del pipeline falla.
- [x] `TestEntityDetails_NullEntries`: tolerancia a entradas nulas en `get_entries`.
- [x] `TestExposed_FailClosed`: consideración estricta fail-closed en lista de exposición.
- [x] `TestGetConfig_Units`: parseo de `unit_system` desde configuración de HA.

### Supervisor de Conexión
- [x] `TestBackoff_Delay`: cálculo exponencial con jitter y cotas mínima/máxima.
- [x] `TestSupervisor_ReconnectsAfterDrop`: reconexión automática tras corte de enlace.
- [x] `TestSupervisor_NoHotLoopWhileDown`: espera adecuada de backoff ante servidor caído.
- [x] `TestSupervisor_AuthInvalidIsTerminal`: parada inmediata ante credenciales inválidas.
- [x] `TestSupervisor_WaitsStartedBeforeSession`: espera a estado RUNNING antes de iniciar sesión.
- [x] `TestSupervisor_HeartbeatDetectsFrozenPeer`: corte de sesión ante peer congelado.
- [x] `TestSupervisor_ResetsBackoffAfterHealthy`: restablecimiento de backoff tras período sano.
- [x] `TestSupervisor_CancelMidBackoffNoLeak`: cancelación limpia de contexto sin fuga de goroutines.

### Núcleo de `entities` y Adaptadores
- [x] `TestAttr_Helpers`: helpers de atributos (tolerancia a NaN, conversiones numéricas, tipos erróneos).
- [x] `TestResolveName`: resolución de nombre primario con fallback ordenado.
- [x] `TestResolveAliases`: filtrado y normalización de alias.
- [x] `TestResolveArea`: asignación de área desde entidad o dispositivo.
- [x] `TestAvailability`: normalización de `unavailable` y `unknown`.
- [x] `TestRisk_Table`: clasificación de riesgo por dominio y clase.
- [x] `TestEntityRisk_IsMaxOfCommands`: riesgo de entidad igual al máximo de sus comandos.
- [x] `TestRegistry_UnsupportedSkipped`: filtrado silencioso de dominios no implementados.
- [x] `TestCanonicalActions_MatchSchema`: conformidad entre acciones Go y JSON Schema.
- [x] Tests de adaptadores de actuadores (`Test<Domain>_Build`): `light`, `switch`, `cover`, `lock`, `alarm_control_panel`, `climate`.
- [x] `TestScene_RiskFromMembers`: riesgo de escena heredado de sus entidades miembro.
- [x] `TestBuildAll_DemoGolden`: suite golden sobre fixtures reales del demo.
- [x] `TestBuildAll_NoErrorsAndFullCoverage`: cobertura exhaustiva de todas las entidades del demo.
- [x] `TestBuildAll_ValidatesAgainstSchema`: validación estricta de todas las entidades producidas contra `entity.v1.json`.
- [x] `TestCapabilityShapeInvariant`: invariante de parámetros por par `(category, action)`.
- [x] `TestBuildAll_DeterministicUnderShuffle`: orden determinista independiente del orden de entrada.

### Directorio y Sincronizador
- [ ] `TestDirectory_NoBumpOnIdentical`: no incrementa versión si no hay cambios sustanciales.
- [ ] `TestDirectory_WatchCoalesces`: canal de watch de capacidad 1 coalescente.
- [ ] `TestDirectory_ConcurrentReaders`: concurrencia segura de múltiples lectores simultáneos.
- [ ] `TestSyncer_InitialSnapshot`: inicialización y carga de snapshot inicial.
- [ ] `TestSyncer_StateChangedUnder1s`: propagación de eventos `state_changed` en sub-segundo.
- [ ] `TestSyncer_EventDuringSnapshot_NewerWins`: colisión en snapshot resuelta a favor del evento más reciente.
- [ ] `TestSyncer_EventDuringSnapshot_OlderIgnored`: descarte de eventos obsoletos recibidos durante snapshot.
- [ ] `TestSyncer_EntityAddedAfterStart`: adición de nueva entidad en tiempo de ejecución.
- [ ] `TestSyncer_EntityRemoved`: eliminación de entidad del directorio.
- [ ] `TestSyncer_RegistryStormDebounced`: ráfaga de 50 eventos agrupada en ≤ 2 relecturas.
- [ ] `TestSyncer_ExposureViaRegistryEvent`: actualización de exposición ante evento de registro.
- [ ] `TestSyncer_ExposurePeriodicRefresh`: refresco cíclico de entidades no registradas.
- [ ] `TestSyncer_ResyncAfterDropAll`: resincronización tras desconexión masiva.
- [ ] `TestSyncer_DropDuringSnapshotNoPartialApply`: no aplica estados parciales si se interrumpe el snapshot.
- [ ] `TestSyncer_OverflowTriggersResync`: desbordamiento de buffer gatilla resync automático.
- [ ] `TestSyncer_ConnectedFlag`: bandera `Connected` refleja el estado de transporte.

### CLI y Herramientas
- [ ] `TestEntitiesDump`: subcomando `edge-core entities dump` con filtros `--exposed-only` y `--category`.
- [ ] `TestRun_LiveUpdatesDirectory`: comando `run` mantiene vivo el directorio y registra cambios en log.
- [x] `TestSeed_SendsExpectedCommands`: verificación de comandos de siembra en `ha-bootstrap seed`.
- [x] `TestSeed_Idempotent`: ejecución idempotente de sembrado.
- [x] `TestCapture_RefusesNonDemo`: rechazo de captura en instancias no autorizadas.
- [x] `TestCapture_RedactsSecrets`: sanitización de tokens y datos privados.
- [x] `TestCapture_DropsUnsupportedDomains`: omisión de dominios no soportados.

### Integración con Home Assistant Real
- [ ] `TestRealHA_EntitiesValidateAndCover`: 100% de entidades reales validan contra el esquema.
- [ ] `TestRealHA_ChangeReflectedUnder1s`: cambio físico reflejado en el directorio en < 1 s.
- [ ] `TestRealHA_ReconnectResync`: resincronización ante caída de conexión.
- [ ] `TestRealHA_RestartResync`: resincronización tras reinicio de contenedor de HA (`docker restart`).

---

## 4. Fases de Ejecución y Tareas

### Fase 0: Preparación
- [x] Renombrar módulo a `github.com/Frey-r/Home-agent/edge`.
- [x] Corregir enlaces absolutos `file:///` a relativos en documentación.
- [x] Actualizar CI (`edge.yml`) para usar `actions/checkout@v7`, `actions/setup-go@v7` y loop de `go.work`.
- [x] Cerrar pregunta abierta #7 en el roadmap maestro.
- [x] Validar suite de pruebas en verde con `-race`.

### Fase 1: Arquitectura y Plan
- [x] Redactar ADR-0007 (`docs/adr/0007-contrato-entidad-v1-tools-por-capacidad.md`).
- [x] Registrar ADR-0007 en `docs/adr/README.md`.
- [x] Persistir este plan en `docs/plans/2026-10-15_m1_entidades.md`.
- [x] Registrar plan en `docs/plans/README.md` y actualizar `roadmap_producto.md`.

### Fase 2: Módulo `contracts`
- [x] Inicializar módulo `contracts/` (`go.mod`, `contracts.go`, `contracts_test.go`).
- [x] Definir `contracts/schemas/entity.v1.json` y compilar esquemas con `jsonschema-go`.
- [x] Escribir tests en rojo (`TestSchemas_Compile`, `TestEntitySchema_*`, `TestValidator_*`, `TestValidateParams`).
- [x] Implementar validador de entidades y parámetros (Fase verde).
- [x] Integrar `contracts` en `go.work` y `edge/go.mod`.

### Fase 3: Simulador `hatest` v2 y Eventos en `ha`
- [x] Extender `hatest` con soporte de estado mutable, ráfagas, congelamiento y desconexión.
- [x] Escribir tests en rojo de eventos y suscripciones WebSocket en `ha/`.
- [x] Implementar `SubscribeEvents`, decodificación de eventos, soporte de arrays coalescentes y manejo de `slow_consumer`.
- [x] Verificar con `goleak` que no existan fugas de goroutines.

### Fase 4: Comandos extendidos de `ha`
- [x] Escribir tests en rojo para `CallService`, `Registries`, `EntityDetails`, `Exposed`, `GetConfig`.
- [x] Implementar métodos en `edge/internal/ha/` (Fase verde).
- [x] Verificar manejo de errores y pipeline de registros todo-o-nada.

### Fase 5: Sembrado (`ha-bootstrap seed`) y Captura (`ha-capture`)
- [x] Actualizar `dev/ha-config/configuration.yaml` con escenas (`modo_noche`, `peliculas`) y script (`saludo`).
- [x] Escribir tests en rojo para `ha-bootstrap seed` y `ha-capture`.
- [x] Implementar comando `ha-bootstrap seed` idempotente.
- [x] Implementar herramienta `ha-capture` con sanitización de secretos.
- [x] Capturar y versionar fixtures en `edge/internal/entities/testdata/demo-2026.10.1/`.

### Fase 6: Supervisor de Conexión (`ha/supervisor.go`)
- [x] Escribir tests en rojo de reconexión, backoff, jitter y heartbeat.
- [x] Implementar `Backoff` y `Supervisor` en `edge/internal/ha/supervisor.go`.
- [x] Verificar con `goleak` la ausencia de fugas ante cancelaciones.

### Fase 7: Núcleo de `entities` y Adaptadores de Actuadores
- [x] Escribir tests en rojo de helpers de atributos, resolución de nombres, áreas, disponibilidad y riesgo.
- [x] Escribir tests en rojo para adaptadores: `light`, `switch`, `cover`, `lock`, `alarm_control_panel`, `climate`.
- [x] Implementar adaptadores y registro central de entidades (Fase verde).
- [x] Verificar invariante de parámetros de capacidades.

### Fase 8: Resto de Adaptadores y Pruebas Golden
- [x] Escribir tests en rojo para `media_player`, `sensor`, `binary_sensor`, `scene`, `script`, `todo`, `weather`.
- [x] Implementar adaptadores restantes y cálculo de riesgo heredado en escenas.
- [x] Implementar `TestBuildAll_DemoGolden`, cobertura del 100% y validación contra `entity.v1.json`.

### Fase 9: Directorio y Sincronizador (`Directory` y `Syncer`)
### Fase 9: Directorio y Syncer Reactivo
- [x] Escribir tests en rojo de `Directory` (concurrencia, watch coalescente, deduplicación de versión).
- [x] Escribir tests en rojo de `Syncer` (snapshot, colisión de eventos, altas, bajas, debounce de registro, resync).
- [x] Implementar `Directory` inmutable y `Syncer` en `edge/internal/entities/`.
- [x] Validar rendimiento y resiliencia contra `hatest`.

### Fase 10: Integración en CLI (`edge-core`)
- [ ] Refactorizar `run` para aceptar dependencias inyectables (`stdout`, `stderr`, lookup).
- [ ] Implementar subcomando `edge-core entities dump [--exposed-only] [--category X]`.
- [ ] Integrar `Syncer` en `edge-core run` para actualización continua en memoria y logging de cambios.
- [ ] Tests en rojo y verde de comandos CLI.

### Fase 11: Pruebas de Integración y Cierre
- [ ] Ejecutar `dev/scripts/test.ps1 -Integration` contra HA demo sembrado.
- [ ] Validar tests reales: `TestRealHA_EntitiesValidateAndCover`, `TestRealHA_ChangeReflectedUnder1s`, `TestRealHA_ReconnectResync`, `TestRealHA_RestartResync`.
- [ ] Actualizar documentación (`docs/arquitectura.md`, `docs/threat-model.md`).
- [ ] Completar checkboxes y bitácora en este plan.

---

## 5. Bitácora de Progreso y Memoria de Sesiones

- **[2026-10-15 10:25]**:
  - Fase 0 completada: módulo renombrado a `github.com/Frey-r/Home-agent/edge`, enlaces relativos corregidos, CI preparado con `actions/checkout@v7` y `actions/setup-go@v7`, pregunta #7 cerrada en el roadmap. Tests existentes validados en verde con `-race`.
  - Fase 1 completada: ADR-0007 redactado y registrado en `docs/adr/README.md`. Plan M1 formateado y registrado en `docs/plans/README.md`.
  - Estado: listo para iniciar Fase 2 (creación del módulo `contracts/` y primeros tests de esquema).
- **[2026-10-15 10:55]**:
  - Fase 2 completada: módulo `contracts/` creado, `entity.v1.json` con 13 categorías y validación de parámetros, structs en Go e integración en `go.work`. Tests en verde con `-race`.
  - Fase 3 completada: `hatest` v2 extendido con estado mutable y ráfagas; `events.go` y suscripciones WebSocket con coalescing y `slow_consumer`. Cero fugas de goroutines con `goleak`.
  - Fase 4 completada: `CallService`, `Registries`, `EntityDetails`, `Exposed` y `GetConfig` en `edge/internal/ha/`.
  - Fase 5 completada: escenas y scripts en `configuration.yaml`, comando `ha-bootstrap seed` idempotente, herramienta `ha-capture` con sanitización de secretos y fixtures capturadas en `edge/internal/entities/testdata/demo-2026.10.1/`.
  - Estado: listo para iniciar Fase 6 (`Supervisor` de conexión).
- **[2026-10-15 11:00]**:
  - Fase 6 completada: `Backoff` con cálculo exponencial y jitter, `Supervisor` en `edge/internal/ha/supervisor.go` gestionando reconexión automática, detección de desconexión abrupta, control de arranque de HA (`WaitStarted`), heartbeat ante peers congelados, terminalidad de credenciales inválidas y restablecimiento de backoff tras período sano.
  - Validación con `goleak` confirmando cero fugas de goroutines y tests con `-race` en verde.
  - Estado: listo para iniciar Fase 7 (núcleo de `entities` y adaptadores de actuadores).
- **[2026-10-15 11:10]**:
  - Fase 7 completada: helpers de atributos seguros (`attrs.go`), resolución de nombres, alias, áreas y disponibilidad (`resolve.go`), tabla y cálculo de riesgo (`risk.go`), builder y despacho dinámico (`builder.go`), y adaptadores de actuadores (`light`, `switch`, `cover`, `lock`, `alarm_control_panel`, `climate`).
  - Validación estricta de todas las entidades producidas contra `entity.v1.json` con `contracts.Validator`, e invariante de parámetros libres de `oneOf`/`anyOf`/`allOf`. Tests con `-race` en verde.
  - Estado: listo para iniciar Fase 8 (adaptadores restantes y pruebas golden).
- **[2026-10-15 11:17]**:
  - Fase 8 completada: adaptadores restantes (`media_player`, `sensor`, `binary_sensor`, `scene`, `script`, `todo`, `weather`).
  - Cálculo de riesgo heredado en escenas a partir de sus miembros (`sceneRisk`).
  - Suite golden sobre 57 entidades reales del demo HA con 100% de validación estricta contra `entity.v1.json`.
  - Invariante de forma de parámetros por par (categoría, acción) verificado.
  - Determinismo estricto verificado ante barajado aleatorio de entradas. Tests con `-race` en verde.
  - Estado: listo para iniciar Fase 9 (`Directory` y `Syncer`).
- **[2026-10-15 11:23]**:
  - Fase 9 completada: `Directory` en memoria con snapshots inmutables (`atomic.Value`), búsquedas concurrentes lock-free, indexación por categoría y exposición, no-bump ante entidades idénticas y canal de notificación coalescente para observers.
  - `Syncer` reactivo: suscripción previa a eventos antes de snapshot para evitar pérdidas intermedias; debouncing de ráfagas de cambios de registros (áreas, dispositivos, entidades) a 100ms; procesamiento de `state_changed` y actualización en < 1s; descarte de eventos más viejos que el snapshot inicial; manejo seguro de altas y bajas; sin estados parciales si la conexión se cae durante el snapshot.
  - Suite completa de tests pasando con `-race` en todos los módulos de `go.work`.
  - Estado: listo para iniciar Fase 10 (Integración en CLI `edge-core`).




