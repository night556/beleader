# ═══════════════════════════════════════════════════════════════
# BeLeader Build System (macOS / Linux)
# ═══════════════════════════════════════════════════════════════
#
# Quick start:
#   make run          → 一键构建并启动，浏览器打开 http://localhost:8080
#
# Windows 用户请使用: .\make.ps1 run
#
# 产物命名：{程序}-{debug|release}-{os}-{arch}  → 统一输出到 bin/
#   bin/
#     beleader-backend-debug-darwin-amd64
#     beleader-backend-release-darwin-amd64

.PHONY: all build run dev clean release:server

# ═══════════════════════════════════════════════════════════════
# 路径
# ═══════════════════════════════════════════════════════════════

OS            := $(shell uname -s | tr '[:upper:]' '[:lower:]')
ARCH          := $(shell uname -m)
BIN_DIR       := bin

BACKEND_DEBUG   := $(BIN_DIR)/beleader-backend-debug-$(OS)-$(ARCH)
BACKEND_RELEASE := $(BIN_DIR)/beleader-backend-release-$(OS)-$(ARCH)

# ═══════════════════════════════════════════════════════════════
# 构建命令
# ═══════════════════════════════════════════════════════════════

# 构建 Go 后端 (debug)
build:
	@mkdir -p $(BIN_DIR)
	go build -o $(BACKEND_DEBUG) .

# 全部构建
all: build

# ═══════════════════════════════════════════════════════════════
# 发布命令 — 产物输出到 bin/
# ═══════════════════════════════════════════════════════════════

# 构建服务端发布版 → bin/beleader-backend-release-{os}-{arch}
release:server:
	@mkdir -p $(BIN_DIR)
	go build -tags release -ldflags="-s -w" -o $(BACKEND_RELEASE) .

# ═══════════════════════════════════════════════════════════════
# 运行命令
# ═══════════════════════════════════════════════════════════════

# 一键构建 + 启动
run: build
	@echo "Starting BeLeader backend on http://localhost:8080 ..."
	./$(BACKEND_DEBUG)

# 同 run，显式指定端口
run-web: build
	@echo "Starting BeLeader (web mode) -> http://localhost:8080"
	./$(BACKEND_DEBUG) --port 8080

# ═══════════════════════════════════════════════════════════════
# 开发命令
# ═══════════════════════════════════════════════════════════════

# 启动 Go 后端 (go run 方式)
dev-backend:
	@echo "Starting Go backend on http://localhost:8080"
	go run . --port 8080

dev: dev-backend

# ═══════════════════════════════════════════════════════════════
# 清理
# ═══════════════════════════════════════════════════════════════

clean:
	rm -rf $(BIN_DIR)
	rm -rf agents
