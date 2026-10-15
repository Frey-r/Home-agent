# Arquitectura del sistema: asistente personal y cerebro domótico sobre Home Assistant

- **Codename**: `domotica`
- **Estado**: Vigente. Reemplaza a [arquitectura_teórica.md](../arquitectura_teórica.md), que queda como documento histórico.
- **Última actualización**: 2026-10-15
- **Decisiones que lo sustentan**:
  - [ADR-0003](adr/0003-construir-sobre-home-assistant.md): construir sobre Home Assistant.
  - [ADR-0004](adr/0004-cerebro-edge-nube-cloudflare-relay-durable-objects.md): cerebro en el edge, nube en Cloudflare y relay con Durable Objects.
  - [ADR-0005](adr/0005-integracion-ha-via-websocket-api.md): integración con HA por WebSocket API.
  - [ADR-0006](adr/0006-persistencia-edge-sqlite-modernc.md): persistencia del edge en SQLite (modernc).
- **Plan de ejecución**: [docs/plans/2026-10-15_roadmap_producto.md](plans/2026-10-15_roadmap_producto.md)

---

## 1. Visión

El producto es comercial y tiene dos mitades que comparten un mismo cerebro:

1. **Cerebro domótico.** Controla en lenguaje natural lo que el hogar tiene conectado (luces, clima, cortinas, cerraduras, TV, etc.), con guardrails de seguridad.
2. **Asistente personal del día a día.** Maneja calendario y correo, recordatorios, memoria de largo plazo, listas y brief matinal. Se le habla por chat (Telegram primero) o por voz en casa, y se conecta a servicios locales vía MCP, como el transporte público de Santiago.

**Go-to-market**:

| Fase | Qué se vende |
|---|---|
| 1 | Una app (antes llamada "add-on") que se instala en un Home Assistant OS existente, más una suscripción a la nube. |
| 2 | Un hub propio: hardware con HA OS y el stack preinstalado. |

**Restricción de marca**: el producto **no** puede usar "Home Assistant" ni su logo en su nombre. Ver §10.

---

## 2. Principios de diseño

1. **Home Assistant es la capa IoT.** No escribimos drivers. HA aporta más de 2.000 integraciones, el discovery, Zigbee/Matter/Thread y el pipeline de voz (Assist). Nosotros somos el cerebro.
2. **El cerebro vive en el edge y la nube es delgada.**
   - En la casa corren el loop del agente, la memoria, el contexto, los guardrails y la voz.
   - La nube cumple solo cinco funciones: relay de mensajes, gateway de LLM, OAuth expuesto como MCP, cuentas y medición.
3. **La nube nunca actúa sobre dispositivos.** Toda acción pasa por los guardrails del edge.
4. **Cloudflare primero** (Mandato III de [AGENTS.md](../AGENTS.md)). Cualquier excepción se justifica con números en un ADR.
5. **El proveedor de LLM es una commodity.** Se soportan los formatos de wire de OpenAI (Chat Completions) y de Anthropic (Messages). Cambiar de proveedor o de modelo es solo configuración. Ver §6.
6. **Todo lo externo vive detrás de una interfaz.** HA, LLM, canales y MCP entran por interfaces Go, así que pasar de modo dev (conexiones directas) a producción (gateway + relay) es un cambio de configuración, no un rewrite.

---

## 3. Vista general

```
 Telegram ─webhook─► ┌──────────── CLOUDFLARE (Workers, TypeScript) ─────────────────────┐
                     │ api: cuentas, pairing, JWT dispositivo, webhook Telegram   [D1, KV]│
                     │ relay: DO HouseholdRelay (WS hibernation + outbox SQLite)          │
                     │ llm-gw: /anthropic/v1/messages → AI Gateway (BYOK)                 │
                     │         /openai/v1/chat/completions → AI Gateway REST (multi-prov) │
                     │         DO HouseholdLedger (cuotas) · Analytics Engine (medición)  │
                     │ mcp-calendar: createMcpHandler + DO HouseholdVault (OAuth AES-GCM) │
                     │ mcp-transporte: createMcpHandler sobre D1 ◄─ Container Go (GTFS)   │
                     └───────────▲─────────────────────▲──────────────────────────────────┘
                                 │ WSS saliente (1/hogar) │ HTTPS + JWT (LLM, MCP)
 ┌──────────── HOGAR: HA OS (RPi5 / x86) ─────────────────┴───────────────────────────────┐
 │ App "domotica" → edge-core (Go, binario único, CGO_ENABLED=0)                          │
 │   ha (WS API) · entities · tools · agent · guardrails · identity · memory · scheduler  │
 │   llm (llmkit) · mcpclient · relay · convapi · web (ingress) · SQLite WAL (/data)      │
 │ HA Core: integraciones + Assist (wake word/STT/TTS)                                    │
 │   └ custom_components/domotica (ConversationEntity) → convapi                          │
 └────────────────────────────────────────────────────────────────────────────────────────┘
```

