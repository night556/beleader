# ═══════════════════════════════════════════════════════════════
# BeLeader Build System (Windows PowerShell)
# ═══════════════════════════════════════════════════════════════
#
# Quick start:
#   .\make.ps1 run             → 一键构建并启动 http://localhost:8080
#   .\make.ps1 release:server  → 构建服务端发布版 → bin/
#
# 产物命名：{程序}-{debug|release}-{os}-{arch}.exe  → 统一输出到 bin/
#   bin/
#     beleader-backend-debug-windows-amd64.exe
#     beleader-backend-release-windows-amd64.exe

param([string]$cmd = "run")

$ErrorActionPreference = "Stop"

# ---- paths ----
$BIN_DIR       = "bin"

# ---- bin/ artifacts ----
$BACKEND_DEBUG   = "$BIN_DIR\beleader-backend-debug-windows-amd64.exe"
$BACKEND_RELEASE = "$BIN_DIR\beleader-backend-release-windows-amd64.exe"

function Ensure-BinDir {
    if (-not (Test-Path $BIN_DIR)) { New-Item -ItemType Directory $BIN_DIR | Out-Null }
}

# ═══════════════════════════════════════════════════════════════
# Build targets
# ═══════════════════════════════════════════════════════════════

function Build-Backend {
    Write-Host "Building Go backend (debug)..." -ForegroundColor Cyan
    Ensure-BinDir
    go build -o $BACKEND_DEBUG .
    Write-Host "  -> $BACKEND_DEBUG" -ForegroundColor Green
}

# ═══════════════════════════════════════════════════════════════
# Release targets
# ═══════════════════════════════════════════════════════════════

function Release-Server {
    Write-Host "Building Go backend (release)..." -ForegroundColor Cyan
    Ensure-BinDir
    go build -ldflags="-s -w" -o $BACKEND_RELEASE .
    Write-Host "  -> $BACKEND_RELEASE" -ForegroundColor Green
}

# ═══════════════════════════════════════════════════════════════
# Clean
# ═══════════════════════════════════════════════════════════════

function Clean {
    Remove-Item -Recurse -Force $BIN_DIR -ErrorAction SilentlyContinue
    Remove-Item -Recurse -Force agents -ErrorAction SilentlyContinue
    Write-Host "Cleaned." -ForegroundColor Green
}

# ═══════════════════════════════════════════════════════════════
# Dispatch
# ═══════════════════════════════════════════════════════════════

switch ($cmd) {
    "build"          { Build-Backend }
    "all"            { Build-Backend }
    "clean"          { Clean }
    "run"            {
        Build-Backend
        Write-Host "Starting BeLeader on http://localhost:8080 ..." -ForegroundColor Cyan
        & $BACKEND_DEBUG
    }
    "dev-backend"    {
        Write-Host "Starting Go backend on http://localhost:8080 ..." -ForegroundColor Cyan
        go run . --port 8080
    }
    "dev"            {
        Write-Host "Starting Go backend on http://localhost:8080 ..." -ForegroundColor Cyan
        go run . --port 8080
    }
    "release:server"  { Release-Server }
    default {
        Write-Host "Unknown command: $cmd" -ForegroundColor Red
        Write-Host "Usage: .\make.ps1 [dev|build|release:server|clean|run]"
    }
}
