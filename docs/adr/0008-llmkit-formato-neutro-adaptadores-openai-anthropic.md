# ADR-0008: Abstracción de LLM Commodity con llmkit, Formato Neutro y Adaptadores OpenAI/Anthropic

- **Estado**: Aceptado
- **Fecha**: 2026-10-15
- **Autores**: Equipo de Desarrollo / Agente de IA
- **Contexto del Proyecto**: AI Harness (`edge-core`), módulo compartido `llmkit/`, contratos `contracts/schemas/llm.v1.json`, gateway cloud (`llm-gw` sobre Cloudflare AI Gateway).

---

## 1. Contexto y Planteamiento del Problema

Uno de los pilares arquitectónicos y comerciales del producto (§6 de `docs/arquitectura.md`) establece que:
> **"El proveedor de LLM debe ser tratado como una commodity intercambiable."**

El orquestador en el edge (`edge-core`) no debe acoplarse a los SDKs o formatos propietarios de ningún proveedor específico (OpenAI, Anthropic, Google, NVIDIA, etc.). Cambiar de proveedor o de modelo para cualquier ruta funcional (`chat`, `voice`, `brief`, `memory_extract`) debe ser una mera operación de configuración en `llm.yaml`, sin requerir modificaciones en el bucle del agente, las definiciones de herramientas domóticas, la memoria ni los guardrails de seguridad.

Asimismo, ante la volatilidad de disponibilidad y latencia de los proveedores en la nube (errores 5xx, límites de tasa 429, degradaciones de servicio), el sistema debe ofrecer conmutación transparente por fallback (por ejemplo, conmutar de Claude a un modelo compatible con OpenAI o local) a mitad de una interacción, lo cual exige que el historial conversacional y las invocaciones de herramientas residan en una estructura neutra e independiente del protocolo wire.

Por último, para pruebas de desarrollo y entornos con presupuestos acotados, el sistema debe admitir proveedores compatibles con OpenAI de consumo gratuito o bajo costo (tales como NVIDIA NIM / Nemotron en la nube, OpenRouter, Groq o instancias locales de Ollama / vLLM), así como un simulador hermético de pruebas offline (`llmtest.Server`).

---

## 2. Decisiones de Diseño Tomadas

### 1. Módulo Independiente `llmkit/`
Se crea un módulo Go dedicado (`llmkit/`) integrado en el espacio de trabajo `go.work`. Este módulo expone una interfaz desacoplada para la ejecución de completaciones y streams conversacionales.

### 2. Formato Neutro Canónico (`contracts/schemas/llm.v1.json`)
Todas las interacciones del agente se modelan sobre tipos neutros serializables:
- **`Message`**: turnos de conversación con roles (`system`, `user`, `assistant`).
- **Bloques de Contenido polimórficos**:
  - `text`: contenido textual.
  - `image`: datos binarios/URL para visión.
  - `tool_call`: llamada generada por el modelo (`id`, `name`, `arguments` validados como JSON).
  - `tool_result`: resultado de la ejecución de una herramienta (`tool_call_id`, `content`, `is_error`).
- **Definición de `Tool`**: nombre, descripción y `parameters` definidos como JSON Schema estricto.
- **`Options`**: control de hiperparámetros (`temperature`, `max_tokens`, `tool_choice`, streaming).
- **`StopReason` normalizado**: enum canónico:
  - `end`: finalización natural de la respuesta.
  - `tool_calls`: el modelo solicita ejecutar una o más herramientas.
  - `max_tokens`: se alcanzó el límite máximo de tokens.
  - `refusal`: rechazo por filtros de seguridad del modelo.
  - `error`: fallo de inferencia o red.
- **`Usage` normalizado**: contabilidad exhaustiva de tokens:
  - `input_tokens`: tokens de entrada.
  - `output_tokens`: tokens generados.
  - `cache_read_tokens`: tokens leídos desde el prompt cache.
  - `cache_write_tokens`: tokens escritos en el prompt cache.

### 3. Adaptadores de Wire Bidireccionales
El sistema soporta dos adaptadores principales que traducen de ida y vuelta:

| Aspecto | `type: openai` (Chat Completions) | `type: anthropic` (Messages) |
|---|---|---|
| **Endpoint base** | `/v1/chat/completions` (configurable con `base_url`) | `/v1/messages` (configurable con `base_url`) |
| **Instrucciones de sistema** | Mensaje con `role: "system"` en el array de mensajes | Campo `system` estructurado de primer nivel |
| **Definición de herramientas** | `tools[].function { name, description, parameters }` | `tools[] { name, description, input_schema }` |
| **Llamadas a herramientas** | Array `tool_calls[]` en mensaje del asistente, con `arguments` como string JSON | Bloques `tool_use` con `input` como objeto JSON nativo |
| **Resultados de herramientas** | Mensajes individuales con `role: "tool"` y `tool_call_id` | Bloques `tool_result` dentro de un turno con `role: "user"` |
| **Prompt Caching** | Automático por prefijo (los hints de cache se ignoran) | Declarativo mediante `cache_control: {"type": "ephemeral"}` |
| **Soporte de ecosistema** | OpenAI, Azure, NVIDIA NIM (Nemotron cloud), Groq, DeepSeek, Workers AI, Ollama local | Claude (Anthropic) |