Todas las conexiones salen desde el hogar: el router del cliente no necesita abrir puertos.

---

## 4. Edge: `edge-core` (Go)

`edge-core` es un binario único compilado con `CGO_ENABLED=0` que corre como app de HA y tiene tres modos de ejecución:

| Modo | Cómo se configura |
|---|---|
| `dev` | Variables de entorno; conecta directo a un HA real o de pruebas. |
| `addon` | `/data/options.json` + `SUPERVISOR_TOKEN`. |
| `docker` | Contenedor plano para usuarios de HA Container (best-effort). |

| Paquete (`edge/internal/...`) | Responsabilidad |
|---|---|
| `config` | Carga y valida la configuración según el modo. |
| `store` | SQLite (`modernc.org/sqlite`), WAL, `busy_timeout`, migraciones embebidas ([ADR-0006](adr/0006-persistencia-edge-sqlite-modernc.md)). |
| `ha` | Cliente de la WebSocket API de HA ([ADR-0005](adr/0005-integracion-ha-via-websocket-api.md)): auth, correlación por id, `get_states`, `subscribe_events(state_changed)` + `coalesce_messages`, `call_service`, registros, `homeassistant/expose_entity/list`. Reconecta con resync. |
| `entities` | Contrato de entidad v1 (§7.1). Adaptadores por dominio de HA y asignación de `risk`. |
| `tools` | Genera las tools del LLM a partir de las capacidades presentes: **una por (categoría, acción)**, no una por entidad. Salida determinista para que el prompt cache sea estable. |
| `agent` | Loop LLM → guardrails → ejecución (máximo 8 pasos). Lleva el presupuesto de contexto y persiste la conversación. |
| `guardrails` | Capas L0–L5 (§8). |
| `identity` | Miembros del hogar, roles y resolución del principal por canal. |
| `memory` | Hechos, notas (FTS5), interfaz `Retriever` y job de extracción. |
| `scheduler` | Jobs, rrule, tzdata embebido (`America/Santiago`), recordatorios, brief matinal. |
| `llm` | Adaptador hacia `llmkit` (§6). En dev llama directo al proveedor; en prod va al `llm-gw`. |
| `mcpclient` | Cliente MCP (`github.com/modelcontextprotocol/go-sdk`) hacia los MCP de la nube, con un JWT por miembro. |
| `channels` | Interfaz `Transport` con implementaciones `telegrampoll` (dev), `relay` (prod) y `cli`. |
| `relay` | Cliente WSS hacia el DO del hogar: outbox, dedupe y keepalive con **frames de ping** (el DO los responde sin despertar). |
| `convapi` | `POST /v1/conversation`, que llama la integración de HA para voz y Assist. |
| `web` | UI de ingress (`go:embed`): pairing, miembros, vinculación de Telegram, políticas, memoria y auditoría. Solo acepta conexiones del peer `172.30.32.2`. |

### Interfaces clave

```go
// ha
type Client interface {
    States(ctx context.Context) ([]State, error)
    SubscribeStateChanged(ctx context.Context, fn func(StateChanged)) (cancel func(), err error)
    CallService(ctx context.Context, domain, service string, data map[string]any, t Target, withResp bool) (map[string]any, error)
    Registries(ctx context.Context) (Registries, error)
    Exposed(ctx context.Context, assistant string) (map[string]bool, error)
}
// channels
type Transport interface {
    Inbound() <-chan Inbound
    Send(ctx context.Context, m Outbound) error
    Capabilities() TransportCaps // Proactive, Buttons, Voice…
}
// guardrails
type Engine interface {
    Evaluate(ctx context.Context, p identity.Principal, call tools.Call, taint Taint) Decision // Allow|Deny|Confirm (+Clamped)
}
```

