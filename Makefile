.PHONY: dev build build-all clean fetch-cloudflared install run help \
        build-darwin-arm64 build-darwin-amd64 build-darwin-universal \
        build-linux-arm64 build-linux-amd64 \
        build-windows-amd64 build-windows-arm64 \
        package package-all

# 默认代理配置，按需修改
PROXY     ?= http://127.0.0.1:7890
GOPROXY   ?= https://goproxy.io,direct
BUILD_DIR := build/bin
DIST_DIR  := dist
APP_NAME  := Prism
VERSION   ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)

# 带代理的环境变量
define WITH_PROXY
export https_proxy=$(PROXY) http_proxy=$(PROXY) all_proxy=$(PROXY) GOPROXY=$(GOPROXY)
endef

# ─── 开发 ───

dev: ## 启动开发环境（热重载）
	$(WITH_PROXY) && wails dev

# ─── 构建：macOS ───

build: build-darwin-arm64 ## 默认构建当前平台（macOS arm64）

build-darwin-arm64: fetch-cloudflared ## macOS arm64 (Apple Silicon)
	$(WITH_PROXY) && wails build -platform darwin/arm64
	@echo "✅ $(BUILD_DIR)/$(APP_NAME).app"

build-darwin-amd64: fetch-cloudflared ## macOS amd64 (Intel)
	$(WITH_PROXY) && wails build -platform darwin/amd64
	@echo "✅ $(BUILD_DIR)/$(APP_NAME).app"

build-darwin-universal: fetch-cloudflared ## macOS universal (Intel + Apple Silicon)
	$(WITH_PROXY) && wails build -platform darwin/universal
	@echo "✅ $(BUILD_DIR)/$(APP_NAME).app"

# ─── 构建：Linux ───

build-linux-amd64: fetch-cloudflared ## Linux amd64
	$(WITH_PROXY) && wails build -platform linux/amd64 -tags webkit2_41
	@echo "✅ $(BUILD_DIR)/$(APP_NAME)"

build-linux-arm64: fetch-cloudflared ## Linux arm64
	$(WITH_PROXY) && wails build -platform linux/arm64 -tags webkit2_41
	@echo "✅ $(BUILD_DIR)/$(APP_NAME)"

# ─── 构建：Windows ───

build-windows-amd64: fetch-cloudflared ## Windows amd64 (NSIS 安装包)
	$(WITH_PROXY) && wails build -platform windows/amd64 -nsis
	@echo "✅ $(BUILD_DIR)/$(APP_NAME).exe + $(BUILD_DIR)/$(APP_NAME)-installer.exe"

build-windows-arm64: fetch-cloudflared ## Windows arm64
	$(WITH_PROXY) && wails build -platform windows/arm64
	@echo "✅ $(BUILD_DIR)/$(APP_NAME).exe"

# ─── 全量构建 ───

PLATFORMS := darwin-arm64 darwin-amd64 darwin-universal linux-amd64 linux-arm64 windows-amd64 windows-arm64

build-all: fetch-cloudflared ## 构建全平台全架构
	@for target in $(PLATFORMS); do \
		echo ""; \
		echo "━━━ 构建 $$target ━━━"; \
		$(MAKE) build-$$target || exit 1; \
	done
	@echo ""
	@echo "✅ 全平台构建完成"

# ─── 打包发布 ───

package: package-darwin-arm64 ## 默认打包 macOS arm64

package-darwin-arm64: build-darwin-arm64 ## 打包 macOS arm64 zip
	@mkdir -p $(DIST_DIR)
	cd $(BUILD_DIR) && zip -r -q ../$(DIST_DIR)/$(APP_NAME)-$(VERSION)-darwin-arm64.zip $(APP_NAME).app
	@echo "✅ $(DIST_DIR)/$(APP_NAME)-$(VERSION)-darwin-arm64.zip"

package-darwin-amd64: build-darwin-amd64 ## 打包 macOS amd64 zip
	@mkdir -p $(DIST_DIR)
	cd $(BUILD_DIR) && zip -r -q ../$(DIST_DIR)/$(APP_NAME)-$(VERSION)-darwin-amd64.zip $(APP_NAME).app
	@echo "✅ $(DIST_DIR)/$(APP_NAME)-$(VERSION)-darwin-amd64.zip"

