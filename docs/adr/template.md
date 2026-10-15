# Plantilla de Registro de Decisión de Arquitectura (ADR)

# ADR-NNNN: [Título Breve y Descriptivo de la Decisión]

- **Estado**: [Propuesto | Aceptado | Rechazado | Reemplazado por ADR-XXXX]
- **Fecha**: YYYY-MM-DD
- **Autores**: [Nombre o Agente]
- **Contexto del Proyecto**: [Go edge-core, Python worker, Base de datos, AI Harness, Infraestructura Cloud, etc.]

---

## 1. Contexto y Planteamiento del Problema

[Describe el contexto actual, el problema técnico o de negocio que se necesita resolver y las restricciones relevantes (por ejemplo: hardware ARM64 limitado, latencia baja, aislamiento de procesos, sin docker, servicios cloud requeridos).]

---

## 2. Opciones Consideradas

### Opción 1: [Nombre de la Opción]
- **Descripción**: [Resumen de cómo funciona]
- **Ventajas (+)**:
  - [Ventaja 1]
- **Desventajas (-)**:
  - [Desventaja 1]

### Opción 2: [Nombre de la Opción]
- **Descripción**: [Resumen de cómo funciona]
- **Ventajas (+)**:
  - [Ventaja 1]
- **Desventajas (-)**:
  - [Desventaja 1]

---

## 3. Decisión Tomada

[Describe claramente la opción elegida y la justificación técnica y comercial que llevó a tomarla sobre las alternativas.]

---

## 4. Análisis de Trade-Offs (Foco Comercial)

### 4.1. Impacto para Nosotros como Proveedores
- **Costos Operativos (OPEX)**: [Impacto en costos de nube, almacenamiento, ancho de banda o licencias]
- **Mantenibilidad y Soporte**: [Facilidad de depuración remota, riesgo de llamadas a soporte técnico]
- **Velocidad de Entrega**: [Tiempos de desarrollo e integración]

### 4.2. Impacto para el Cliente Final
- **Experiencia de Usuario (UX) y Latencia**: [Tiempos de respuesta, fluidez de la interfaz y automatizaciones]
- **Privacidad y Seguridad**: [Datos que quedan locales vs enviados fuera del hogar]
- **Continuidad Operativa**: [Comportamiento del sistema ante cortes de internet]

### 4.3. Evaluación Cloudflare-First (Si aplica infraestructura Cloud)
- **Servicio Cloudflare Seleccionado**: [R2 (Buckets) | Queues (Colas) | Workers (Cómputo Edge) | D1/KV (Datos) | Tunnels]
- **Justificación Costo / Calidad**: [Por qué este servicio de Cloudflare optimiza costo sin penalizar calidad, o justificación explícita si se propuso una alternativa]

---

## 5. Consecuencias y Efectos Secundarios

### Positivas (+)
- [Beneficio técnico o comercial esperado]

### Negativas / Riesgos Mitigados (-)
- [Compromiso técnico asumido, deuda controlada o consideraciones a vigilar]

---

## 6. Referencias y Documentación Relacionada
- [Enlace a documentación oficial, issues, commits o especificaciones de Cloudflare]