### Integración con HA (`ha-integration/`, Python)

Es una custom integration mínima que registra un `ConversationEntity`. Su `_async_handle_message` reenvía la petición a `convapi` y devuelve un `IntentResponse`. Gracias a eso, el pipeline Assist de HA (wake word, STT y TTS) usa nuestro agente. La opción "prefer local intents" sigue activa, de modo que los comandos simples se resuelven en HA sin pasar por el LLM.

### Funcionamiento sin internet

| Sigue funcionando | Deja de funcionar |
|---|---|
| Control local vía HA, automatizaciones, intents locales de Assist, recordatorios del scheduler (anunciados en satélites con `assist_satellite.announce`) | El asistente basado en LLM y el canal Telegram |

En la Fase 2 se cubre ese hueco con una ruta de fallback `type: openai` hacia un LLM local (Ollama en un hub con hardware suficiente).

---

## 5. Nube: Cloudflare (TypeScript sobre Workers)

| Componente | Servicio | Notas |
|---|---|---|
| Cuentas, hogares, dispositivos, pairing | Worker `api` + **D1** (Sessions API) + **KV** (lista de revocación) | JWT de dispositivo verificado con WebCrypto Ed25519. |
| Relay nube↔hogar | Worker `relay` + **DO `HouseholdRelay`** (SQLite, WebSocket Hibernation, alarmas) | **Siempre** `ctx.acceptWebSocket()`, nunca `ws.accept()`. Va en un Worker propio que casi no se despliega, porque cada despliegue reinicia los DO y corta todos los WS. Sabe al instante si el hogar está offline. |
| Gateway LLM | Worker `llm-gw` → **AI Gateway** | Las llaves de los proveedores viven en AIG (BYOK). Cada llamada lleva `cf-aig-metadata` con el hogar y `cf-aig-collect-log-payload: false` (no se guardan prompts). Cuotas en **DO `HouseholdLedger`**. Medición leyendo `usage` del SSE (en OpenAI, con `stream_options.include_usage`), eventos a **Analytics Engine** y un Cron que consolida en D1. Los *spend limits* de AIG quedan como red de seguridad. |
| Vault OAuth (Google/Microsoft) | **DO `HouseholdVault`** | Cifrado envolvente AES-GCM con una clave maestra guardada como secret del Worker. El DO serializa los refresh. |
| MCP calendario | Worker `mcp-calendar` con `createMcpHandler` (streamable HTTP, sin estado) | Bearer de dispositivo + miembro. **No** se usa `McpAgent` (Cloudflare desaconseja usarlo en servidores nuevos). |
| MCP transporte | Worker `mcp-transporte` sobre **D1**, alimentado por un Cron que revisa el ETag del GTFS de DTPM y un **Container** Go que hace la ingesta (GTFS → SQL → D1). Snapshot en **R2**. | El feed pesa 69,7 MB y `stop_times` tiene 1,1M de filas: no conviene cargarlo desde un Worker. |
| Telegram | Webhook en el Worker `api` → RPC al DO del hogar | Un solo bot del producto. Bots separados para dev y prod. |
| Imágenes de la app | **ghcr.io** (excepción a Cloudflare primero, ADR-0010) | El Supervisor de HA necesita pull anónimo y `serverless-registry` exige credenciales. Ambos cuestan USD 0. |
| Acceso remoto a la UI (Fase 2) | Wildcard `*.dominio` → Worker → DO → multiplexado sobre el WS existente | Sin video (ToS de Cloudflare). Reemplaza al túnel por hogar ([ADR-0004](adr/0004-cerebro-edge-nube-cloudflare-relay-durable-objects.md)). |
| Cobros (M11) | Mercado Pago o Flow (ADR-0012) | Cloudflare no ofrece una alternativa. |

### Por qué no un Cloudflare Tunnel por hogar

Cloudflare limita a **1.000 túneles por cuenta**. Además, cada hogar necesitaría un registro DNS, y el límite es de **200 registros por zona en el plan Free y 3.500 en Pro/Business**. Con un túnel por hogar, el producto no pasa de unos 1.000 clientes sin Enterprise. El relay con Durable Objects no depende de DNS por hogar y cuesta del orden de USD 0,003 al mes por hogar con Hibernation. Ver [ADR-0004](adr/0004-cerebro-edge-nube-cloudflare-relay-durable-objects.md).