### 4. Streaming SSE Unificado
Ambos adaptadores implementan streaming Server-Sent Events (SSE) entregando un flujo de eventos normalizados:
- `StreamEventText`: fragmentos incrementales de texto.
- `StreamEventToolCall`: fragmentos incrementales de argumentos de herramientas (ensamblados de forma segura).
- `StreamEventEnd`: evento final que contiene el `stop_reason` y el `usage` acumulado.
En el adaptador de OpenAI se exige enviar `stream_options: {"include_usage": true}` para que el usage sea reportado por el servidor sin requerir estimaciones heurísticas locales.

### 5. Router de Inferencia con Fallback Resiliente
El `Router` gestiona la selección del proveedor para cada ruta funcional (`chat`, `voice`, etc.):
- Cada ruta define un proveedor primario y una lista ordenada de fallbacks.
- Ante errores recuperables (códigos HTTP 429 de saturación, 5xx del proveedor o timeouts de red), el router commuta automáticamente al siguiente proveedor en la lista sin perder el estado de la conversación.
- Los errores de validación de cliente (400, 401 de credenciales inválidas) no activan fallback y fallan de inmediato.

---

## 3. Evaluación de Trade-Offs Duales

### Trade-offs para Nosotros como Proveedores
- **Minimización de OPEX y Vendor Lock-in (Positivo)**:
  - Libertad absoluta para redirigir el tráfico a los modelos más eficientes o económicos según evolucione el mercado sin tocar código.
  - En desarrollo y pruebas, capacidad de usar endpoints cloud gratuitos o económicos (como NVIDIA NIM Nemotron o servidores Ollama locales) antes de consumir cuotas de modelos comerciales premium.
- **Complejidad de Mantenimiento de Adaptadores (Negativo)**:
  - Requiere mantener la traducción y compatibilidad entre dos dialectos API principales (OpenAI Chat Completions y Anthropic Messages).
  - *Mitigación*: Se implementa una suite de conformidad común (`TestConformance_Common`) que ejecuta exactamente los mismos casos borde contra ambos adaptadores mediante un simulador local (`llmtest.Server`), previniendo desvíos de comportamiento.
- **Preservación del Margen Comercial (Positivo)**:
  - Posibilidad de absorber subidas de precios de un proveedor específico migrando instantáneamente a alternativas más convenientes.

### Trade-offs para el Cliente Final
- **Máxima Resiliencia y Disponibilidad (Positivo)**:
  - Ante una caída global de OpenAI o Anthropic, el asistente domótico conmuta de inmediato al proveedor secundario o a un modelo local en cuestión de milisegundos. El usuario nunca experimenta un apagón total en su hogar.
- **Soberanía y Privacidad de Datos (Positivo)**:
  - Los hogares con requerimientos estrictos de privacidad pueden configurar un proveedor local (`type: openai` apuntando a Ollama/vLLM en su red LAN), reteniendo todo el procesamiento en el edge sin enviar telemetría a servidores externos.
- **Consistencia de Experiencia de Usuario (Positivo)**:
  - El usuario recibe respuestas uniformes, ejecuciones precisas de herramientas domóticas y respuestas fluidas por streaming sin notar qué proveedor está respondiendo en cada momento.

---

## 4. Estrategia Cloudflare-First

1. **Alineación con Cloudflare AI Gateway**:
   - En producción (Fase 1, M7), el tráfico del edge saldrá hacia el Worker `llm-gw` conectado a Cloudflare AI Gateway.
   - Dado que AI Gateway expone interfaces compatibles con los estándares OpenAI y Anthropic (BYOK - Bring Your Own Keys), el diseño de `llmkit` encaja directamente con la arquitectura cloud de Cloudflare sin requerir capas de adaptación intermedias.
2. **Registro y Privacidad**:
   - Todas las llamadas a través de AI Gateway llevarán la cabecera `cf-aig-collect-log-payload: false` para asegurar que las conversaciones privadas de los usuarios nunca se almacenen en los logs de la nube, cumpliendo con los estándares de privacidad del producto.
3. **Métricas en Edge y Cuotas**:
   - La normalización de `Usage` permite alimentar directamente el motor de cuotas en Durable Objects (`HouseholdLedger`) y Cloudflare Analytics Engine.

---

## 5. Pruebas y Criterios de Aceptación (TDD)

1. **Suite de Conformidad Común (`TestConformance_Common`)**:
   - Mismo test ejecutado sobre adaptadores OpenAI y Anthropic:
     - Mensajes de texto simples (single-turn y multi-turn).
     - Invocación de herramienta simple con parámetros estructurados.
     - Invocación de herramientas en paralelo (múltiples tool calls en un solo turno).
     - Procesamiento de resultados de herramientas (`tool_result`) y reanudación de respuesta.
     - Streaming SSE con ensamblado de texto y de bloques `tool_call`.
     - Extracción correcta de métricas de `Usage` (input, output, cache read/write).
     - Mapeo adecuado de `StopReason`.
2. **Simulador `llmtest.Server`**:
   - Servidor HTTP local con capacidad de inyectar respuestas reales, flujos SSE simulados, retardos, fallos de red, errores 429 y 500.
3. **Pruebas de Router y Fallback**:
   - Conmutación automática ante errores 429 y 500 del proveedor primario hacia el secundario.
   - Cancelación limpia de contextos sin fuga de goroutines (`goleak`).
4. **Smoke Test Opcional contra Endpoint Real**:
   - Test de integración activable mediante build tag o variable de entorno para validar contra endpoints reales (como Nemotron cloud o Ollama).
