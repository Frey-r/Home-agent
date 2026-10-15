# ADR-0002: Estrategia Cloudflare-First y Análisis Dual de Trade-Offs Comerciales

- **Estado**: Aceptado. Parcialmente reemplazado por [ADR-0004](0004-cerebro-edge-nube-cloudflare-relay-durable-objects.md), solo en la conectividad nube↔hogar. La conexión pasa de un Cloudflare Tunnel por hogar a un relay con Durable Objects, porque Cloudflare limita a 1.000 túneles por cuenta y a 200–3.500 registros DNS por zona. La política Cloudflare-First y el análisis dual se mantienen vigentes.
- **Fecha**: 2026-10-14
- **Autores**: Equipo de Desarrollo / Agente de IA
- **Contexto del Proyecto**: Infraestructura Cloud, Modelo de Negocio y Criterios de Selección Tecnológica

---

## 1. Contexto y Planteamiento del Problema

El Hub de Domótica es un producto comercial diseñado para operar de forma confiable en los hogares de los clientes. Como proveedores, cada hub desplegado no debe convertirse en un centro de costos insostenible en la nube (OPEX descontrolado por egress, almacenamiento o cómputo centralizado).

Al mismo tiempo, el cliente exige:
1. Privacidad absoluta en su hogar.
2. Cero latencia en el control de dispositivos (ej. pulsar un interruptor o pedir por voz encender la luz debe ser instantáneo).
3. Conectividad remota segura y transparente sin necesidad de configurar puertos en el router.

Para cualquier servicio que requiera infraestructura en la nube (almacenamiento de actualizaciones de firmware, colas de telemetría o eventos asíncronos, túneles de acceso remoto, metadatos y cómputo serverless en el edge), se necesita definir una estrategia uniforme que garantice la mejor relación costo/calidad.

---

## 2. Opciones Consideradas

### Opción 1: Nube Tradicional Centralizada (AWS / GCP / Azure)
- **Descripción**: Alojar microservicios en Kubernetes/ECS, buckets S3 y colas SQS/PubSub.
- **Ventajas (+)**: Ecosistemas masivos y maduros.
- **Desventajas (-)**:
  - Tarifas exorbitantes de transferencia de datos saliente (**data egress fees**).
  - Costo base elevado por infraestructura fija (clusters, balanceadores de carga, IPs públicas).
  - Mayor latencia al enrutar tráfico a regiones de datos específicas.

### Opción 2: Estrategia Cloudflare-First (Elegida)
- **Descripción**: Basar toda la arquitectura cloud en el stack serverless y de red global de Cloudflare:
  - **Cloudflare Tunnels**: Acceso remoto Zero-Trust bidireccional sin abrir puertos en los routers residenciales.
  - **Cloudflare R2**: Almacenamiento de objetos y artefactos (firmware OTA, plugins) con **$0 de tarifa de egress**.
  - **Cloudflare Queues**: Procesamiento asíncrono y desacoplado de eventos, métricas y telemetría de hubs.
  - **Cloudflare Workers**: API de aprovisionamiento global y cómputo serverless ultra ligero en el edge.
  - **Cloudflare D1 / KV**: Almacenamiento distribuido de bajo costo para metadatos de aprovisionamiento y claves.
- **Ventajas (+)**:
  - Eliminación del riesgo financiero por transferencia de datos saliente (R2 no cobra egress).
  - Costos de cómputo fraccionados y planes generosos que escalan orgánicamente por uso real.
  - Red global con latencia mínima cerca de los hogares de los usuarios.
- **Desventajas (-)**:
  - Entorno de ejecución de Workers basado en V8/Wasm (no es un contenedor Linux completo en la nube, lo cual coincide con nuestra filosofía de mantener la lógica pesada en el edge local).

---

## 3. Decisión Tomada

Se adopta de forma mandataria:
1. **Política Cloudflare-First**: Toda necesidad de infraestructura en la nube (buckets, colas, cómputo edge, DNS, túneles) priorizará de forma predeterminada el catálogo de Cloudflare (R2, Queues, Workers, D1/KV, Cloudflare Tunnels).
2. **Excepción justificada**: Cualquier propuesta de usar un proveedor cloud externo a Cloudflare requerirá un ADR demostrando cuantitativamente que la solución de Cloudflare no cumple técnica o económicamente.
3. **Análisis Dual de Trade-Offs**: Toda decisión de arquitectura en el proyecto debe explicitar el impacto dual:
   - Para nosotros como **proveedores** (OPEX, soporte técnico, mantenibilidad, velocidad).
   - Para el **cliente final** (experiencia de usuario, latencia, privacidad, funcionamiento offline).

---

## 4. Análisis de Trade-Offs (Foco Comercial)

### 4.1. Impacto para Nosotros como Proveedores
- **Costos Operativos (OPEX)**: Drástica reducción de costos recurrentes. R2 sin costos de egress protege el margen del negocio cuando miles de dispositivos descarguen actualizaciones de firmware o modelos de IA.
- **Mantenibilidad y Soporte**: Menos piezas móviles y servidores que parchar; infraestructura administrada globalmente por Cloudflare.
- **Escalabilidad**: Soporta millones de solicitudes concurrentes automáticamente sin aprovisionar capacidad fija.

### 4.2. Impacto para el Cliente Final
- **Experiencia de Usuario (UX)**: Conexión remota fluida y casi instantánea gracias al ruteo de Cloudflare Anycast.
- **Privacidad y Seguridad**: Zero-Trust nativo; sin agujeros de seguridad como UPnP ni puertos abiertos hacia la LAN del usuario.
- **Confiabilidad**: El hub local sigue operando localmente incluso si la conexión a internet cae.

### 4.3. Evaluación Cloudflare-First
- **Servicios seleccionados**: Cloudflare Tunnels, R2, Queues, Workers y D1/KV.
- **Justificación**: Máxima calidad y seguridad al menor coste unitario por usuario del mercado.

---

## 5. Consecuencias y Efectos Secundarios

### Positivas (+)
- Alineación total entre arquitectura técnica y viabilidad financiera del producto comercial.
- Claridad absoluta para agentes de IA y desarrolladores al evaluar cualquier integración externa.

### Negativas / Riesgos Mitigados (-)
- Dependencia del ecosistema Cloudflare para las funciones en la nube. Se mitiga mediante el desacoplamiento: el hub local (`edge-core`) es autónomo y no depende de la nube para el control domótico en la red local.

---

## 6. Referencias y Documentación Relacionada
- [AGENTS.md](file:///c:/Users/eduar/Workspace/personal/Dom%C3%B3tica/AGENTS.md)
- [ADR-0001: Adopción Obligatoria de TDD, ADRs y Planes Persistentes](file:///c:/Users/eduar/Workspace/personal/Dom%C3%B3tica/docs/adr/0001-adopcion-tdd-adrs-planes.md)
- [arquitectura_teórica.md](file:///c:/Users/eduar/Workspace/personal/Dom%C3%B3tica/arquitectura_te%C3%B3rica.md)