### Costo estimado de la plataforma (sin tokens LLM)

| | 1.000 hogares | 10.000 hogares |
|---|---|---|
| Relay (DO con Hibernation) | ~USD 0–50/mes | ~USD 27–500/mes |
| Resto (Workers, logs, cuotas) | — | ~USD 60–100/mes |

El escenario de 10.000 hogares cuesta del orden de USD 0,003 al mes por hogar. **Sin Hibernation, el relay costaría más de USD 4.000/mes con 1.000 hogares.**

---

## 6. Capa de LLM: el proveedor es una commodity

Es un requisito obligatorio del producto. El sistema de agentes **debe** hablar tanto el formato de OpenAI como el de Claude, y cambiar de proveedor o de modelo tiene que ser solo configuración.

### `llmkit/` (módulo Go compartido)

**Formato neutro** (`contracts/schemas/llm.v1.json`):

- `system`
- `messages[]` con bloques `text | image | tool_call | tool_result`
- `tools[]` (nombre, descripción, JSON Schema)
- `tool_choice`, `max_tokens`, `temperature`, `stream`
- hints de cache
- `stop_reason` normalizado: `end | tool_calls | max_tokens | refusal | error`
- `usage` normalizado: `input`, `output`, `cache_read`, `cache_write`

**Adaptadores de wire.** Ambos hacen la traducción ida y vuelta, streaming SSE incluido (se convierte a eventos neutros):

| Aspecto | `type: openai` (Chat Completions) | `type: anthropic` (Messages) |
|---|---|---|
| System | mensaje `role:system` | campo `system` de nivel superior |
| Definición de tools | `tools[].function{name, parameters}` | `tools[]{name, input_schema}` |
| Llamadas a tools | `tool_calls[]` en el mensaje del assistant, con los argumentos como string JSON | bloques `tool_use` con `input` como objeto |
| Resultados de tools | mensajes `role:tool` con `tool_call_id` | bloques `tool_result` dentro del turno `user` |
| Cache | automático (los hints se ignoran) | `cache_control` |

**Cobertura.**
- `type: openai` cubre cualquier endpoint compatible cambiando solo `base_url`: OpenAI, Azure OpenAI, Gemini (endpoint OpenAI-compatible), OpenRouter, Groq, DeepSeek, Workers AI, y servidores locales como Ollama, vLLM o LM Studio.
- `type: anthropic` cubre Claude.

**Implementación.** Los adaptadores usan `github.com/openai/openai-go` y `github.com/anthropics/anthropic-sdk-go` con `base_url` configurable. Si algún endpoint "compatible" se desvía del estándar, se usa HTTP crudo con structs propios.

**Capacidades por modelo.** Se declaran en la config: `tools`, `parallel_tool_calls`, `strict_schema`, `prompt_cache`, `vision`, `max_context`. El agente se adapta; por ejemplo, serializa las llamadas si el modelo no soporta paralelas.

**Router.** Cada ruta (`chat | voice | brief | memory_extract`) tiene un proveedor primario y una cadena de fallback que se activa ante 5xx, timeout o 429. Las conversaciones se guardan en el formato neutro, así que pueden continuar en otro proveedor.

### Configuración (`llm.yaml`)

En producción, el Worker `api` la entrega al edge con `config.update`, por plan u hogar, desde D1.

```yaml
providers:
  anthropic:    {type: anthropic, base_url: https://api.anthropic.com, api_key_env: ANTHROPIC_API_KEY}   # dev
  openai:       {type: openai,    base_url: https://api.openai.com/v1, api_key_env: OPENAI_API_KEY}      # dev
  local:        {type: openai,    base_url: http://localhost:11434/v1}                                   # Ollama
  gw-anthropic: {type: anthropic, base_url: https://llm.<dominio>/anthropic, auth: device_jwt}           # prod
  gw-openai:    {type: openai,    base_url: https://llm.<dominio>/openai,    auth: device_jwt}           # prod
routes:
  chat:  {provider: gw-anthropic, model: <id>, fallback: [{provider: gw-openai, model: <id>}]}
  voice: {provider: gw-openai,    model: <id>}
```

En producción, `llm-gw` **no traduce formatos**: reenvía cada uno a su endpoint de AI Gateway. El formato Anthropic solo llega a modelos de Anthropic; el resto de los proveedores entra por el formato OpenAI vía la REST API de AIG (ids como `openai/…`, `google-ai-studio/…`, `workers-ai/…`). El modelo de cada ruta se elige con las evals de M2b.

