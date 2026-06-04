# ═══════════════════════════════════════════════════════════════
# IAmHuman Build System (Windows PowerShell)
# ═══════════════════════════════════════════════════════════════
#
# Quick start:
#   .\make.ps1 run             → 一键构建并启动 http://localhost:8080
#   .\make.ps1 dev-desktop     → 启动 Tauri 桌面应用（需先 dev-backend）
#   .\make.ps1 release:server  → 构建服务端发布版 → bin/
#   .\make.ps1 release:desktop → 构建桌面发布版 → bin/
#
# 产物命名：{程序}-{debug|release}-{os}-{arch}.exe  → 统一输出到 bin/
#   bin/
#     iamhuman-agent-release-windows-amd64.exe
#     iamhuman-backend-debug-windows-amd64.exe
#     iamhuman-backend-release-windows-amd64.exe
#     iamhuman-desktop-release-windows-amd64.exe

param([string]$cmd = "run")

$ErrorActionPreference = "Stop"

# ---- paths ----
$BIN_DIR       = "bin"
$DESKTOP_SRC   = "desktop\src-tauri"
$DESKTOP_BINARIES = "$DESKTOP_SRC\binaries"
$AGENT_SRC     = "robot\Cargo.toml"
$AGENT_BUILT   = "robot\target\release\iamhuman-agent.exe"

# ---- bin/ artifacts ----
$AGENT_RELEASE   = "$BIN_DIR\iamhuman-agent-release"
$BACKEND_DEBUG   = "$BIN_DIR\iamhuman-backend-debug-windows-amd64.exe"
$BACKEND_RELEASE = "$BIN_DIR\iamhuman-backend-release-windows-amd64.exe"
$DESKTOP_RELEASE = "$BIN_DIR\iamhuman-desktop-release-windows-amd64.exe"

# ---- binaries/ (for Rust include_bytes!) ----
$BACKEND_EMBED   = "$DESKTOP_BINARIES\iamhuman-backend-release"

function Ensure-BinDir {
    if (-not (Test-Path $BIN_DIR)) { New-Item -ItemType Directory $BIN_DIR | Out-Null }
}

