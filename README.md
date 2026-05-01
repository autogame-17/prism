# Prism

一个把 [one-hub](https://github.com/MartialBE/one-api) LLM 网关核心内嵌到桌面端的跨平台 App，基于 Wails v2 (Go + React)，并打包了 Cloudflare Tunnel。

一句话：**在本地跑一个统一 OpenAI 兼容接口的多模型网关，顺手把它临时公网化**，Cursor / Cline / Cherry Studio 这类工具只要指一个 URL、贴一个 token 就能用上 Claude、Gemini、Bedrock、DeepSeek 等 30+ 家模型。

---

## 能力概览

- **内嵌 one-hub 核心**：37 家 provider（OpenAI、Anthropic、Gemini、Bedrock、Vertex AI、DeepSeek、Groq、Zhipu、Moonshot ……），原封的 OpenAI 兼容路由。
- **渠道 / Token / 日志 CRUD**：原生 React UI（shadcn/ui + Tailwind），不依赖 one-hub 原版 Web 静态页。
- **Cloudflare Tunnel 一键公网化**：打包 `cloudflared`，Dashboard 点一下拿到 `*.trycloudflare.com` 临时域名，无需 Cloudflare 账号。
- **Request / Response 全文捕获（Traces）**：中间件级别原样记录上下行 body（包括 SSE 流），落到 SQLite。排查 provider 报错非常好用。
- **macOS 菜单栏驻留**：原生 `NSStatusItem`（CGO 实现，不依赖第三方 systray 库），关闭窗口不杀进程，菜单里直接启停 Tunnel / 复制 URL。
- **毛玻璃 UI**：`TitleBarHiddenInset` + `WindowIsTranslucent`，深浅主题适配。设计语言见 [`DESIGN.md`](./DESIGN.md)。
- **中英双语**。

---

## 开发环境启动

依赖：

- Go 1.24+
- Node 20+，推荐 `pnpm` 9+
- Wails CLI：`go install github.com/wailsapp/wails/v2/cmd/wails@latest`
- macOS：Xcode Command Line Tools（用于 CGO 链接 Cocoa，给菜单栏图标用）

首次拉下来要先抓一次 `cloudflared` 二进制（不入库）：

```bash
./scripts/fetch_cloudflared.sh
```

然后：

```bash
wails dev
```

首次启动会生成每用户配置目录：

- macOS：`~/Library/Application Support/Prism/`
- Windows：`%APPDATA%/Prism/`
- Linux：`~/.config/Prism/`

里面是：

- `prism.yaml`：运行时配置（secrets 首次启动自动生成）
- `prism.db`：SQLite（channels / tokens / logs / options / prism_traces）
- `logs/`：滚动日志

---

## 首次使用 5 步

1. 启动 Prism，欢迎向导会讲清楚下面四步。
2. **Channels → New**：选 provider（如 Anthropic），贴 API Key。如果是 Bedrock，格式是 `region|AccessKeyID|SecretAccessKey`；被墙的 provider（Gemini / Anthropic 直连）记得在 "Outbound proxy" 里填本机代理，比如 `http://127.0.0.1:7890`。
3. **Tokens → New**：起个名、选 "Unlimited quota"（单用户场景推荐）。
4. **Dashboard → Start tunnel**：Prism 启动 `cloudflared`，几秒后给出一个公网 URL。
5. **Tokens** 列表里点 snippet 图标，选 Cursor / Cline / Cherry Studio，把 JSON 贴到对端配置。

---

## 生产构建

当前 macOS 构建走 Wails 原生命令：

```bash
# 先确保前端依赖和 cloudflared 二进制到位
cd frontend && pnpm install && cd ..
./scripts/fetch_cloudflared.sh

# 构建
wails build -m                          # 标准构建
wails build -platform darwin/universal  # 通用二进制（Intel + Apple Silicon）
```

产物在 `build/bin/Prism.app`，可以直接 `cp -R build/bin/Prism.app /Applications/`。

Windows / Linux 的打包脚本在 `scripts/build_appimage.sh` 等位置，目前手工维护，CI matrix 未启用。

---

## 目录结构

```
prism-desktop/
  main.go                     Wails 入口；macOS NSStatusItem 菜单栏挂载点
  app.go                      应用生命周期（Startup / Show / Hide / BeforeClose）
  app_menu.go                 Wails 应用菜单
  backend/
    embed/                    one-hub 核心启动序列（in-process）
    bindings/                 暴露给 React 的方法（channels/tokens/logs/tunnel/traces/settings）
    tunnel/                   cloudflared 子进程管理
    tray/                     macOS 菜单栏（CGO + Objective-C）
    fakectx/                  给 one-hub 原版 controller 用的 gin.Context stub
    prism/
      trace/                  prism_traces 表和 Gorm 模型
      capture/                非破坏式抓取上下行 body 的 gin middleware
    core/                     vendored one-hub 源码（有 patch，见下）
  frontend/                   React 18 + Vite + shadcn/ui
  resources/cloudflared/      各平台 cloudflared 二进制位置（二进制本身不入库）
  scripts/                    fetch_cloudflared / sync_upstream / build_appimage
  build/                      Wails 构建输出 + 平台模板 + 图标
  DESIGN.md                   UI 设计语言
  AGENTS.md                   项目与代码约定
```

---

## 相对 one-hub 的改动

这些是直接在 `backend/core/` 里的源码改动（不是 monkey patch）：

- `core/providers/claude/chat.go`：修复 Cursor 这类客户端把 Anthropic 格式 tools 发到 OpenAI 兼容入口时 `tools.0.custom.name: Field required` 的问题。
- `core/providers/gemini/chat.go`：`system` prompt 在消息里或顶层都能正确聚合。
- `core/common/gin.go`：`ErrorWrapper` 网络层错误不再被 "请求上游地址失败" 吞掉，原始 `dial tcp …` 会一起写到响应里、进 Traces。
- `core/providers/bedrock/*`：配合 Anthropic tool 协议修复。
- 新增 `backend/prism/capture/middleware.go`：非破坏式 tee `http.Request.Body` / `gin.ResponseWriter`，流式 SSE 也照抓不误，落库走 `context.Background()` 避免被 client cancel 截断。

`scripts/sync_upstream.sh` 用来拉 upstream，手动 three-way merge 后再提交。

---

## License

Prism 内嵌了 one-hub（GPL-3.0），本仓库继承同一许可证。
