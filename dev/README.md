# Entorno de desarrollo

## Requisitos

| Herramienta | Versión | Para qué |
|---|---|---|
| Go | 1.27+ | edge-core |
| gcc (MinGW-w64, WinLibs) | — | solo para `go test -race` en Windows (`winget install BrechtSanders.WinLibs.POSIX.UCRT`) |
| Docker Desktop (WSL 2) | — | Home Assistant de desarrollo |

## 1. Levantar Home Assistant (demo)

```powershell
docker compose -f dev/docker-compose.yml up -d
```

- Levanta HA **2026.10.1**, fijada en [docker-compose.yml](docker-compose.yml), con la integración `demo:`.
- El demo trae unas 120 entidades: luces, cerraduras, cortinas y garage, clima, media players, calendarios, `todo` y `weather`.
- Queda disponible en http://localhost:8123.

## 2. Onboarding y token (una sola vez)

```powershell
go run ./edge/cmd/ha-bootstrap
```

El comando hace tres cosas:

1. Espera a que HA responda.
2. Crea el usuario owner `dev` con contraseña `dev-password`. Esta credencial **solo sirve para desarrollo local**.
3. Crea un token de larga duración y lo escribe en `dev/.env`, que no se versiona.

Si HA ya tenía hecho el onboarding, el comando falla con un mensaje claro. En ese caso, reinicia el entorno (ver abajo) o reutiliza tu `dev/.env`.

## 3. Tests (Mandato I: `go vet` + `go test -v -race`)

```powershell
# Unitarios, en todos los módulos de go.work
powershell -ExecutionPolicy Bypass -File dev/scripts/test.ps1

# Agrega los tests de integración contra el HA del paso 1 (carga dev/.env)
powershell -ExecutionPolicy Bypass -File dev/scripts/test.ps1 -Integration

# Filtrar por nombre de test
powershell -ExecutionPolicy Bypass -File dev/scripts/test.ps1 -Run TestMigrate
```

## 4. Ejecutar edge-core en modo dev

```powershell
Get-Content dev/.env | Where-Object { $_ -match '^\s*([A-Z_]+)=(.*)$' } | ForEach-Object { Set-Item "env:$($Matches[1])" $Matches[2] }
go run ./edge/cmd/edge-core run --once   # conecta, sincroniza, registra la versión y el número de entidades, y sale
go run ./edge/cmd/edge-core run          # queda corriendo hasta Ctrl+C
```

Variables de entorno: ver [`edge/internal/config`](../edge/internal/config/config.go). Los datos (SQLite) se guardan en `DOMOTICA_DATA_DIR`, que por defecto es `.data/` y no se versiona.

## Reiniciar el entorno

```powershell
docker compose -f dev/docker-compose.yml down
Get-ChildItem dev/ha-config -Force | Where-Object Name -ne 'configuration.yaml' | Remove-Item -Recurse -Force
Remove-Item dev/.env
```
