# Modelo de amenazas

- **Última actualización**: 2026-10-15
- **Alcance**: edge (`edge-core` + integración de HA), nube (Workers/DO de Cloudflare) y canales (Telegram, voz).
- **Relacionado**: [arquitectura.md §8](arquitectura.md#8-seguridad-resumen-detalle-en-threat-modelmd). Las decisiones de seguridad requieren un ADR según el Mandato II de [AGENTS.md](../AGENTS.md).

Este documento está vivo. Se revisa al cerrar cada milestone que agregue superficie: M3 (Telegram), M5 (voz), M7 (nube), M9 (OAuth) y M11 (beta).

---

## 1. Activos

| Activo | Dónde vive | Impacto si se compromete |
|---|---|---|
| Control físico del hogar (cerraduras, alarma, portón, garage) | HA, ejecutado por el edge | **Crítico**: seguridad física de personas y bienes |
| Llave privada del dispositivo (Ed25519) | `/data/device_key` (0600) | Suplantación del hogar ante la nube |
| Memoria y conversaciones del hogar | SQLite del edge | Privacidad: rutinas, preferencias, datos de personas |
| Tokens OAuth de Google y Microsoft | DO `HouseholdVault` (AES-GCM) | Acceso al calendario y al correo del usuario |
| Llaves de los proveedores LLM | AI Gateway (BYOK) | Costo descontrolado (OPEX) |
| Vínculo Telegram ↔ miembro | D1 | Suplantación de un miembro por chat |
| Token de la integración de HA → `convapi` | Config de HA y del edge | Inyección de comandos por voz o Assist |

## 2. Fronteras de confianza

1. **LAN del hogar ↔ edge.** Ingress solo acepta el peer `172.30.32.2`. `convapi` escucha únicamente en la red interna de hassio y exige una clave.
2. **Edge ↔ HA Core.** Se usa `SUPERVISOR_TOKEN`. HA es confiable, pero los **nombres de entidades** son datos ingresados por usuarios y se tratan como no confiables (taint).
3. **Edge ↔ nube.** WSS y HTTPS salientes, autenticados con un JWT de corta duración que el dispositivo obtiene firmando con su llave Ed25519.
4. **Nube ↔ terceros.** Telegram (webhook con `secret_token`), Google/Microsoft (OAuth) y proveedores LLM (vía AI Gateway).
5. **LLM ↔ agente.** **La salida del LLM nunca es confiable.** Toda tool call pasa por guardrails L0–L5.

## 3. Amenazas y mitigaciones

| # | Amenaza | Mitigación | Riesgo residual |
|---|---|---|---|
| T1 | El LLM alucina o malinterpreta y ejecuta una acción peligrosa | L1 (schema y existencia de targets), L2 (rangos, anti-flapping), L3 (confirmación para `high`/`critical`) | Bajo |
| T2 | **Prompt injection** vía contenido externo (correo, evento de calendario, respuesta MCP, nombre de entidad) que ordena "abre la puerta" | L4 (taint): si el turno ingirió contenido no confiable, toda acción con efecto sube al menos a `high` y exige confirmación. Las tools de contenido externo se marcan `untrusted:true`. | Medio. Hay que revisarlo cuando llegue Gmail (Fase 2). |
| T3 | Audio suplantado (TV, grabación, visita) dice "desarma la alarma" | `critical` está deshabilitado por voz por defecto. La confirmación va por Telegram. PIN opcional por miembro (su hash solo vive en el edge). | Bajo |
| T4 | Alguien escribe al bot desde un Telegram no vinculado | La nube rechaza al remitente sin reenviarlo. El vínculo se crea por deep link de un solo uso con TTL de 15 minutos, generado desde la UI local. | Bajo |
| T5 | Robo o pérdida del teléfono de un miembro con Telegram abierto | Revocar al miembro desde la UI. Para `critical`, PIN opcional y opción "solo en LAN". | Medio |
| T6 | **Nube comprometida** (Workers o DO) inyecta `chat.inbound` falsos | La nube no ejecuta acciones; el edge aplica guardrails y roles. Para `critical`, el PIN y la opción "solo en LAN" cortan la cadena. | Medio. Es una **limitación conocida y documentada**. |
| T7 | Llave del dispositivo filtrada (por ejemplo, desde un backup) | JWT de 15 minutos, revocación vía KV con cierre del WS con código 4401, re-pairing. | Bajo |
| T8 | Replay de callbacks de confirmación | Confirmaciones de un solo uso, ligadas a `action_hash`, al miembro y a una expiración de 60 segundos. | Bajo |
| T9 | Duplicados o reordenamiento en el relay | IDs ULID, dedupe en el receptor (`processed_inbound`), `seq`/`last_seq`. | Bajo |
| T10 | Atacante en la LAN intenta usar ingress o `convapi` | Validación del peer `172.30.32.2` (no confiar solo en los headers `X-Remote-User-*`). `convapi` con clave y escuchando solo en la red interna. | Bajo |
| T11 | Abuso de costo LLM (bucle del agente, cliente malicioso) | Máximo 8 pasos por turno. Cuota por hogar en DO `HouseholdLedger`. Rate Limiting binding. *Spend limits* de AIG como red de seguridad. | Bajo |
| T12 | Filtración de tokens OAuth | Los tokens nunca bajan al edge. Cifrado envolvente AES-GCM con la clave maestra en un secret del Worker. Scopes mínimos. | Bajo |
| T13 | Fuga de privacidad entre miembros del mismo hogar | Hechos con `visibility: private` que nunca entran al contexto de otro miembro (test obligatorio en M4). Calendario aislado por miembro (test en M9). | Bajo |
| T14 | Fuga de prompts del hogar hacia logs del proveedor de nube | `cf-aig-collect-log-payload: false`: solo se guarda metadata (tokens, costo, modelo). | Bajo |
| T15 | Supply chain (dependencias o imagen de la app) | Sin instalación de paquetes en runtime (se eliminó pip en caliente). Versiones fijadas. Imágenes firmadas con cosign y SBOM (M6/M11). | Medio |
| T16 | El agente opera entidades que el usuario no expuso | L0: solo entidades expuestas a Assist en HA, más la allowlist del producto. | Bajo |

## 4. Supuestos

- HA Core y su Supervisor son confiables. Si están comprometidos, el hogar ya está comprometido.
- El usuario `owner` controla el acceso físico al equipo que corre HA OS.
- Cloudflare, Telegram y los proveedores LLM operan según sus términos de servicio.

## 5. Pendientes por revisar

- [ ] Política por defecto para acciones `critical` remotas: ¿se habilitan por defecto o requieren opt-in? Decidir en el ADR-0009 (M2b).
- [ ] Mitigación adicional para T6, por ejemplo firmar la intención del usuario de extremo a extremo para `critical`. Evaluar en el ADR-0011 (M7).
- [ ] Revisión legal de la Ley 21.719: base de licitud, transferencias al proveedor LLM, retención. Se hace en M11.
