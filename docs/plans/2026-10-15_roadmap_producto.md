# Plan: Roadmap del producto (Fases 0–2, milestones M0–M11)

- **ID del Plan**: `2026-10-15_roadmap_producto`
- **Estado**: En Progreso
- **Fecha de Creación**: 2026-10-15
- **Última Actualización**: 2026-10-15
- **ADR Relacionado**:
  - [ADR-0003](../adr/0003-construir-sobre-home-assistant.md): construir sobre Home Assistant.
  - [ADR-0004](../adr/0004-cerebro-edge-nube-cloudflare-relay-durable-objects.md): cerebro en el edge, nube en Cloudflare, relay con Durable Objects.
  - [ADR-0005](../adr/0005-integracion-ha-via-websocket-api.md): integración con HA vía WebSocket API.
  - [ADR-0006](../adr/0006-persistencia-edge-sqlite-modernc.md): persistencia en SQLite (modernc).

---

## 1. Objetivo y Alcance

Construir el producto descrito en [docs/arquitectura.md](../arquitectura.md): un asistente personal con IA y cerebro domótico sobre Home Assistant, con el cerebro en el edge en Go y una nube delgada en Cloudflare con TypeScript.

El trabajo se organiza en milestones verticales. Cada uno debe ser demostrable de punta a punta y tiene **su propio plan** en `docs/plans/`, con los tests definidos antes de escribir código.

Este documento es el índice maestro: dice en qué milestone vamos y cuál sigue.

**Fuera de alcance de este roadmap**:
- La definición comercial: precio en CLP, planes y nombre de marca.
- El hardware del hub de la Fase 2.

Ambos temas se registran como preguntas abiertas.

---

## 2. Requisitos Previos y Contexto Técnico

- **Componentes**: `edge/`, `llmkit/`, `contracts/`, `ha-integration/`, `ha-addon/`, `cloud/` y `dev/`. El layout está en §12 de la arquitectura.
- **Contratos de datos**: entidad v1, relay v1, llm v1 y conversation-api v1, todos en `contracts/schemas/`.
- **Servicios de Cloudflare**: Workers, Durable Objects, D1, KV, AI Gateway, Analytics Engine, Containers y R2, a partir de M7.
- **Impacto comercial**: el OPEX estimado es de unos USD 0,003 por hogar al mes en el relay, sin contar tokens LLM. La privacidad se resuelve dejando la memoria en el edge.
- **Toolchain**: Go 1.27.0, gcc de MinGW-w64 (WinLibs 16.2) para `-race`, Docker 27, Python 3.13, uv 0.11 y Node 24.

### Prerrequisitos del usuario
- [ ] API keys de desarrollo para un proveedor de cada formato (OpenAI y Anthropic). Ollama sirve como alternativa local para el formato OpenAI. Se necesitan desde M2a.
- [ ] Un bot de Telegram de **desarrollo** creado con @BotFather. Desde M3.
- [ ] HA OS en casa para dogfooding. Desde M3 hasta M6.
- [ ] Una cuenta de Cloudflare en el plan Workers Paid (USD 5 al mes) y un dominio. Desde M7.

---

## 3. Estrategia de Pruebas Primero (TDD)

Cada milestone define en su plan los tests que se escriben en rojo antes de implementar. Estos son los tests clave de cada uno:

| Milestone | Tests clave |
|---|---|
| M0 | Config table-driven; store (WAL, migraciones idempotentes, rollback, concurrencia); cliente HA contra un fake WS (auth, correlación por id, caídas); `run --once`. |
| M1 | Golden tests por dominio (estado de HA → entidad v1), validación de schema, asignación de `risk`, reconexión con resync. |
| M2a | Suite de conformidad de `llmkit` común a ambos adaptadores (texto, tools simples y paralelas, streaming, usage, fallback). |
| M2b | Tools deterministas byte a byte, tabla de guardrails, agente con LLM `fake` y evals en es-CL. |
| M3 | Fake de la Bot API de Telegram, confirmaciones (expiración y uso único), roles, auditoría. |
| M4 | Reloj falso para recordatorios (incluye cambio de horario en `America/Santiago`), rrule, aislamiento de memoria privada, FTS5. |
| M5 | Contrato de conversation-api; pytest de la integración de HA. |
| M6 | Modo app (`options.json`, `SUPERVISOR_TOKEN`) y restricción del peer de ingress. |
| M7 | Vitest pool-workers (DO relay, JWT, outbox, extracción de usage desde SSE, cuotas); cliente relay en Go. |
| M8 | Webhook (`secret_token`), dedupe de `update_id`, enrutamiento a múltiples hogares, hogar offline. |
| M9 | Vault AES-GCM, refresh serializado, aislamiento por miembro, MCP en memoria. |
| M10 | Parser GTFS con fixture, SQL idempotente, tools MCP sobre D1 de prueba. |
| M11 | Borrado en cascada y exportación de datos. |