function Ensure-CargoPath {
    $env:PATH = "$env:USERPROFILE\.cargo\bin;$env:PATH"
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

function Build-Agent {
    Write-Host "Building Rust agent (release)..." -ForegroundColor Cyan
    Ensure-CargoPath
    Ensure-BinDir
    cargo build --release --manifest-path $AGENT_SRC
    Copy-Item -Force $AGENT_BUILT $AGENT_RELEASE
    Write-Host "  -> $AGENT_RELEASE" -ForegroundColor Green
}

function Build-Desktop {
    Write-Host "Building Tauri desktop (release)..." -ForegroundColor Cyan
    Ensure-BinDir
    Push-Location desktop
    npm install --silent
    npm run build
    Pop-Location
    Ensure-CargoPath
    cargo build --release --manifest-path $DESKTOP_SRC\Cargo.toml
    Copy-Item -Force "$DESKTOP_SRC\target\release\iamhuman-desktop.exe" $DESKTOP_RELEASE
    Write-Host "  -> $DESKTOP_RELEASE" -ForegroundColor Green
}

# ═══════════════════════════════════════════════════════════════
# Release targets
# ═══════════════════════════════════════════════════════════════

function Release-Server {
    Ensure-BinDir
    Ensure-CargoPath

    # 1. Build agent (release)
    Write-Host "[1/2] Building Rust agent (release)..." -ForegroundColor Cyan
    cargo build --release --manifest-path $AGENT_SRC
    Copy-Item -Force $AGENT_BUILT $AGENT_RELEASE
    Write-Host "  -> $AGENT_RELEASE" -ForegroundColor Green

    # 2. Build Go backend (release, embeds agent via go:embed)
    Write-Host "[2/2] Building Go backend (release, embeds agent)..." -ForegroundColor Cyan
    go build -tags release -ldflags="-s -w" -o $BACKEND_RELEASE .
    Write-Host "  -> $BACKEND_RELEASE" -ForegroundColor Green

    # 3. Copy backend to binaries/ for desktop embedding
    if (-not (Test-Path $DESKTOP_BINARIES)) { New-Item -ItemType Directory $DESKTOP_BINARIES | Out-Null }
    Copy-Item -Force $BACKEND_RELEASE $BACKEND_EMBED
    Write-Host "  -> $BACKEND_EMBED" -ForegroundColor Green
}

function Release-Desktop {
    # Ensure server release is done first (backend binary for include_bytes!)
    if (-not (Test-Path $BACKEND_EMBED)) {
        Write-Host "Backend binary not found, building server first..." -ForegroundColor Yellow
        Release-Server
    }

    Write-Host "Building Tauri desktop (release)..." -ForegroundColor Cyan
    Ensure-BinDir
    Push-Location desktop
    npm run build
    Pop-Location
    Ensure-CargoPath
    cargo build --release --manifest-path $DESKTOP_SRC\Cargo.toml
    Copy-Item -Force "$DESKTOP_SRC\target\release\iamhuman-desktop.exe" $DESKTOP_RELEASE
    # Copy frontend dist next to exe so iamhuman:// protocol can find it at runtime
    $BIN_DIST = "$BIN_DIR\dist"
    if (Test-Path $BIN_DIST) { Remove-Item -Recurse -Force $BIN_DIST }
    Copy-Item -Recurse -Force "desktop\dist" $BIN_DIST
    Write-Host "  -> $DESKTOP_RELEASE" -ForegroundColor Green
    Write-Host "  -> $BIN_DIST" -ForegroundColor Green
}

# ═══════════════════════════════════════════════════════════════
# Clean
# ═══════════════════════════════════════════════════════════════

function Clean {
    Remove-Item -Recurse -Force $BIN_DIR -ErrorAction SilentlyContinue
    Remove-Item -Recurse -Force robot\target -ErrorAction SilentlyContinue
    Remove-Item -Recurse -Force $DESKTOP_SRC\target -ErrorAction SilentlyContinue
    Remove-Item -Recurse -Force agents -ErrorAction SilentlyContinue
    # Legacy root-level binaries
    Remove-Item -Force iamhuman.exe, iamhuman-agent.exe -ErrorAction SilentlyContinue
    Write-Host "Cleaned." -ForegroundColor Green
}

# ═══════════════════════════════════════════════════════════════
# Dispatch
# ═══════════════════════════════════════════════════════════════

switch ($cmd) {
    "build"          { Build-Backend }
    "build-agent"    { Build-Agent }
    "build-desktop"  { Build-Desktop }
    "all"            { Build-Agent; Build-Desktop; Build-Backend }
    "clean"          { Clean }
    "run"            {
        Build-Agent
        Build-Backend
        Write-Host "Starting IAmHuman on http://localhost:8080 ..." -ForegroundColor Cyan
        & $BACKEND_DEBUG
    }
    "dev-backend"    {
        Build-Agent
        Write-Host "Starting Go backend on http://localhost:8080 ..." -ForegroundColor Cyan
        go run . --port 8080
    }
    "dev-desktop"    {
        Ensure-CargoPath
        Write-Host "Starting Tauri desktop (backend must be running on :8080)" -ForegroundColor Cyan
        Push-Location desktop
        npm install --silent
        npx tauri dev
        Pop-Location
    }
    "release:server"  { Release-Server }
    "release:desktop" { Release-Desktop }
    default {
        Write-Host "Unknown command: $cmd" -ForegroundColor Red
        Write-Host "Usage: .\make.ps1 [dev-backend|dev-desktop|build|build-agent|build-desktop|release:server|release:desktop|clean|run]"
    }
}
