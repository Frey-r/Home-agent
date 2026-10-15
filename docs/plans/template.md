# Plantilla de Plan de Trabajo Persistente

# Plan: [Nombre Claro y Conciso de la Tarea / Funcionalidad]

- **ID del Plan**: `YYYY-MM-DD_<nombre_tarea>`
- **Estado**: [No Iniciado | En Progreso | En Revisión | Completado | Pausado / Bloqueado]
- **Fecha de Creación**: YYYY-MM-DD
- **Última Actualización**: YYYY-MM-DD
- **ADR Relacionado**: [Enlace a ADR o N/A]

---

## 1. Objetivo y Alcance

[Descripción concisa del problema que se aborda, qué se construirá y qué queda explícitamente fuera de alcance.]

---

## 2. Requisitos Previos y Contexto Técnico

- **Componentes Afectados**: [ej. `edge-core/pkg/...`, `python-worker/...`, `data/...`]
- **Contratos de Datos / Protocolos**: [ej. Entidad Normalizada, endpoint MQTT, etc.]
- **Servicios Cloudflare Involucrados**: [R2 (Buckets), Queues, Workers, D1/KV, Tunnels o Ninguno (100% Local)]
- **Impacto Comercial / Trade-Off**: [Minimización de OPEX para el proveedor y preservación de privacidad/latencia para el cliente]
- **Dependencias**: [Nuevas librerías o herramientas necesarias]

---

## 3. Estrategia de Pruebas Primero (TDD)

*Definir explícitamente los tests antes de escribir el código de producción.*

- [ ] **Test Unitario / Integración 1**: `TestNombreDelComportamiento`
  - *Archivo*: `path/to/component_test.go` o `tests/test_component.py`
  - *Comportamiento esperado*: [Qué inputs recibe y qué resultado verifica]
  - *Casos de error / límite*: [Qué condiciones de fallo se cubren]
- [ ] **Test Unitario / Integración 2**: ...

---

## 4. Fases de Ejecución y Tareas

### Fase 1: Pruebas Iniciales (Rojo)
- [ ] Crear archivos de test y assertions.
- [ ] Ejecutar suite y verificar que fallan como se espera (`go test` / `pytest`).

### Fase 2: Implementación Mínima (Verde)
- [ ] Implementar la lógica mínima requerida para satisfacer los tests.
- [ ] Ejecutar suite y verificar que todos los tests pasen exitosamente.

### Fase 3: Refactorización y Calidad
- [ ] Limpiar código, optimizar estructuras y comprobar estilo / linter.
- [ ] Re-ejecutar toda la suite de pruebas (cero regresiones).

### Fase 4: Documentación y Cierre
- [ ] Actualizar documentación técnica o ADR si surgieron variaciones.
- [ ] Marcar este plan como Completado en `docs/plans/README.md`.

---

## 5. Bitácora de Progreso y Memoria de Sesiones

*Espacio para registrar notas de avance, bloqueos o decisiones tomadas durante la ejecución:*
- **[YYYY-MM-DD HH:MM]**: [Registro de lo realizado o estado actual al pausar la sesión].