---

## 7. Contratos

Todos los contratos son JSON Schema en `contracts/schemas/`. Para Go y TypeScript se generan o mantienen tipos equivalentes, y los fixtures en `contracts/fixtures/` se comparten entre Go, TS y Python.

### 7.1 Entidad normalizada v1 (`entity.v1.json`)

Evoluciona la sección 5 del documento original. Cambios respecto de esa versión:

- `id` lleva prefijo de origen, para poder reemplazar HA más adelante.
- `room` pasa a llamarse `area` y viene del registro de áreas de HA.
- Se agregan `risk` y `aliases`.
- `params` pasa a ser un fragmento de JSON Schema.

```json
{
  "id": "ha:light.living_lampara",
  "source": {"kind": "home_assistant", "entity_id": "light.living_lampara", "device_id": "…"},
  "name": "Lámpara living", "aliases": ["luz de pie"],
  "category": "light", "area": "living", "available": true, "risk": "low",
  "state": {"power": "on", "brightness_pct": 60, "color_temp_k": 3000},
  "capabilities": [
    {"action": "set_power", "params": {"state": {"enum": ["on", "off"]}}},
    {"action": "set_brightness", "params": {"brightness_pct": {"type": "integer", "minimum": 1, "maximum": 100}}}
  ]
}
```

**Riesgo.** Se clasifica como `low | medium | high | critical` según el dominio y el `device_class`: `lock`, `alarm_control_panel` y los cover de tipo `garage`, `gate` o `door` son `critical`.

**Tools.** Se genera una por cada par (categoría, acción) presente en el hogar, con `targets: string[]`. Ejemplos: `light_set_power`, `light_set_brightness`, `climate_set_temperature`, `cover_set_position`, `lock_set`, `media_player_control`, `scene_activate`, `script_run`, más `home_query(area?, category?, ids?)`. En total son entre 20 y 40 tools, sin importar cuántas entidades tenga el hogar.

### 7.2 Relay v1 (`relay.v1.json`)

- **Conexión**: `wss://relay.<dominio>/v1/edge`, subprotocolo `domotica.relay.v1`, `Authorization: Bearer <JWT del dispositivo>`.
- **Envelope**:

  ```json
  {"v":1,"id":"01JA…ULID","seq":4812,"type":"chat.inbound","ts":"2026-10-14T12:00:00Z","ack":true,"payload":{}}
  ```

- **Tipos de mensaje**:

  | Dirección | Tipos |
  |---|---|
  | edge → nube | `hello{device_id, addon_version, ha_version, features, last_seq}`, `chat.outbound{channel, member_id, text, buttons?, reply_to?}`, `chat.typing`, `notify.push{member_id, text, priority, dedupe_key, expires_at}`, `link.create{kind, member_id}`, `ack`, `nack` |
  | nube → edge | `welcome{session, plan, limits, features}`, `chat.inbound{msg_id, channel, member_id, text}`, `chat.callback{confirmation_id, decision, member_id}`, `link.created{url, expires_at}`, `member.linked`, `member.unlinked`, `config.update`, `error{code}` |

- **Semántica**: entrega *at-least-once* con IDs ULID y dedupe en el receptor. Del lado del edge hay un outbox en SQLite; del lado de la nube, un outbox en el DO. Al reconectar se usa `last_seq`.
- **Fuera del relay**: el LLM y los MCP **no** pasan por el WebSocket; van por HTTPS directo con el mismo JWT.

### 7.3 API de conversación (`conversation-api.v1.json`)

- **Request**: `POST /v1/conversation` con `{text, conversation_id, language, device_id, satellite_id, ha_user_id, extra_system_prompt}` y `Bearer integration_key`.
- **Response**: `{speech, conversation_id, continue_conversation, response_type}`.

---

## 8. Seguridad (resumen; detalle en [threat-model.md](threat-model.md))

**Identidad del dispositivo**

1. Al primer arranque se genera una llave Ed25519 en `/data`.
2. Pairing: el owner ingresa en el dashboard web el código que muestra el dispositivo.
3. El dispositivo recibe un JWT de 15 minutos con scopes `relay llm mcp`.
4. Revocación: se registra en KV y el WS se cierra con el código `4401`.

