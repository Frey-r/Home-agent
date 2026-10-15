# ADR-0003: Construir sobre Home Assistant como capa IoT y distribuir como app de HA

- **Estado**: Aceptado
- **Fecha**: 2026-10-15
- **Autores**: Equipo de Desarrollo / Agente de IA
- **Contexto del Proyecto**: Arquitectura general, `edge-core` en Go, capa IoT, distribución

---

## 1. Contexto y Planteamiento del Problema

[arquitectura_teórica.md](../../arquitectura_teórica.md) proponía construir una capa IoT propia con cuatro piezas:

- un worker Python gestionado con `uv`, que instala en caliente librerías de fabricantes desde PyPI (LG, Samsung, Tuya, Shelly, Matter, BLE);
- un escáner mDNS/SSDP propio;
- un broker MQTT embebido;
- un OTA propio.

Esa capa es la parte más costosa de construir y mantener. Cada marca cambia sus APIs, y algunas librerías citadas, como `aiopylgtv`, ya están abandonadas. Matter, además, necesita su propio servidor y un border router Thread. Instalar paquetes de PyPI en runtime dentro de un venv único genera conflictos de dependencias y un riesgo de supply chain en hardware de clientes.

Por otro lado, el valor del producto está en el **cerebro**: el agente IA, la memoria, la proactividad, el asistente personal y los guardrails. No está en los drivers.

Hay tres restricciones:

- El producto es comercial.
- La Fase 1 debe validarse rápido con clientes reales.
- La Fase 2 incluye un hub propio.

## 2. Opciones Consideradas

### Opción 1: Capa IoT propia (arquitectura teórica original)
- **Descripción**: Worker Python con `uv` y drivers descargados bajo demanda, discovery propio y MQTT interno.
- **Ventajas (+)**:
  - Control total del stack y de la experiencia.
  - No depende de un proyecto de terceros.
- **Desventajas (-)**:
  - Exige años de trabajo para igualar la cobertura de dispositivos que ya existe.
  - Mantener un driver por marca indefinidamente se traduce en tickets de soporte cada vez que un fabricante cambia su API.
  - Instalar paquetes en runtime implica conflictos de dependencias en un venv único y riesgo de supply chain.
  - Matter, Thread y Zigbee requieren piezas adicionales complejas.

### Opción 2: Construir sobre Home Assistant (Elegida)
- **Descripción**: HA aporta las integraciones (más de 2.000), el discovery, Zigbee, Matter, Thread y el pipeline de voz (Assist). Nuestro `edge-core` corre como **app de HA** (los "add-ons" pasaron a llamarse "apps" en HA 2026.2) y consume HA por su WebSocket API ([ADR-0005](0005-integracion-ha-via-websocket-api.md)). Una integración Python mínima registra nuestro agente como `ConversationEntity` para la voz.
- **Ventajas (+)**:
  - Cobertura de dispositivos inmediata y mantenida por la comunidad.
  - Llegamos a producto en semanas, no en años.
  - Las actualizaciones se distribuyen por la tienda de apps de HA, así que no hace falta un OTA propio.
  - La licencia Apache-2.0 permite el uso comercial.
- **Desventajas (-)**:
  - Dependemos del ritmo de releases mensuales de HA y de algunos comandos WS no documentados, como los registros.
  - Las apps requieren Supervisor, es decir HA OS. Core y Supervised están deprecados desde 2025.12, y en HA Container solo hay soporte best-effort.
  - La marca "Home Assistant" no puede usarse en el nombre del producto.
  - HA ya trae agentes LLM nativos, así que la diferenciación debe venir del servicio gestionado, el multiusuario por Telegram, la memoria y proactividad, el asistente personal y la localización para Chile.

### Opción 3: Otra plataforma base (openHAB, Homey, SmartThings)
- **Descripción**: Usar otro hub como capa IoT.
- **Ventajas (+)**:
  - openHAB también es open source.
- **Desventajas (-)**:
  - Ecosistema y comunidad menores (openHAB).
  - Plataformas cerradas o dependientes de la nube (Homey, SmartThings), lo que contradice el principio edge-first.

## 3. Decisión Tomada

