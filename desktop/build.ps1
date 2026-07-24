# Build BeLeader Desktop for Windows.
# Prerequisites: Go 1.21+, Node.js (npm)
#
# Usage:
#   .\build.ps1              # Build for Windows amd64
#   .\build.ps1 -All         # Build for all platforms

param([switch]$All)

$ErrorActionPreference = "Stop"
$ROOT = Split-Path -Parent (Split-Path -Parent $MyInvocation.MyCommand.Path)
$DESKTOP_DIR = Join-Path $ROOT "desktop"
$WEB_DIR = Join-Path $ROOT "web"
$OUTPUT_DIR = Join-Path $ROOT "dist"

# Build web frontend
Write-Host "=== Building web frontend ===" -ForegroundColor Cyan
Push-Location $WEB_DIR
try {
    npm install --silent
    npm run build
} finally {
    Pop-Location
}

# Sync web dist into desktop embed directory
$WEB_DIST = Join-Path $DESKTOP_DIR "webdist"
if (Test-Path $WEB_DIST) { Remove-Item -Recurse -Force $WEB_DIST }
Copy-Item -Recurse (Join-Path $WEB_DIR "dist") $WEB_DIST

# Build Go binary
Write-Host "=== Building desktop binary ===" -ForegroundColor Cyan
Push-Location $DESKTOP_DIR
try {
    go mod tidy

    if (-not (Test-Path $OUTPUT_DIR)) { New-Item -ItemType Directory -Path $OUTPUT_DIR | Out-Null }

    function Build-One {
        param($Os, $Arch, $Ext)
        $out = Join-Path $OUTPUT_DIR "beleader-${Os}-${Arch}${Ext}"
        Write-Host "  -> $Os/$Arch" -ForegroundColor Yellow
        $env:CGO_ENABLED = "0"
        $env:GOOS = $Os
        $env:GOARCH = $Arch
        go build -ldflags="-s -w" -o $out .
        Write-Host "     $(if (Test-Path $out) { 'OK' } else { 'FAILED' }) $out"
    }

    if ($All) {
        Build-One "windows" "amd64" ".exe"
        Build-One "linux"   "amd64" ""
        Build-One "darwin"  "amd64" ""
        Build-One "darwin"  "arm64" ""
    } else {
        Build-One "windows" "amd64" ".exe"
    }
} finally {
    Pop-Location
}

Write-Host "=== Done. Output in $OUTPUT_DIR ===" -ForegroundColor Cyan
Get-ChildItem $OUTPUT_DIR | ForEach-Object { Write-Host "  $($_.Name)  $('{0:N0}' -f $_.Length) bytes" }