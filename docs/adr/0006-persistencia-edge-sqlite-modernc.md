# ADR-0006: Persistencia del edge con SQLite (`modernc.org/sqlite`, Go puro) y migraciones embebidas

- **Estado**: Aceptado
- **Fecha**: 2026-10-15
- **Autores**: Equipo de Desarrollo / Agente de IA
- **Contexto del Proyecto**: `edge-core` en Go, base de datos local, política de migraciones

---

## 1. Contexto y Planteamiento del Problema

`edge-core` guarda localmente, en `/data` de la app de HA, lo siguiente: miembros del hogar, conversaciones, memoria, notas (búsqueda de texto completo), recordatorios, jobs, confirmaciones, auditoría y el outbox e inbox del relay.

Requisitos:

- Compilar con `CGO_ENABLED=0`, para hacer cross-compile trivial a `aarch64` y `amd64` y obtener un binario estático.
- Lecturas concurrentes y escrituras sin `SQLITE_BUSY`.
- Búsqueda de texto completo (FTS5).
- Migraciones que funcionen cuando la app se actualiza por la tienda de HA, sin intervención del usuario.

El documento original nombraba `glebarez/sqlite`, que es un driver de GORM montado sobre modernc.

## 2. Opciones Consideradas

### Opción 1: `mattn/go-sqlite3` (CGO)
- **Ventajas (+)**: El driver más usado y el más rápido.
- **Desventajas (-)**: **Requiere CGO**, lo que rompe el cross-compile simple y el binario estático.

### Opción 2: `ncruces/go-sqlite3` (WASM/wazero)
- **Ventajas (+)**: No requiere CGO y permite cargar `sqlite-vec` para vectores sin CGO.
- **Desventajas (-)**: Consume más memoria (runtime WASM), los bindings de `sqlite-vec` podrían quedar desactualizados y hoy no necesitamos vectores.

### Opción 3: `glebarez/sqlite` + GORM
- **Ventajas (+)**: Un ORM rápido de usar.
- **Desventajas (-)**: Agrega una capa de abstracción y "magia" (auto-migraciones implícitas) que dificulta controlar el esquema, lo que choca con el Mandato II (ADR para cambios de esquema).

### Opción 4: `modernc.org/sqlite` directo con `database/sql` (Elegida)
- **Ventajas (+)**:
  - Go puro, sin CGO.
  - Es SQLite real, transpilado a Go, con FTS5.
  - Usa la API estándar `database/sql`.
  - Pocas dependencias.
- **Desventajas (-)**: Algo más lento que mattn, aunque irrelevante para la carga de un hogar.

## 3. Decisión Tomada

Se adopta **`modernc.org/sqlite` directo** con `database/sql` y SQL escrito a mano, con esta configuración:

- **Pragmas** al abrir: `journal_mode=WAL`, `busy_timeout=5000`, `foreign_keys=ON`, `synchronous=NORMAL`.
- **Transacciones de escritura** en modo `IMMEDIATE`, para evitar deadlocks cuando una transacción intenta pasar de lectura a escritura.
- **Migraciones**:
  - Son archivos `NNNN_<nombre>.sql` embebidos con `//go:embed`.
  - Se aplican en orden, cada una en su propia transacción.
  - Se registran en la tabla `schema_migrations(version, applied_at)`.
  - Si una falla, se hace rollback y la versión no cambia.
  - Aplicarlas es idempotente.
  - No se usa ninguna herramienta externa de migraciones.
- **Vectores**: la búsqueda semántica (Fase 2) se hará con embeddings guardados como BLOB y similitud coseno por fuerza bruta, detrás de la interfaz `memory.Retriever`. Si algún día la escala lo exige, se reevalúa la Opción 2 en un ADR nuevo.

Cada cambio de esquema posterior requiere una migración nueva. Los cambios que alteren el modelo de datos de forma relevante requieren además un ADR, según el Mandato II.

## 4. Análisis de Trade-Offs (Foco Comercial)

### 4.1. Impacto para Nosotros como Proveedores
- **Costos Operativos (OPEX)**: Cero costo cloud, porque los datos del hogar viven en el edge.
- **Mantenibilidad y Soporte**:
  - Las migraciones automáticas al actualizar la app eliminan pasos manuales de soporte.
  - El SQL explícito facilita depurar.
  - Las tablas son SQLite estándar, así que se pueden inspeccionar con cualquier herramienta.
- **Velocidad de Entrega**: El cross-compile multi-arch funciona sin toolchain C en la build.

### 4.2. Impacto para el Cliente Final
- **Experiencia de Usuario (UX) y Latencia**: Lecturas locales en microsegundos a milisegundos.
- **Privacidad y Seguridad**:
  - La memoria y las conversaciones quedan en el dispositivo del cliente.
  - Se incluyen en los backups de HA, que quedan bajo control del usuario.
- **Continuidad Operativa**:
  - Funciona sin internet.
  - WAL con `synchronous=NORMAL` tolera cortes de energía sin corromper la base. En el peor caso se pierden las últimas transacciones.
- **Costo total y consumo**: Huella de memoria baja, apta para una RPi.

### 4.3. Evaluación Cloudflare-First (Si aplica infraestructura Cloud)
- **Servicio Cloudflare Seleccionado**: No aplica (persistencia local).
- **Justificación Costo / Calidad**: No hay costo cloud.

## 5. Consecuencias y Efectos Secundarios

### Positivas (+)
- Un binario estático y multi-arch.
- Esquema versionado y auditable.

### Negativas / Riesgos Mitigados (-)
- **Backups "en caliente" de HA con WAL.** Copiar los archivos mientras hay escrituras puede dejar una copia inconsistente. En M6 se mitiga con un hook `backup_pre` que ejecute `wal_checkpoint(TRUNCATE)` o `VACUUM INTO` para producir una copia consistente.
- **Si `modernc.org/sqlite` dejara de mantenerse**, la interfaz `database/sql` permite cambiar a `ncruces/go-sqlite3` con poco esfuerzo.
- **Tests en Windows con `-race`.** Requieren `CGO_ENABLED=1` y gcc solo para el race detector. La producción se compila con `CGO_ENABLED=0`.

## 6. Referencias y Documentación Relacionada
- [modernc.org/sqlite](https://pkg.go.dev/modernc.org/sqlite)
- [SQLite WAL](https://www.sqlite.org/wal.html)
- [ncruces/go-sqlite3](https://github.com/ncruces/go-sqlite3)
- [docs/arquitectura.md](../arquitectura.md) §4