Se adopta la **Opción 2**: HA es la capa IoT y nuestro producto se distribuye como app de HA, con `edge-core` en Go más una integración Python mínima. Se eliminan el worker Python con `uv`, la instalación de PyPI en runtime, el escáner mDNS/SSDP, `device_catalog.json`, Mochi-MQTT, el supervisor de procesos propio y el OTA propio.

El **contrato de entidad normalizada** se mantiene como capa de abstracción sobre HA. Los IDs llevan un prefijo de origen (`ha:`), así que la fuente de dispositivos se podría reemplazar más adelante sin tocar el agente.

## 4. Análisis de Trade-Offs (Foco Comercial)

### 4.1. Impacto para Nosotros como Proveedores
- **Costos Operativos (OPEX)**:
  - **Cero costo cloud adicional**: el control de dispositivos es 100 % local.
  - No hace falta infraestructura de OTA ni de hosting de drivers. Las imágenes de la app se publican en un registry; ver ADR-0010.
- **Mantenibilidad y Soporte**:
  - Desaparece el mantenimiento de drivers por marca, que es la principal fuente de tickets.
  - A cambio, hay que vigilar la deriva de la API de HA. Se mitiga con una CI que prueba contra tres versiones de HA y prefiriendo comandos documentados.
- **Velocidad de Entrega**:
  - La Fase 0 (dogfooding) se estima en unas 7 a 8 semanas, frente a meses o años con drivers propios.

### 4.2. Impacto para el Cliente Final
- **Experiencia de Usuario (UX) y Latencia**:
  - Soporta prácticamente cualquier dispositivo que el cliente ya tenga.
  - El control es local, con latencia de LAN.
  - Los comandos simples por voz se resuelven en HA ("prefer local") sin pasar por el LLM.
- **Privacidad y Seguridad**:
  - El estado del hogar se procesa localmente.
  - Se elimina la ejecución de código descargado de PyPI en el equipo del cliente.
- **Continuidad Operativa**:
  - Sin internet, HA sigue controlando el hogar y ejecutando automatizaciones.
  - Solo se degrada el asistente basado en LLM; ver [ADR-0004](0004-cerebro-edge-nube-cloudflare-relay-durable-objects.md).
- **Costo total y consumo**:
  - En la Fase 1, el cliente necesita HA OS, que suele tener ya (por ejemplo una RPi5, que consume unos 5 a 7 W).
  - En la Fase 2, el hub propio incluye todo.

### 4.3. Evaluación Cloudflare-First (Si aplica infraestructura Cloud)
- **Servicio Cloudflare Seleccionado**: No aplica. Esta decisión es 100 % local (edge).
- **Justificación Costo / Calidad**: Mover los drivers a HA local evita cualquier cómputo cloud por dispositivo.

## 5. Consecuencias y Efectos Secundarios

### Positivas (+)
- Cobertura amplia de dispositivos desde el día uno.
- Menor superficie de ataque, porque no se instalan paquetes en runtime.
- El equipo puede concentrarse en el diferenciador: agente, memoria, asistente y guardrails.

### Negativas / Riesgos Mitigados (-)
- **Mercado acotado en la Fase 1** a usuarios de HA OS. Se mitiga con el hub de la Fase 2.
- **Riesgo de marca.** Se mitiga con un nombre comercial propio y consultando a la Open Home Foundation antes de lanzar.
- **Competencia con los agentes nativos de HA.** Se mitiga validando la disposición a pagar durante la Fase 0.
- **Comandos WS no documentados**, como los registros. Se aíslan detrás de un adaptador con tests contra una matriz de versiones.

## 6. Referencias y Documentación Relacionada
- [docs/arquitectura.md](../arquitectura.md)
- [HA: Deprecating Core and Supervised (2025-05-22)](https://www.home-assistant.io/blog/2025/05/22/deprecating-core-and-supervised-installation-methods-and-32-bit-systems/)
- [HA 2026.2 (add-ons → apps)](https://www.home-assistant.io/blog/2026/02/04/release-20262/)
- [App configuration](https://developers.home-assistant.io/docs/apps/configuration/)
- [Conversation entity](https://developers.home-assistant.io/docs/core/entity/conversation/)
- [Licencia de HA](https://www.home-assistant.io/developers/license/)
- [Uso comercial del logo de HA](https://github.com/home-assistant/assets/blob/master/logo/README.md)