---

## 4. Fases de Ejecución y Tareas

### Fase 0: Dogfooding en casa (sin nube; LLM con API key de dev)
- [x] **M0**: Fundaciones (completado el 2026-10-15). Plan: [2026-10-15_m0_fundaciones.md](2026-10-15_m0_fundaciones.md). Incluye monorepo, config, store, cliente HA base, compose de HA demo y CI.
- [ ] **M1**: Cliente HA completo y entidades normalizadas v1. Plan: [2026-10-15_m1_entidades.md](2026-10-15_m1_entidades.md). Requiere [ADR-0007](../adr/0007-contrato-entidad-v1-tools-por-capacidad.md).
- [ ] **M2a**: `llmkit`, el proveedor de LLM como commodity con formatos OpenAI y Anthropic. Requiere ADR-0008.
- [ ] **M2b**: Tools, agente, guardrails L0–L3 y L5, CLI `edge-core chat` y evals. Requiere ADR-0009 (modelo de seguridad).
- [ ] **M3**: Identidad, Telegram por long polling, confirmaciones con botones y auditoría.
- [ ] **M4**: Memoria como grafo de conocimiento sobre SQLite (requiere ADR), scheduler, recordatorios, listas en `todo` de HA y brief matinal.
- [ ] **M5**: `convapi` e integración de HA (`ConversationEntity`) para voz.
- [ ] **M6**: App de HA multi-arch e instalación en HA OS. Requiere ADR-0010 (imágenes en ghcr.io). **Cierra la Fase 0.** Desde aquí se inicia la verificación OAuth de Google y la *publisher verification* de Microsoft.

### Fase 1: Beta pagada (10–20 hogares)
- [ ] **M7**: Nube base: Worker `api`, DO relay y `llm-gw` sobre AI Gateway, más los clientes del edge. Requiere ADR-0011 (identidad del dispositivo, relay v1, vault).
- [ ] **M8**: Bot de Telegram del producto: webhook, deep link y canal relay.
- [ ] **M9**: Google Calendar vía MCP (`mcp-calendar`, `HouseholdVault`) y `mcpclient` en el edge.
- [ ] **M10**: `transporte-cl`: ingesta GTFS en un Container Go y `mcp-transporte`.
- [ ] **M11**: Endurecimiento para la beta: onboarding, cobro (ADR-0012), observabilidad, Ley 21.719 y revisión del threat model.

### Fase 2: Escala y hub propio
- [ ] Hub con hardware propio y HA OS preinstalado.
- [ ] Canal WhatsApp con templates.
- [ ] Gmail (requiere CASA) y correo de Microsoft.
- [ ] Embeddings para la memoria.
- [ ] Acceso remoto a la UI multiplexado por el relay.
- [ ] Fallback a un LLM local cuando no hay internet (`type: openai` apuntando a Ollama).
- [ ] Voz con streaming; transcripción de notas de voz de Telegram.

---

## 5. Bitácora de Progreso y Memoria de Sesiones

- **[2026-10-15]**:
  - Se aprobó el diseño completo; está en [docs/arquitectura.md](../arquitectura.md) y en los ADRs 0003 a 0006. AGENTS.md se actualizó solo en sus partes descriptivas, sin tocar los mandatos.
  - Se instaló el toolchain: Go 1.24.1 → 1.27.0 y WinLibs gcc 16.2. Se verificó que `go test -race` funciona en Windows.
  - Se inicia M0.
- **[2026-10-15]**: M0 completado. Hay un cliente WS de HA, store SQLite y `edge-core run --once` validado contra HA 2026.10.1 real (123 entidades del demo). **Siguiente: M1**, que requiere escribir antes el ADR-0007 y el plan `docs/plans/YYYY-MM-DD_m1_entidades.md`.

### Preguntas abiertas (no bloquean M0–M3)

| # | Pregunta | Cuándo se resuelve |
|---|---|---|
| 1 | Economía del LLM: precio en CLP y qué modelo usar en cada ruta. | Con los datos de las evals de M2b. |
| 2 | ¿Pagarían los usuarios por esto frente a los agentes nativos de HA? | Validación en la Fase 0. |
| 3 | Política por defecto para acciones `critical` remotas: PIN, "solo LAN" u opt-in. | ADR-0009. |
| 4 | Distribución de la integración: ¿la copia la app a `/config` o se instala vía HACS? | M5/M6. |
| 5 | Nombre comercial sin la marca HA. | Consulta a la Open Home Foundation. |
| 6 | Acceso a la API de tiempo real de DTPM y licencia del GTFS. | Antes de M10. |
| 7 | Si el ID de módulo Go `domotica/...` debe pasar a la ruta del repositorio remoto. | Resuelta en M1: adoptado `github.com/Frey-r/Home-agent/edge`. |
