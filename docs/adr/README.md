# Registro de Decisiones de Arquitectura (ADRs)

Este directorio alberga los registros formales de todas las decisiones arquitectónicas y de diseño tomadas a lo largo de la evolución del proyecto.

## Criterio Comercial y de Infraestructura Cloud
En este proyecto:
1. **Foco Comercial**: Toda decisión debe explicitar los trade-offs para nosotros como proveedores (OPEX, soporte, escalabilidad) y para el cliente final (UX, latencia, privacidad, funcionamiento offline).
2. **Cloudflare-First**: Cualquier servicio cloud necesario (buckets, colas, cómputo edge, DNS, túneles) prioriza la solución de Cloudflare (R2, Queues, Workers, D1/KV, Cloudflare Tunnels) por su óptima relación costo/calidad.

---

## Índice de ADRs

| Número | Título | Estado | Fecha | Contexto |
| :--- | :--- | :--- | :--- | :--- |
| [ADR-0001](0001-adopcion-tdd-adrs-planes.md) | Adopción Obligatoria de TDD, ADRs y Planes Persistentes como Memoria | Aceptado | 2026-10-14 | Gobernanza y Calidad |
| [ADR-0002](0002-estrategia-cloud-cloudflare-first-y-foco-comercial.md) | Estrategia Cloudflare-First y Análisis Dual de Trade-Offs Comerciales | Aceptado (parcialmente reemplazado por ADR-0004 en conectividad nube↔hogar) | 2026-10-14 | Estrategia Cloud y Negocio |
| [ADR-0003](0003-construir-sobre-home-assistant.md) | Construir sobre Home Assistant como capa IoT y distribuir como app de HA | Aceptado | 2026-10-15 | Arquitectura general / Capa IoT |
| [ADR-0004](0004-cerebro-edge-nube-cloudflare-relay-durable-objects.md) | Cerebro en el edge, nube delgada en Cloudflare (TypeScript) y relay con Durable Objects | Aceptado | 2026-10-15 | Infraestructura Cloud / Conectividad |
| [ADR-0005](0005-integracion-ha-via-websocket-api.md) | Integración edge↔Home Assistant vía WebSocket API (no vía MCP Server de HA) | Aceptado | 2026-10-15 | Protocolo edge↔HA |
| [ADR-0006](0006-persistencia-edge-sqlite-modernc.md) | Persistencia del edge con SQLite (`modernc.org/sqlite`) y migraciones embebidas | Aceptado | 2026-10-15 | Base de datos local |
| [ADR-0007](0007-contrato-entidad-v1-tools-por-capacidad.md) | Contrato de Entidad Normalizada v1 y Tools por Capacidad | Aceptado | 2026-10-15 | Contratos, Entidades y Cliente HA |
| [ADR-0008](0008-llmkit-formato-neutro-adaptadores-openai-anthropic.md) | Abstracción de LLM Commodity con llmkit, Formato Neutro y Adaptadores OpenAI/Anthropic | Aceptado | 2026-10-15 | Capa de LLM, Formatos Wire y Router |

---

## Cómo Crear un Nuevo ADR
1. Copiar la plantilla [docs/adr/template.md](template.md).
2. Asignar el siguiente número secuencial (ejemplo: `0003-eleccion-broker-mqtt.md`).
3. Completar todas las secciones obligatorias, incluyendo el análisis de trade-offs duales (proveedor vs cliente) y la evaluación Cloudflare-First.
4. Añadir una fila a la tabla de este archivo `README.md`.
