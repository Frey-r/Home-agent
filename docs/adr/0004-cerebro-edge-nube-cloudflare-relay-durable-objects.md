# ADR-0004: Cerebro en el edge, nube delgada en Cloudflare (TypeScript) y relay con Durable Objects

- **Estado**: Aceptado
- **Fecha**: 2026-10-15
- **Autores**: Equipo de Desarrollo / Agente de IA
- **Contexto del Proyecto**: Infraestructura Cloud, conectividad nube↔hogar, AI Harness
- **Relación**: **Reemplaza parcialmente al [ADR-0002](0002-estrategia-cloud-cloudflare-first-y-foco-comercial.md)** solo en un punto: la conectividad nube↔hogar ya no usa un Cloudflare Tunnel por hogar. La política Cloudflare-First y el análisis dual del ADR-0002 siguen vigentes.

---

## 1. Contexto y Planteamiento del Problema

El producto, además de domótica, es un **asistente personal**. Al cliente le hablamos por Telegram (más adelante también por WhatsApp), así que la nube tiene que poder entregarle mensajes al agente de cada hogar y llevar de vuelta las respuestas y los mensajes proactivos (recordatorios, brief matinal). También se necesitan:

- un gateway de LLM, para que las llaves de los proveedores no vivan en el dispositivo y para medir el consumo de cada hogar;
- OAuth de Google y Microsoft con los tokens guardados en la nube;
- servidores MCP para servicios como calendario y transporte.

Hay que decidir tres cosas:

1. **Dónde vive el cerebro**: el loop del agente, la memoria y los guardrails.
2. **Cómo se conecta la nube con el hogar.**
3. **En qué lenguaje se escribe la nube**, respetando Cloudflare-First.

Estos son los datos verificados que condicionan la decisión, al 2026-10-14:

- Cloudflare permite como máximo **1.000 túneles por cuenta**. Ampliarlo requiere Enterprise.
- Una zona DNS admite **200 registros en el plan Free** (zonas creadas desde 2024-09) y **3.500 en Pro/Business**. Cada hogar con un túnel necesita su propio hostname público.
- Los Durable Objects (DO) con **WebSocket Hibernation** no cobran duración mientras están hibernados. Los mensajes WS entrantes se facturan en proporción 20:1 y los pings de protocolo los responde el runtime sin despertar al objeto.
- Las herramientas maduras para Workers (DO, Hibernation, `createMcpHandler`, `workers-oauth-provider`, AI binding) existen primero en TypeScript. Ejecutar Go en Workers vía Wasm (`syumai/workers-go`) se declara **experimental**, y no está confirmado que soporte DO ni Hibernation.

## 2. Opciones Consideradas

### Opción 1: Cerebro en la nube (cloud-first)
- **Descripción**: El agente, la memoria y los guardrails corren en la nube. El dispositivo solo ejecuta acciones.
- **Ventajas (+)**: Iteración rápida. El asistente sigue respondiendo aunque el hogar esté offline.
- **Desventajas (-)**:
  - La memoria y las conversaciones del hogar quedan en la nube, lo que afecta la privacidad.
  - Hay más cómputo y más almacenamiento por hogar, así que sube el OPEX.
  - Contradice el principio Edge-First de AGENTS.md.

### Opción 2: Cerebro en el edge y un Cloudflare Tunnel por hogar (lo que planteaba el ADR-0002)
- **Descripción**: Cada hogar corre `cloudflared` con un túnel propio y un hostname público. La nube le habla al edge por HTTP a través del túnel.
- **Ventajas (+)**: No hace falta un protocolo de relay propio. Da acceso remoto a la UI sin trabajo extra.
- **Desventajas (-)**:
  - **No escala comercialmente**: el límite de 1.000 túneles por cuenta y de 200 a 3.500 registros DNS por zona obliga a pasar a Enterprise o a repartir los hogares entre varias cuentas.
  - Agrega un proceso extra en el dispositivo.
  - Hay que proteger cada hostname.

