# Registro y Memoria de Planes de Trabajo

Este directorio almacena el historial y estado de todos los planes de trabajo, tareas complejas y refactorizaciones del proyecto. Sirve como **memoria persistente** para que los agentes de IA y desarrolladores puedan reanudar tareas entre sesiones sin perder contexto ni duplicar esfuerzos.

## Planes Activos y Recientes

| ID del Plan | Título / Objetivo | Estado | Fecha Inicio | Última Actualización |
| :--- | :--- | :--- | :--- | :--- |
| [2026-10-15_roadmap_producto](2026-10-15_roadmap_producto.md) | Roadmap maestro: Fases 0–2, milestones M0–M11 | En Progreso | 2026-10-15 | 2026-10-15 |
| [2026-10-15_m0_fundaciones](2026-10-15_m0_fundaciones.md) | M0: monorepo, config, store, cliente HA base, entorno dev | En Progreso | 2026-10-15 | 2026-10-15 |

---

## Historial de Planes Completados

| ID del Plan | Título | Fecha Completado | ADRs Asociados |
| :--- | :--- | :--- | :--- |
| *(Aún no hay planes completados)* | - | - | - |

---

## Cómo Crear un Nuevo Plan
1. Copiar la plantilla [docs/plans/template.md](file:///c:/Users/eduar/Workspace/personal/Dom%C3%B3tica/docs/plans/template.md).
2. Nombrar el archivo con el formato `YYYY-MM-DD_<nombre_tarea>.md` (ejemplo: `2026-10-15_inicializar_core_go.md`).
3. Definir primero los tests a construir (TDD) y las subtareas con checkboxes.
4. Agregar el plan a la tabla de **Planes Activos** en este archivo `README.md`.
5. A medida que se avance, marcar las casillas correspondientes y registrar notas en la bitácora de progreso.