**Guardrails.** Cada llamada a una tool pasa por estas capas, en orden:

| Capa | Qué controla |
|---|---|
| L0, alcance | Solo entidades expuestas en HA (`expose_entity/list`) más la allowlist del producto. |
| L1, schema | Parámetros válidos según el JSON Schema de la capacidad; cada target debe existir y tener esa capacidad. |
| L2, política | Rangos por entidad (por ejemplo, clima entre 16 y 28 °C: se ajusta al rango o se rechaza explicando el motivo), anti-flapping, ventanas horarias y permisos por rol. |
| L3, riesgo | `low` se ejecuta. `medium` se ejecuta e informa. `high` pide confirmación al solicitante. `critical` exige confirmación por Telegram de un rol autorizado; por voz está desactivado salvo que el satélite tenga una política explícita, y hay un PIN opcional. |
| L4, taint | Si el turno ingirió contenido no confiable (un correo, una descripción de evento, una respuesta MCP o nombres de entidades), toda acción con efecto sube al menos a `high`. |
| L5, auditoría | `audit_log` registra quién, canal, tool, parámetros, decisión, motivo y resultado. |

**Roles**

| Rol | Permisos |
|---|---|
| `owner` | Todo. |
| `adult` | Todo, con las acciones `critical` confirmadas. |
| `child` | Solo sus áreas, sin acciones `high` ni `critical`. |
| `guest` | Acceso temporal. |

**Principal según canal**

| Canal | Cómo se identifica |
|---|---|
| Telegram | Por la vinculación (identidad fuerte). |
| Usuario de HA | Se mapea a un miembro. |
| Voz | Por la política del satélite (identidad débil). |
| Jobs del scheduler | Heredan los permisos de su creador. |

**Tokens OAuth.** Viven en `HouseholdVault` y **nunca bajan al edge**.

---

## 9. Flujos principales

**Telegram**

1. El webhook llega al Worker `api`, que valida el `secret_token`, deduplica por `update_id` y resuelve el hogar y el miembro.
2. `api` llama por RPC al DO `HouseholdRelay`. Si no hay WS abierto, responde de inmediato "tu hogar está desconectado".
3. El DO envía `chat.inbound` al edge.
4. El edge ejecuta `agent.Run`: llama al LLM vía `llm-gw` y a las tools (`home.*`, `memory.*` y MCP).
5. El edge responde con `chat.outbound` al DO, y `api` llama a `sendMessage`.

**Voz.** Assist (con "prefer local") → `conversation.domotica` → `POST /v1/conversation` → la respuesta vuelve a TTS.

**Proactivo.** El `scheduler` genera `notify.push` → outbox → relay → Telegram. Opcionalmente también se anuncia en un satélite con `announce`.

**Acción sensible**

1. El guardrail responde `Confirm`.
2. Se envían botones por Telegram, de un solo uso, válidos 60 segundos y atados a un hash de la acción.
3. Llega `chat.callback`.
4. El edge verifica miembro, hash y expiración, y recién entonces ejecuta.

Si el pedido llegó por voz, la confirmación se envía al Telegram del miembro.

**Vinculación de cuentas**

- Telegram: deep link `t.me/<bot>?start=<token>`. Se muestra como QR en la UI de ingress; el token es de un solo uso y vence en 15 minutos.
- Google y Microsoft: OAuth en la nube.

---

## 10. Hallazgos externos que condicionan el diseño (verificados el 2026-10-14)

**Home Assistant**
- Solo se soportan **HA OS** y **HA Container**; Core y Supervised están deprecados desde 2025.12. Las apps requieren Supervisor, es decir HA OS.
- En HA 2026.2, los add-ons pasaron a llamarse **"apps"**.
- Desde Supervisor 2026.04, las apps se construyen con `FROM ghcr.io/home-assistant/base` y BuildKit multi-arch. `build.yaml` ya no se usa.
- El **MCP Server oficial de HA** no entrega el stream de estados, los registros ni parámetros arbitrarios. Ver [ADR-0005](adr/0005-integracion-ha-via-websocket-api.md).
- HA ya trae agentes LLM nativos y un cliente MCP. Nuestra diferenciación es:
  - servicio gestionado, sin que el usuario maneje API keys;
  - multiusuario por Telegram;
  - memoria y proactividad;
  - calendario y correo;
  - foco en Chile.

