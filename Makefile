# ═══════════════════════════════════════════════════════════════
# IAmHuman Build System (macOS / Linux)
# ═══════════════════════════════════════════════════════════════
#
# Quick start:
#   make run          → 一键构建并启动，浏览器打开 http://localhost:8080
#   make dev-desktop  → 启动 Tauri 桌面应用（需先运行 make dev-backend）
#
# Windows 用户请使用: .\make.ps1 run
#
# 产物命名：{程序}-{debug|release}-{os}-{arch}  → 统一输出到 bin/
#   bin/
#     iamhuman-agent-release-darwin-amd64
#     iamhuman-backend-debug-darwin-amd64
#     iamhuman-backend-release-darwin-amd64
#     iamhuman-desktop-release-darwin-amd64

.PHONY: all build run dev clean build-agent build-desktop release:server release:desktop

# ═══════════════════════════════════════════════════════════════
# 路径
# ═══════════════════════════════════════════════════════════════

OS            := $(shell uname -s | tr '[:upper:]' '[:lower:]')
ARCH          := $(shell uname -m)
BIN_DIR       := bin
DESKTOP_SRC   := desktop/src-tauri
DESKTOP_BINARIES := $(DESKTOP_SRC)/binaries
AGENT_SRC     := robot/Cargo.toml
AGENT_BUILT   := robot/target/release/iamhuman-agent

ifeq ($(OS),darwin)
	AGENT_BUILT := robot/target/release/iamhuman-agent
	DESKTOP_EXT :=
	BACKEND_EXT :=
else
	AGENT_BUILT := robot/target/release/iamhuman-agent
	DESKTOP_EXT :=
	BACKEND_EXT :=
endif

AGENT_RELEASE   := $(BIN_DIR)/iamhuman-agent-release
BACKEND_DEBUG   := $(BIN_DIR)/iamhuman-backend-debug-$(OS)-$(ARCH)
BACKEND_RELEASE := $(BIN_DIR)/iamhuman-backend-release-$(OS)-$(ARCH)
DESKTOP_RELEASE := $(BIN_DIR)/iamhuman-desktop-release-$(OS)-$(ARCH)
BACKEND_EMBED   := $(DESKTOP_BINARIES)/iamhuman-backend-release

# ═══════════════════════════════════════════════════════════════
# 构建命令
# ═══════════════════════════════════════════════════════════════

# 构建 Go 后端 (debug)
build:
	@mkdir -p $(BIN_DIR)
	go build -o $(BACKEND_DEBUG) .

# 构建 Rust agent (release)
build-agent:
	@mkdir -p $(BIN_DIR)
	@PATH="$$HOME/.cargo/bin:$$PATH" cargo build --release --manifest-path $(AGENT_SRC)
	@cp -f $(AGENT_BUILT) $(AGENT_RELEASE)

# 构建 Tauri 桌面 (release)
build-desktop:
	@mkdir -p $(BIN_DIR)
	cd desktop && npm install --silent && npm run build
	@PATH="$$HOME/.cargo/bin:$$PATH" cargo build --release --manifest-path $(DESKTOP_SRC)/Cargo.toml
	@cp -f $(DESKTOP_SRC)/target/release/iamhuman-desktop$(DESKTOP_EXT) $(DESKTOP_RELEASE)

# 全部构建
all: build-agent build-desktop build

# ═══════════════════════════════════════════════════════════════
# 发布命令 — 产物输出到 bin/
# ═══════════════════════════════════════════════════════════════

# 构建服务端发布版 → bin/iamhuman-backend-release-{os}-{arch}
# 后端内嵌 agent（go:embed），同时复制到 binaries/ 供桌面端打包
release:server:
	@mkdir -p $(BIN_DIR)
	@PATH="$$HOME/.cargo/bin:$$PATH" cargo build --release --manifest-path $(AGENT_SRC)
	@cp -f $(AGENT_BUILT) $(AGENT_RELEASE)
	go build -tags release -ldflags="-s -w" -o $(BACKEND_RELEASE) .
	@mkdir -p $(DESKTOP_BINARIES)
	@cp -f $(BACKEND_RELEASE) $(BACKEND_EMBED)

# 构建桌面发布版 → bin/iamhuman-desktop-release-{os}-{arch}
# 前提：已执行 make release:server（确保后端二进制在 binaries/ 中供 include_bytes!）
release:desktop:
	@if [ ! -f "$(BACKEND_EMBED)" ]; then $(MAKE) release:server; fi
	cd desktop && npm run build
	@PATH="$$HOME/.cargo/bin:$$PATH" cargo build --release --manifest-path $(DESKTOP_SRC)/Cargo.toml
	@mkdir -p $(BIN_DIR)
	@cp -f $(DESKTOP_SRC)/target/release/iamhuman-desktop$(DESKTOP_EXT) $(DESKTOP_RELEASE)
	@cp -rf desktop/dist $(BIN_DIR)/dist

# ═══════════════════════════════════════════════════════════════
# 运行命令
# ═══════════════════════════════════════════════════════════════

# 一键构建 + 启动
run: build-agent build
	@echo "Starting IAmHuman backend on http://localhost:8080 ..."
	./$(BACKEND_DEBUG)

# 同 run，显式指定端口
run-web: build-agent build
	@echo "Starting IAmHuman (web mode) -> http://localhost:8080"
	./$(BACKEND_DEBUG) --port 8080

# ═══════════════════════════════════════════════════════════════
# 开发命令
# ═══════════════════════════════════════════════════════════════

# 启动 Go 后端 (go run 方式)
dev-backend: build-agent
	@echo "Starting Go backend on http://localhost:8080"
	go run . --port 8080

# 启动 Tauri 桌面应用 (开发模式)
# 前提：Go 后端已在 :8080 运行 (先执行 make dev-backend)
dev-desktop:
	@echo "Starting Tauri desktop (Go backend must be running on :8080)"
	cd desktop && npm install --silent && npx tauri dev

# ═══════════════════════════════════════════════════════════════
# 清理
# ═══════════════════════════════════════════════════════════════

clean:
	rm -rf $(BIN_DIR)
	rm -rf robot/target
	rm -rf $(DESKTOP_SRC)/target
	rm -rf agents
