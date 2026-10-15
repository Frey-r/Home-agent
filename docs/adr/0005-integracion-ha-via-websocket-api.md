# ADR-0005: Integración edge↔Home Assistant vía WebSocket API (no vía el MCP Server de HA)

- **Estado**: Aceptado
- **Fecha**: 2026-10-15
- **Autores**: Equipo de Desarrollo / Agente de IA
- **Contexto del Proyecto**: `edge-core` en Go, paquete `ha`, protocolo edge↔HA

---

## 1. Contexto y Planteamiento del Problema

El [ADR-0003](0003-construir-sobre-home-assistant.md) define que HA es la capa IoT. `edge-core` necesita cinco cosas de HA:

1. El estado de todas las entidades.
2. Un **stream de cambios de estado**, que es lo que hace posible la proactividad.
3. Los registros de áreas, dispositivos y entidades, para construir el directorio del hogar.
4. La lista de entidades expuestas al asistente.
5. Ejecutar servicios con control total de los parámetros.

HA ofrece tres vías para esto:

- la REST API;
- la WebSocket API;
- desde 2025.2, un **MCP Server oficial** (`/api/mcp`, streamable HTTP).

## 2. Opciones Consideradas

### Opción 1: MCP Server oficial de HA
- **Descripción**: El agente consume las tools que expone HA (intents de Assist y `GetLiveContext`).
- **Ventajas (+)**: Es estándar MCP, no hay que mantener un cliente propio y se reutilizan los intents de HA.
- **Desventajas (-)**:
  - **No soporta notificaciones**, así que no hay stream de estados ni proactividad.
  - No expone los registros ni permite parámetros arbitrarios de servicio.
  - Nos ata a la semántica de los intents de Assist.
  - Requiere por defecto una cuenta admin.

### Opción 2: REST API con polling
- **Descripción**: `GET /api/states` periódico y `POST /api/services/...`.
- **Ventajas (+)**: Simple y documentada.
- **Desventajas (-)**:
  - El polling da latencia y carga innecesaria.
  - No da acceso a los registros.
  - No hay eventos en tiempo real.

### Opción 3: WebSocket API (Elegida)
- **Descripción**: Cliente WS propio sobre `github.com/coder/websocket`, con dos tipos de comandos.
  - **Documentados**:
    - `auth`
    - `supported_features` (con `coalesce_messages`)
    - `get_states`
    - `subscribe_events` (con `state_changed`)
    - `call_service` (con `return_response`)
    - `get_services`
    - `get_config`
    - `homeassistant/expose_entity/list`
  - **No documentados** (los usa el propio frontend de HA): los `config/{area,device,entity}_registry/list`. Quedan aislados detrás de un adaptador.
- **Ventajas (+)**:
  - Eventos en tiempo real.
  - Acceso completo a registros y servicios.
  - Una sola conexión persistente.
  - En modo app, el mismo endpoint está disponible en `ws://supervisor/core/websocket` con `SUPERVISOR_TOKEN`.
- **Desventajas (-)**:
  - Hay que mantener un cliente propio: correlación por id y reconexión con resync.
  - Los comandos de registros no documentados pueden cambiar entre versiones.

## 3. Decisión Tomada

Se adopta la **Opción 3**:

- El paquete `edge/internal/ha` expone la interfaz `ha.Client`: `States`, `SubscribeStateChanged`, `CallService`, `Registries` y `Exposed`.
- Al reconectar, siempre hace un **resync completo** (`get_states` más la resuscripción).
- Las entidades fuera de la lista de expuestas no llegan al agente: es la capa L0 de los guardrails.
- El MCP Server de HA queda como compatibilidad opcional a futuro. No es el transporte principal.

## 4. Análisis de Trade-Offs (Foco Comercial)

### 4.1. Impacto para Nosotros como Proveedores
- **Costos Operativos (OPEX)**: Cero. Es comunicación local entre la app y HA Core.
- **Mantenibilidad y Soporte**:
  - Hay que mantener un cliente WS propio.
  - El riesgo de que cambie la API se mitiga con:
    - tests contra un fake WS para la lógica de protocolo;
    - integration tests contra un HA real fijado a una versión en `dev/docker-compose.yml`;
    - el workflow `ha-matrix`, que corre contra tres versiones de HA (stable, la anterior y beta).
- **Velocidad de Entrega**: El cliente base (auth y `get_states`) es pequeño y entra en M0. El resto (suscripciones, registros, reconexión) va en M1.

### 4.2. Impacto para el Cliente Final
- **Experiencia de Usuario (UX) y Latencia**:
  - Los cambios de estado llegan en menos de 1 segundo, sin polling.
  - Habilita respuestas proactivas, como "se quedó abierta la puerta del garage".
- **Privacidad y Seguridad**:
  - El tráfico es 100 % local.
  - Solo se usan las entidades que el usuario expuso explícitamente al asistente.
- **Continuidad Operativa**: Funciona sin internet. Si HA se reinicia, el cliente reconecta con backoff y hace resync.

### 4.3. Evaluación Cloudflare-First (Si aplica infraestructura Cloud)
- **Servicio Cloudflare Seleccionado**: No aplica (comunicación local).
- **Justificación Costo / Calidad**: No hay costo cloud.

## 5. Consecuencias y Efectos Secundarios

### Positivas (+)
- Hay base para la proactividad (watchers de eventos) y para un directorio del hogar completo.
- La interfaz `ha.Client` desacopla al agente de HA, en coherencia con el prefijo `ha:` de las entidades.

### Negativas / Riesgos Mitigados (-)
- **Comandos de registros no documentados.** Se aíslan en un adaptador y se prueban contra la matriz de versiones de HA.
- **Mensajes grandes en hogares con muchas entidades.** Se mitiga con `coalesce_messages` y aumentando el límite de lectura del WS.

## 6. Referencias y Documentación Relacionada
- [WebSocket API de HA](https://developers.home-assistant.io/docs/api/websocket/)
- [Integración MCP Server de HA](https://www.home-assistant.io/integrations/mcp_server/)
- [Comunicación de apps con HA](https://developers.home-assistant.io/docs/apps/communication/)
- [docs/arquitectura.md](../arquitectura.md) §4
