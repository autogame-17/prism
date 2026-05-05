# Prism

[![License: AGPL-3.0](https://img.shields.io/badge/License-AGPL--3.0-blue.svg)](https://www.gnu.org/licenses/agpl-3.0)
[![Platform: macOS](https://img.shields.io/badge/platform-macOS-lightgrey.svg)](https://github.com/autogame-17/prism/releases)
[![Go 1.24+](https://img.shields.io/badge/Go-1.24%2B-00ADD8.svg)](https://go.dev/)
[![Wails v2](https://img.shields.io/badge/Wails-v2-DF0000.svg)](https://wails.io/)

> 一台机器、一键启动、一个网址。把 Cursor / Cline / Cherry Studio 接到本地多模型网关。

Prism 是一个跨平台桌面 App（Wails v2 + Go + React），把 [one-hub](https://github.com/MartialBE/one-api) LLM 网关核心整个内嵌进进程内，再打包了 Cloudflare Tunnel —— 启动后你会得到：

- 一个 OpenAI 兼容的 HTTP 入口（本地或临时公网）
- 后面挂着 37 家 provider：OpenAI / Anthropic / Gemini / Bedrock / Vertex AI / DeepSeek / Groq / Zhipu / Moonshot……
- 原生 React UI 管渠道、Token、日志、请求 traces
- 一键复制配置 snippet 直接贴进 Cursor

整个东西是一个 single binary，开箱即用。

---

## 下载

最新发布版：[Releases](https://github.com/autogame-17/prism/releases)

- macOS universal (Intel + Apple Silicon)：`Prism-vX.Y.Z-darwin-universal.dmg`
- 校验：`SHA256SUMS.txt`

未签名 / 未公证。首次启动如果系统拦截：

```bash
xattr -dr com.apple.quarantine /Applications/Prism.app
```

Windows / Linux 包正在路上。

---

## 5 步上手

1. 启动 Prism，欢迎页会带你过一遍。
2. **Channels → New**：选 provider（如 Anthropic），贴 API Key。
   - Bedrock 的 Key 格式：`region|AccessKeyID|SecretAccessKey`
   - 被墙的 provider（Gemini / Anthropic 直连）：在 "Outbound proxy" 填本机代理，比如 `http://127.0.0.1:7890`
3. **Tokens → New**：起个名，选 "Unlimited quota"（单用户场景推荐）。
4. **Dashboard → Start tunnel**：Prism 启动 cloudflared，几秒后给出公网 URL。
5. 在 Tokens 列表点 snippet 图标，选 Cursor / Cline / Cherry Studio，把 JSON 贴进对端配置。

---

## 主要能力

- **37 家 provider 一统**：OpenAI 兼容入口背后是 vendored 的 one-hub，路由几乎零改动。
- **Cloudflare Tunnel 一键公网**：打包了 cloudflared 二进制，零账号、`*.trycloudflare.com` 临时域名，开关在 Dashboard。
- **Request / Response 全文捕获 (Traces)**：中间件级别 tee 上下行 body（含 SSE 流），落 SQLite。排查 provider 报错首选。
- **macOS 菜单栏驻留**：原生 NSStatusItem（CGO 实现），关窗不杀进程，菜单里直接启停 Tunnel / 复制 URL。
- **毛玻璃 UI**：`TitleBarHiddenInset` + `WindowIsTranslucent`，深浅主题适配。
- **中英双语**。

---

## 从源码构建

依赖：

- Go 1.24+
- Node 20+，推荐 `pnpm` 9+
- Wails CLI：`go install github.com/wailsapp/wails/v2/cmd/wails@latest`
- macOS：Xcode Command Line Tools（CGO 链接 Cocoa 用，给菜单栏图标）

首次拉下来要先抓 `cloudflared` 二进制（不入库）：

```bash
./scripts/fetch_cloudflared.sh
```

然后：

```bash
wails dev                                # 开发模式
wails build -platform darwin/universal   # 生产构建（universal）
```

产物在 `build/bin/Prism.app`。

首次启动会生成每用户配置目录：

- macOS：`~/Library/Application Support/Prism/`
- Windows：`%APPDATA%/Prism/`
- Linux：`~/.config/Prism/`

里面是 `prism.yaml`（运行时配置，secrets 自动生成）、`prism.db`（SQLite）、`logs/`。

---

## 相对 one-hub 的修改

直接在 `backend/core/` 里的源码改动：

- `core/providers/claude/chat.go`：修复 Cursor 这类客户端把 Anthropic 格式 tools 发到 OpenAI 兼容入口时 `tools.0.custom.name: Field required` 的问题。
- `core/providers/gemini/chat.go`：`system` prompt 在消息里或顶层都能正确聚合。
- `core/common/gin.go`：`ErrorWrapper` 网络层错误不再被 "请求上游地址失败" 吞掉，原始 `dial tcp …` 会一并写到响应里、进 Traces。
- `core/providers/bedrock/*`：配合 Anthropic tool 协议修复。
- 新增 `backend/prism/capture/middleware.go`：非破坏式 tee `http.Request.Body` 与 `gin.ResponseWriter`，流式 SSE 也照抓不误。

---

## 贡献

欢迎 Issue / PR。详细约定见 [`CONTRIBUTING.md`](./CONTRIBUTING.md)。

报安全问题：见 [`SECURITY.md`](./SECURITY.md)。

---

## License

Prism 采用 [**AGPL-3.0-or-later**](./LICENSE)。

为什么是 AGPL：Prism 是个网络服务（暴露 OpenAI 兼容 API 给 Cursor 等客户端调用），AGPL-3.0 是为这种"网络可见"形态而生的——任何在网络上提供服务的修改版本必须把对应源码同样开放给最终用户。这条款也是内嵌的 [one-hub](https://github.com/MartialBE/one-api)（GPL-3.0）允许下游在采用更严格 copyleft 的前提下重新发布的合理选择。