**Google y Microsoft**
- Gmail usa scopes *restricted*, que exigen una auditoría **CASA** anual pagada.
- Calendar usa scopes *sensitive*: requiere verificación, pero no CASA.
- Por eso el orden es **Calendar en la Fase 1 y Gmail en la Fase 2**.
- Microsoft exige *publisher verification*.

**Transporte (Santiago)**
- DTPM publica un **GTFS estático oficial**.
- **No existe una API pública oficial de predicción de llegada de buses**. Hay que gestionar acceso con DTPM y revisar la licencia de uso comercial del GTFS.

**Marca**
- La licencia Apache-2.0 de HA no otorga derechos de marca.
- El logo de HA no puede usarse comercialmente.
- Antes de lanzar, hay que consultar a la Open Home Foundation si podemos decir "compatible con HA".

**Regulación**
- La **Ley 21.719** de datos personales entra en vigencia el 1 de diciembre de 2026, con posible postergación.

**Cloudflare**
- Túneles y DNS: ver §5.
- AI Gateway:
  - Su rate limit es por gateway, no por hogar.
  - BYOK está en beta (Secrets Store).
  - `/compat` está deprecado; para el formato OpenAI hacia cualquier proveedor se usa la REST API.
- El paquete `agents` sigue en versión 0.x; hay que fijar versiones exactas.

---

## 11. Qué se mantiene, qué cambia y qué se elimina respecto de la arquitectura original

| Elemento | Decisión | Motivo |
|---|---|---|
| `edge-core` en Go, binario único | Se mantiene | Ahora corre como app de HA. |
| SQLite WAL en Go puro | Se mantiene | `modernc.org/sqlite` directo ([ADR-0006](adr/0006-persistencia-edge-sqlite-modernc.md)). |
| Contrato de entidad → tools | Se mantiene y evoluciona | v1 con `risk`, `aliases`, JSON Schema y tools por capacidad. |
| Guardrails en Go | Se mantiene y se amplía | Roles, confirmaciones, taint y auditoría. |
| PWA embebida | Se mantiene | Servida vía ingress de HA. |
| AI Harness como servidor MCP | Cambia | El edge es **cliente** MCP. |
| SDKs de LLM en el dispositivo | Cambia | Los adaptadores van en `llmkit`; en producción, vía gateway. No hay llaves en el dispositivo. |
| Aprovisionamiento + túnel por hogar | Cambia | Pairing + relay con DO ([ADR-0004](adr/0004-cerebro-edge-nube-cloudflare-relay-durable-objects.md)). |
| `secrets.enc` | Cambia | Solo queda la llave del dispositivo; los tokens OAuth viven en la nube. |
| Worker Python + `uv` + pip en runtime | Se elimina | HA aporta los drivers, y la instalación desde PyPI en runtime era un riesgo de supply chain ([ADR-0003](adr/0003-construir-sobre-home-assistant.md)). |
| Escáner mDNS/SSDP + `device_catalog.json` | Se elimina | Lo hace el discovery de HA. |
| Mochi-MQTT embebido | Se elimina | El bus es la WebSocket API de HA. |
| Supervisor de procesos / systemd | Se elimina | Lo hace el Supervisor de HA. |
| OTA propio | Se elimina | Las actualizaciones llegan por la tienda de apps de HA. |

---

## 12. Layout del monorepo

```
Domótica/
├── AGENTS.md · arquitectura_teórica.md (histórico)
├── go.work                       # edge, contracts/go, llmkit, cloud/containers/gtfs-ingest
├── contracts/  schemas/ · go/ · ts/ · fixtures/
├── llmkit/     neutral · config · adapters/{openai,anthropic,fake} · router · conformance/
├── edge/       cmd/edge-core · internal/...
├── ha-addon/   repository.yaml · domotica/
├── ha-integration/ custom_components/domotica/ · tests/ · pyproject.toml (uv)
├── cloud/      workers/{api,relay,llm-gw,mcp-calendar,mcp-transporte}/ · migrations/ · containers/gtfs-ingest/
├── dev/        docker-compose.yml · ha-config/ · scripts/
├── docs/       arquitectura.md · threat-model.md · adr/ · plans/
└── .github/workflows/
```

Los módulos se crean recién en el milestone que los necesita, para no dejar scaffolding vacío.
