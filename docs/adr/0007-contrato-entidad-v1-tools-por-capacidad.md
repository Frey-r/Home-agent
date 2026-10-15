# ADR-0007: Contrato de Entidad Normalizada v1 y Tools por Capacidad

- **Estado**: Aceptado
- **Fecha**: 2026-10-15
- **Autores**: Equipo de Desarrollo / Agente de IA
- **Contexto del Proyecto**: `edge-core` (Go), módulo `contracts/`, cliente WebSocket de Home Assistant y AI Harness

---

## 1. Contexto y Planteamiento del Problema

En Home Assistant, las entidades presentan formatos heterogéneos, estados no estructurados (strings en `state`), atributos opcionales dispersos en un diccionario genérico, listas de `supported_features` expresadas en máscaras de bits específicas por integración y esquemas de servicios variables.

Para que un modelo de lenguaje (LLM) opere de forma confiable, segura y determinista sobre el hogar, el orquestador en el edge (`edge-core`) debe transformar el universo de HA a un contrato canónico:
1. **Representación unificada**: estados estrictamente tipados por categoría sin campos arbitrarios.
2. **Capacidades explícitas**: cada acción posible debe acompañarse de un esquema de parámetros validable y un nivel de riesgo riguroso para guardrails.
3. **Optimización de contexto para LLM**: preservación del prompt cache de los proveedores de LLM evitando que cambios frecuentes de estado o adición de dispositivos invaliden esquemas de herramientas.
4. **Resiliencia y sincronización local**: mantener el estado en memoria actualizado en tiempo real (< 1 s) ante caídas de red, reinicios de HA y tormentas de eventos.

---

## 2. Decisiones de Diseño Tomadas

### 1. Contrato de Entidad v1
- **Identidad y origen**:
  - Identificador canónico: `id: "ha:<entity_id>"`.
  - Origen: `source{kind: "ha", entity_id, device_id}`.
  - `device_class`: campo de primer nivel extraído de `attributes.device_class` (no del registro, para reflejar el estado actual del dispositivo).
  - `last_changed`: timestamp ISO 8601 del último cambio de estado.
- **Ubicación**:
  - `area`: objeto estructurado `{id, name}` o `null`.
- **Nombre y alias**:
  - `name`: resuelto desde `friendly_name` (aprovecha la resolución nativa de HA con overrides de usuario).
  - `aliases`: normalizados y saneados (sin valores nulos, sin duplicados, excluyendo el nombre principal, máximo 10 alias de 64 caracteres).
- **Exposición (`exposed`)**:
  - Obligatorio y **fail-closed**: si una entidad no aparece en la lista de exposición o carece de valor booleano explícito, se considera no expuesta (`exposed: false`).
  - Las entidades no expuestas se mantienen en el directorio para métricas/reglas internas pero quedan completamente ocultas al LLM (filtro L0).
- **Disponibilidad (`available`)**:
  - Estado `unavailable` de HA mapea a `available: false`, con `state: {}` vacío y **sin eliminar capacidades**, garantizando que el conjunto de tools presentado al LLM no fluctúe.
  - Estado `unknown` omite el campo `available` o lo mantiene true según disponibilidad de transporte.
- **Estado (`state`)**:
  - Estrictamente tipado por categoría mediante subesquemas `if`/`then` en JSON Schema, con `additionalProperties: false`.
- **Unidades**:
  - Sin conversiones arbitrarias; se preserva la unidad de temperatura reportada por el sistema (`temp_unit` desde `get_config.unit_system`).
- **Texto libre no confiable (L4)**:
  - Campos de entrada de usuario (`name`, `aliases`, `area.name`, `media_title`, ítems de `todo`) son catalogados como L4 para protección contra inyección de prompt.

### 2. Capacidades y Parámetros Estrictos
Cada entidad expone una lista de capacidades:
`{action, kind: "command"|"query", risk, requires_code, params}`.
- `params` es un JSON Schema de objeto en un dialecto restringido, compatible tanto con el modo Structured Outputs / Strict Mode de OpenAI como con Anthropic Tool Calling:
  - Se permite: `type`, `properties`, `required`, `enum`, `minimum`, `maximum`, `minLength`, `maxLength`, `items`, `additionalProperties: false`.
  - Se prohíbe: `oneOf`, `anyOf`, `allOf`, `format`, `pattern` (para garantizar interoperabilidad total).

