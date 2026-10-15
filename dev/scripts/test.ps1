<#
.SYNOPSIS
  Ejecuta go vet + go test -v -race en todos los modulos de go.work (AGENTS.md, Mandato I).

.PARAMETER Integration
  Agrega -tags integration y carga dev/.env (requiere el HA de dev/docker-compose.yml
  y haber ejecutado ha-bootstrap; ver dev/README.md).

.PARAMETER Run
  Filtro opcional de tests (go test -run).

.EXAMPLE
  powershell -ExecutionPolicy Bypass -File dev/scripts/test.ps1
  powershell -ExecutionPolicy Bypass -File dev/scripts/test.ps1 -Integration
#>
param(
  [switch]$Integration,
  [string]$Run
)
$ErrorActionPreference = 'Stop'
$root = Resolve-Path (Join-Path $PSScriptRoot '..\..')
Set-Location $root

# gcc instalado via winget puede no estar en el PATH de la sesion actual.
$env:Path = [Environment]::GetEnvironmentVariable('Path', 'Machine') + ';' + [Environment]::GetEnvironmentVariable('Path', 'User')
if (-not (Get-Command gcc -ErrorAction SilentlyContinue)) {
  throw 'gcc no encontrado: -race requiere CGO. Instalar: winget install BrechtSanders.WinLibs.POSIX.UCRT'
}
# CGO solo para el race detector; los binarios de produccion usan CGO_ENABLED=0.
$env:CGO_ENABLED = '1'

$tags = @()
if ($Integration) {
  $envFile = Join-Path $root 'dev\.env'
  if (-not (Test-Path $envFile)) {
    throw 'falta dev/.env: levanta HA (docker compose -f dev/docker-compose.yml up -d) y ejecuta ha-bootstrap (ver dev/README.md)'
  }
  Get-Content $envFile | Where-Object { $_ -match '^\s*([A-Z_]+)=(.*)$' } | ForEach-Object {
    Set-Item "env:$($Matches[1])" $Matches[2]
  }
  $tags = @('-tags', 'integration')
}

$modules = (go work edit -json | ConvertFrom-Json).Use | ForEach-Object { Join-Path $root $_.DiskPath }
$failed = $false
foreach ($m in $modules) {
  Write-Host "==> $m" -ForegroundColor Cyan
  Push-Location $m
  try {
    go vet @tags ./...
    if ($LASTEXITCODE -ne 0) { $failed = $true }

    $testArgs = @('test', '-v', '-race', '-count=1') + $tags
    if ($Run) { $testArgs += @('-run', $Run) }
    $testArgs += './...'
    go @testArgs
    if ($LASTEXITCODE -ne 0) { $failed = $true }
  }
  finally {
    Pop-Location
  }
}

if ($failed) {
  Write-Host 'FALLO' -ForegroundColor Red
  exit 1
}
Write-Host 'OK' -ForegroundColor Green