### Opción 3: Cerebro en el edge, nube delgada en Cloudflare y relay con Durable Objects (Elegida)
- **Descripción**: El edge abre **un WebSocket saliente** hacia un DO `HouseholdRelay` (uno por hogar) que usa Hibernation y guarda un outbox en SQLite. La nube consiste en Workers escritos en **TypeScript**:
  - `api`: cuentas, pairing y Telegram, sobre D1 y KV.
  - `relay`: un Worker propio que casi no se despliega.
  - `llm-gw`: delante de AI Gateway, con DO `HouseholdLedger` y Analytics Engine.
  - `mcp-calendar`: con el DO `HouseholdVault`.
  - `mcp-transporte`: sobre D1, alimentado por un Container Go que ingiere el GTFS.
- **Ventajas (+)**:
  - Escala sin depender de DNS por hogar.
  - Costo cercano a cero por hogar.
  - La nube sabe al instante si un hogar está offline.
  - Sigue siendo 100 % Cloudflare.
- **Desventajas (-)**:
  - Hay que diseñar y mantener un protocolo de relay propio, con dedupe y outbox.
  - Cada despliegue del Worker reinicia los DO y corta todos los WebSockets, así que el Worker de relay debe desplegarse lo menos posible.
  - El acceso remoto a la UI no viene incluido; se construye en la Fase 2 multiplexándolo sobre el WS existente.

### Subdecisión: lenguaje de la nube
- **TypeScript (elegido)**: primera clase para DO, Hibernation, `createMcpHandler` y AI binding. Los tests corren dentro de workerd con `@cloudflare/vitest-pool-workers`.
- **Go vía Wasm**: experimental, sin soporte confirmado para DO ni Hibernation.
- **Python Workers**: GA desde 2026-09, pero sin un ecosistema MCP, OAuth y DO maduro.

Go se mantiene para el edge y para el Container que ingiere el GTFS. Los contratos entre edge y nube se definen en JSON Schema (`contracts/`).

## 3. Decisión Tomada

Se adopta la **Opción 3** con la nube en **TypeScript**:

- El cerebro vive en el hogar.
- La nube solo hace de relay, gateway de LLM, OAuth expuesto como MCP, cuentas y medición.
- **La nube nunca ejecuta acciones sobre dispositivos.**
- La conectividad nube↔hogar pasa por DO con WebSocket Hibernation. Hay que usar siempre `ctx.acceptWebSocket()`, nunca `ws.accept()`.
- Los Cloudflare Tunnels quedan reservados para casos puntuales. No se usan como mecanismo por hogar.

## 4. Análisis de Trade-Offs (Foco Comercial)

### 4.1. Impacto para Nosotros como Proveedores
- **Costos Operativos (OPEX)**: Estimación con la lista de precios publicada por Cloudflare, sin contar tokens LLM:

  | Componente | 1.000 hogares | 10.000 hogares |
  |---|---|---|
  | Relay | ~USD 0–50/mes | ~USD 27–500/mes (en el escenario realista, ~USD 0,003 por hogar) |
  | Resto de la plataforma | — | ~USD 60–100/mes |
  | Relay **sin Hibernation** (referencia) | más de USD 4.000/mes | — |

  Usar Hibernation de forma correcta es obligatorio.
- **Mantenibilidad y Soporte**:
  - El protocolo de relay propio implica más código, pero queda acotado por los contratos en `contracts/schemas/relay.v1.json` y por tests de caos (cortes, duplicados, reordenamiento).
  - Separar el Worker `relay` de los demás evita cortar conexiones con cada despliegue.
- **Velocidad de Entrega**:
  - TypeScript en Workers es el camino documentado.
  - Que las interfaces del edge sean intercambiables permite desarrollar la Fase 0 sin nube: Telegram por long polling y la API key directa.

### 4.2. Impacto para el Cliente Final
- **Experiencia de Usuario (UX) y Latencia**:
  - Los mensajes van por un WS que ya está abierto, sin handshake por cada mensaje.
  - El usuario se entera al instante si su hogar está offline.