### 3. Invariante de Forma de Capacidades
Cada par `(category, action)` posee una definición canónica de parámetros compartida. Cada entidad concreta utiliza un subconjunto con los mismos nombres y tipos de parámetros, variando únicamente cotas numéricas o conjuntos enums.
Esto permite al AI Harness (M2) consolidar herramientas por `(category, action)` admitiendo `targets: string[]`. El parámetro `targets` no se modela como un `enum` de entidades, protegiendo así el prompt cache del LLM ante cambios en el número de dispositivos.

### 4. Niveles de Riesgo y Seguridad
Cada acción clasifica su riesgo en: `low`, `medium`, `high`, `critical`. El riesgo global de la entidad es `entity.risk = max(riesgo de sus comandos)`:
- `unlock`, `unlatch` (`lock.open`) → `critical`; `lock` → `medium`.
- Alarma: `disarm` → `critical`; `arm` → `high`; `trigger` queda excluido del agente.
- Covers: tipo garaje, portón o puerta `open`, `close`, `set_position` → `critical`; tipo ventana → `high`; persianas/cortinas generales → `medium`; `stop` siempre es `low` (función de parada de emergencia).
- Switch, clima y scripts → `medium`. Luces, reproductores multimedia, listas todo y consultas (queries) → `low`.
- Escenas: riesgo dinámico calculado como `max(medium, riesgo de sus miembros)`.
- El LLM nunca recibe ni manipula códigos de seguridad (`requires_code: true`); la inyección de PINs o tokens es responsabilidad exclusiva de los guardrails (M2b).

### 5. Librería de Validación JSON Schema
Se adopta `github.com/google/jsonschema-go` v0.4.3:
- Cero dependencias externas.
- Misma versión y estructura requerida por el SDK de MCP (M9).
- Soporte para esquemas tipados en Go que facilita la validación de `params` en tiempo de compilación y ejecución.

### 6. Módulo `contracts/`
- Ubicado en la raíz del repositorio (`/contracts`) con `package contracts`.
- Embebe los esquemas JSON (`schemas/entity.v1.json`) usando `go:embed`.
- Incluido en `go.work` y requerido por `edge/go.mod` mediante directiva `replace ../contracts`.

### 7. Segregación de Interfaces en `ha`
El paquete `ha` expone tipos concretos y métodos específicos (`Conn`, `Command`, `CallService`, `Registries`, `EntityDetails`, `Exposed`). Los consumidores (`Syncer`, `tools.ServiceCaller`) declaran interfaces pequeñas y enfocadas en sus necesidades.

### 8. Sincronización en Tiempo Real (`Directory` y `Syncer`)
- Secuencia de arranque: 1º Suscribirse a eventos WebSocket (`state_changed`, actualizaciones de registros), 2º Realizar snapshot de entidades.
- Goroutine única escritora en el `Directory`.
- Buffer acotado de eventos concurrentes recibidos durante el snapshot: se aplican con la condición `event.last_updated > snapshot.last_updated`. Tras el snapshot, los eventos se aplican secuencialmente sin comprobación de reloj (resiliencia ante saltos de RTC en SBCs).
- Debounce de 300 ms para ráfagas de eventos de registro.
- Sondeo periódico de exposición cada 60 s para capturar cambios en entidades no registradas.
- El `Directory` reside 100% en memoria RAM (inmutable copy-on-write para lectores concurrentes, sin escrituras en disco en M1).

### 9. Resiliencia y Reconexión (`Supervisor`)
- Backoff exponencial de 0,5 s a 30 s con jitter completo.
- Heartbeat activo con `Ping` cada 30 s para detectar conexiones zombi.
- Reset de backoff a valor mínimo tras un intervalo de salud continua.
- Errores de autenticación (`ErrAuthInvalid`) son terminales (evita bloqueos de IP en HA).
- `CallService` no se reintenta automáticamente para prevenir doble ejecución de comandos físicos.

