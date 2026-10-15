# ADR-0001: Adopción Obligatoria de TDD, ADRs y Planes Persistentes como Memoria

- **Estado**: Aceptado
- **Fecha**: 2026-10-14
- **Autores**: Equipo de Desarrollo / Agente de IA
- **Contexto del Proyecto**: Metodología transversal de desarrollo de software para el Hub de Domótica

---

## 1. Contexto y Planteamiento del Problema

El proyecto es un Hub de Domótica Comercial embebido que integra Go (core orquestador) y Python (worker de drivers dinámicos con `uv`), con control de hardware, protocolos de red IoT y un AI Harness. 

Debido a la criticidad de la estabilidad en hardware físico (Raspberry Pi/ARM64) y la interacción entre múltiples subprocesos y agentes de IA:
1. El código sin pruebas previas introduce regresiones difíciles de diagnosticar en dispositivos finales.
2. Las decisiones técnicas tomadas en conversaciones aisladas de agentes de IA se pierden fácilmente cuando expira la ventana de contexto.
3. Los planes de trabajo volátiles impiden el seguimiento histórico y la continuidad del desarrollo entre distintas sesiones.

---

## 2. Opciones Consideradas

### Opción 1: Desarrollo tradicional (Code-first) con documentación informal en chat
- **Ventajas (+)**: Prototipado inicial más rápido.
- **Desventajas (-)**: Cobertura de tests deficiente o inexistente, pérdida de memoria técnica, incompatibilidades no detectadas entre Go y Python, degradación de la calidad a mediano plazo.

### Opción 2: TDD estricto + ADRs versionados + Planes persistidos en el repositorio (Elegida)
- **Ventajas (+)**:
  - Garantiza que cada pieza de código responde a una especificación validada antes de escribirse.
  - El repositorio retiene la memoria técnica de largo plazo (ADRs y planes de trabajo).
  - Cualquier agente o desarrollador puede incorporarse y conocer el historial y estado exacto del proyecto.
- **Desventajas (-)**: Mayor esfuerzo inicial de disciplina y documentación previa antes de implementar.

---

## 3. Decisión Tomada

Se adopta como política obligatoria e inquebrantable para el repositorio:
1. **Test-Driven Development (TDD)**: Todo cambio o funcionalidad debe iniciar con un test que falle (Rojo), seguido por la implementación mínima (Verde) y posterior refactorización (Refactor).
2. **Architecture Decision Records (ADR)**: Todo cambio de dependencias, protocolos, modelos de datos o arquitectura debe plasmarse en un ADR numerado en `docs/adr/`.
3. **Planes de Trabajo Persistentes**: Todo hito, funcionalidad o refactor se planifica y registra en `docs/plans/` con seguimiento por tareas y estados, sirviendo de memoria duradera para los agentes.

---

## 4. Consecuencias y Efectos Secundarios

### Positivas (+)
- Alta confiabilidad del software desplegado en el hardware IoT.
- Trazabilidad total de decisiones y continuidad de trabajo entre sesiones de IA.
- Contratos claros y testeados para la comunicación Go ↔ Python.

### Negativas / Riesgos Mitigados (-)
- Requiere disciplina para no codificar sin test previo ni omitir la actualización de planes. Se mitiga mediante el protocolo explícito en `AGENTS.md`.

---

## 5. Referencias y Documentación Relacionada
- [AGENTS.md](file:///c:/Users/eduar/Workspace/personal/Dom%C3%B3tica/AGENTS.md)
- [arquitectura_teórica.md](file:///c:/Users/eduar/Workspace/personal/Dom%C3%B3tica/arquitectura_te%C3%B3rica.md)