- **Privacidad y Seguridad**:
  - La memoria, las conversaciones y el estado del hogar quedan **en el edge**.
  - Por la nube solo pasan mensajes en tránsito, y AI Gateway se configura con `cf-aig-collect-log-payload: false` para no guardar prompts.
  - Los tokens OAuth viven cifrados en la nube y **nunca bajan al edge**.
- **Continuidad Operativa**:
  - Sin internet, HA sigue controlando el hogar localmente: automatizaciones, intents locales de Assist y los recordatorios que anuncia el scheduler del edge.
  - Lo que se degrada es el asistente LLM y Telegram.
  - En la Fase 2 está previsto un fallback a un LLM local mediante `llmkit` con `type: openai` apuntando a Ollama.
- **Costo total y consumo**: No requiere ningún proceso extra en el dispositivo (se elimina `cloudflared`).

### 4.3. Evaluación Cloudflare-First (Si aplica infraestructura Cloud)
- **Servicios Cloudflare seleccionados**:
  - Workers y Durable Objects (con SQLite, Hibernation y alarmas).
  - D1, con Sessions API.
  - KV.
  - AI Gateway, con BYOK.
  - Analytics Engine.
  - Containers, para la ingesta del GTFS.
  - R2.
  - Cron Triggers y el Rate Limiting binding.
- **Justificación Costo / Calidad**:
  - Los DO con Hibernation dan conexiones persistentes de costo casi nulo cuando están inactivas, sin límites de DNS ni de túneles.
  - Este ADR no introduce ningún proveedor fuera de Cloudflare. Las excepciones previstas son las imágenes en ghcr.io y la pasarela de pago, y cada una tendrá su propio ADR (0010 y 0012).

## 5. Consecuencias y Efectos Secundarios

### Positivas (+)
- La arquitectura escala a decenas de miles de hogares sin pasar a Enterprise.
- El modelo de privacidad es claro: la memoria vive en la casa.
- El OPEX es predecible y muy bajo por hogar.

### Negativas / Riesgos Mitigados (-)
- **Si el hogar está offline, el asistente está offline.** Se acepta conscientemente y se mitiga con el control local de HA y, en la Fase 2, con el fallback a un LLM local.
- **Despliegues del relay.** Se mitiga con un Worker separado, reconexión con backoff y jitter, y outboxes en ambos extremos.
- **Una nube comprometida podría inyectar mensajes de chat.** Se mitiga con guardrails, PIN para acciones `critical` y la opción "solo en LAN". Ver [threat-model.md](../threat-model.md), amenaza T6.
- **El paquete `agents` sigue en 0.x y cambia rápido.** Se fijan versiones exactas.
- **BYOK de AI Gateway está en beta.** El plan B es guardar las llaves como secrets del Worker.
- **Este ADR cambia el Mandato III de AGENTS.md**: el ítem de túneles pasa a "Conectividad nube↔hogar: relay con Durable Objects".

## 6. Referencias y Documentación Relacionada
- [docs/arquitectura.md](../arquitectura.md) §5 y §7.2
- [Límites de Zero Trust (túneles)](https://developers.cloudflare.com/cloudflare-one/account-limits/)
- [Registros DNS por zona](https://developers.cloudflare.com/dns/manage-dns-records/)
- [Precios de Durable Objects](https://developers.cloudflare.com/durable-objects/platform/pricing/)
- [WebSockets en DO (Hibernation)](https://developers.cloudflare.com/durable-objects/best-practices/websockets/)
- [AI Gateway: BYOK](https://developers.cloudflare.com/ai-gateway/configuration/bring-your-own-keys/)
- [AI Gateway: custom metadata](https://developers.cloudflare.com/ai-gateway/observability/custom-metadata/)
- [Transporte MCP en Workers](https://developers.cloudflare.com/agents/model-context-protocol/transport/)
- [Lenguajes soportados por Workers](https://developers.cloudflare.com/workers/languages/)
- [syumai/workers-go](https://github.com/syumai/workers-go)