package-darwin-universal: build-darwin-universal ## 打包 macOS universal zip
	@mkdir -p $(DIST_DIR)
	cd $(BUILD_DIR) && zip -r -q ../$(DIST_DIR)/$(APP_NAME)-$(VERSION)-darwin-universal.zip $(APP_NAME).app
	@echo "✅ $(DIST_DIR)/$(APP_NAME)-$(VERSION)-darwin-universal.zip"

package-linux-amd64: build-linux-amd64 ## 打包 Linux amd64 tar.gz
	@mkdir -p $(DIST_DIR)
	tar -czf $(DIST_DIR)/$(APP_NAME)-$(VERSION)-linux-amd64.tar.gz -C $(BUILD_DIR) $(APP_NAME)
	@echo "✅ $(DIST_DIR)/$(APP_NAME)-$(VERSION)-linux-amd64.tar.gz"

package-linux-arm64: build-linux-arm64 ## 打包 Linux arm64 tar.gz
	@mkdir -p $(DIST_DIR)
	tar -czf $(DIST_DIR)/$(APP_NAME)-$(VERSION)-linux-arm64.tar.gz -C $(BUILD_DIR) $(APP_NAME)
	@echo "✅ $(DIST_DIR)/$(APP_NAME)-$(VERSION)-linux-arm64.tar.gz"

package-windows-amd64: build-windows-amd64 ## 打包 Windows amd64 zip
	@mkdir -p $(DIST_DIR)
	cd $(BUILD_DIR) && zip -r -q ../$(DIST_DIR)/$(APP_NAME)-$(VERSION)-windows-amd64.zip $(APP_NAME).exe
	@echo "✅ $(DIST_DIR)/$(APP_NAME)-$(VERSION)-windows-amd64.zip"

package-windows-arm64: build-windows-arm64 ## 打包 Windows arm64 zip
	@mkdir -p $(DIST_DIR)
	cd $(BUILD_DIR) && zip -r -q ../$(DIST_DIR)/$(APP_NAME)-$(VERSION)-windows-arm64.zip $(APP_NAME).exe
	@echo "✅ $(DIST_DIR)/$(APP_NAME)-$(VERSION)-windows-arm64.zip"

package-all: ## 全平台打包
	@rm -rf $(DIST_DIR)
	@for target in darwin-arm64 darwin-amd64 darwin-universal linux-amd64 linux-arm64 windows-amd64 windows-arm64; do \
		echo ""; \
		echo "━━━ 打包 $$target ━━━"; \
		$(MAKE) package-$$target || exit 1; \
	done
	@echo ""
	@echo "✅ 全平台打包完成，产物在 $(DIST_DIR)/"
	@ls -lh $(DIST_DIR)/

# ─── 依赖 ───

fetch-cloudflared: ## 下载 cloudflared 二进制
	$(WITH_PROXY) && bash ./scripts/fetch_cloudflared.sh

deps-frontend: ## 安装前端依赖
	cd frontend && pnpm install

deps-go: ## 下载 Go 依赖
	$(WITH_PROXY) && go mod download

deps: deps-go deps-frontend fetch-cloudflared ## 安装所有依赖

# ─── 安装 & 运行 ───

install: build-darwin-arm64 ## 构建并安装到 /Applications
	@rm -rf /Applications/$(APP_NAME).app
	cp -R $(BUILD_DIR)/$(APP_NAME).app /Applications/
	xattr -cr /Applications/$(APP_NAME).app
	@echo "✅ 已安装到 /Applications/$(APP_NAME).app"

run: install ## 构建安装并启动
	open /Applications/$(APP_NAME).app

# ─── 清理 ───

clean: ## 清理构建产物和发布包
	rm -rf $(BUILD_DIR) $(DIST_DIR)
	@echo "✅ 已清理 $(BUILD_DIR) $(DIST_DIR)"

# ─── 其他 ───

sync-upstream: ## 同步 one-hub 上游代码
	bash ./scripts/sync_upstream.sh

help: ## 显示帮助
	@python3 -c 'import re; [print(f"\033[36m{m.group(1):<28s}\033[0m {m.group(2)}") for line in open("Makefile") if (m := re.match(r"^([a-zA-Z0-9_-]+):.*?## (.+)$$", line))]'