### 10. Protocolo WebSocket Robusto
- Reserva previa de ID de mensaje antes de la transmisión.
- Negociación opcional de `coalesce_messages` recibiendo lotes en arrays JSON.
- Canales de suscripción con buffer: si un consumidor se satura, emite `ErrSlowConsumer` y gatilla resincronización limpia en lugar de bloquear el `readLoop`.
- Cierre y drenaje coordinado bajo `c.mu` esperando la terminación de goroutines.

### 11. Relaciones y Grafo sobre SQLite
- Se modela la topología de la casa como grafo pero se almacena en SQLite relacional (en M4). No se introducen motores de grafos externos (Neo4j, Memgraph, Kùzu).
- El contrato v1 expone identificadores relacionales estables (`area.id`, `source.device_id`, `scene.members`).

### 12. Sanitización y Privacidad en Fixtures de Prueba
- Herramienta de captura (`ha-capture`) condicionada exclusivamente al perfil dev ("Casa Dev" con integración `demo`).
- Redacción automática de secretos, tokens de acceso y fotografías personales.
- Filtrado estricto de dominios no soportados (`person`, `device_tracker`).

---

## 3. Análisis de Trade-Offs Duales

### 3.1. Impacto para Nosotros como Proveedores
- **Costos Operativos (OPEX)**:
  - Cero costo de infraestructura adicional en nube; la normalización, sincronización y validación ocurren 100% en el hardware local (Edge-First).
  - Maximización de cache hits en LLM API Gateway: el invariante de forma de tools evita invalidar prompts ante variaciones de dispositivos o estados en el hogar.
- **Mantenibilidad y Soporte**:
  - Detección temprana de incompatibilidades de HA mediante validación exhaustiva contra esquemas JSON embebidos.
  - Fail-closed en exposición minimiza incidentes de seguridad y consultas no deseadas.
- **Velocidad de Iteración**:
  - Contrato unificado desacopla completamente el desarrollo del agente y herramientas (M2) de los detalles de bajo nivel de la API de Home Assistant.

### 3.2. Impacto para el Cliente Final
- **Latencia y Experiencia de Usuario**:
  - Directorio en memoria permite consultas instantáneas en microsegundos sin I/O de disco para construir contextos de conversación.
  - Tiempos de propagación de eventos sub-segundo (< 1 s) desde que ocurre un cambio en HA hasta que está disponible en el asistente.
- **Privacidad y Seguridad**:
  - El LLM solo ve entidades explícitamente expuestas a Assist.
  - Dispositivos críticos (cerraduras, alarmas, puertas de garaje) protegidos por niveles de riesgo rigurosos y confirmaciones obligatorias.
- **Continuidad Operativa**:
  - Funcionamiento ininterrumpido en red local sin conexión a internet.
  - Recuperación automática ante reinicios de Home Assistant o microcortes de red LAN.

### 3.3. Evaluación Cloudflare-First
- En esta etapa (M1), todo el procesamiento y almacenamiento reside en el Edge local (`edge-core`). Los contratos de esquemas definidos en `contracts/` serán compartidos en etapas posteriores con los Workers y Cloudflare Containers mediante esquemas JSON estándar.

---

## 4. Consecuencias

### Positivas (+)
- Base sólida, tipada y determinista para la construcción de tools en M2.
- Pruebas reproducibles con fixtures reales y simulador `hatest` avanzado.
- Cero asignaciones excesivas de memoria; concurrencia segura sin race conditions.

### Negativas / Deuda Asumida (-)
- Entidades complejas o dominios adicionales (aspiradoras, válvulas, humidificadores) diferidos para milestones posteriores.
- Si una entidad no registrada en HA cambia su configuración de exposición, el refresco puede demorar hasta 60 segundos (mitigado con revalidación bajo demanda en M2b).

---

## 5. Referencias
- [docs/arquitectura.md](../arquitectura.md) (§7.1 Entidad Normalizada v1).
- [ADR-0005: Integración con HA vía WebSocket API](0005-integracion-ha-via-websocket-api.md).
- [ADR-0006: Persistencia Edge con SQLite](0006-persistencia-edge-sqlite-modernc.md).
