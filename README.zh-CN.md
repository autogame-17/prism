# Prism

把一个本地 LLM 网关塞进 macOS / Windows / Linux 桌面端，对外吐出一个 OpenAI 兼容的 HTTP 入口。Cursor、Cline、Cherry Studio 这些只认 `/v1/chat/completions` 的客户端，配一个 URL 就能切到 37 家 provider。

English version: [README.md](./README.md).

![Prism Dashboard](./docs/screenshots/hero.png)

[![License: AGPL-3.0](https://img.shields.io/badge/License-AGPL--3.0-blue.svg)](https://www.gnu.org/licenses/agpl-3.0)
[![Releases](https://img.shields.io/github/v/release/autogame-17/prism)](https://github.com/autogame-17/prism/releases)
[![Go 1.24+](https://img.shields.io/badge/Go-1.24%2B-00ADD8.svg)](https://go.dev/)
[![Wails v2](https://img.shields.io/badge/Wails-v2-DF0000.svg)](https://wails.io/)

## 这是什么

Prism 把 [one-hub](https://github.com/MartialBE/one-api) 网关核心当库直接 import 进 Wails (Go + React) 桌面 App。网关跑在 App 的进程里 —— 不需要 Docker，也不用单独维护一份 `docker-compose.yml`，更不用满电脑找空闲端口。app 还把 `cloudflared` 二进制按平台 embed 进去了，点一下就有 `*.trycloudflare.com` 的临时公网 URL，方便临时给同事或者远端 agent 用。

具体能用上的东西：

- 一个 OpenAI 兼容的 HTTP 入口，本地或临时公网。
- 入口背后挂着 37 家 provider：OpenAI、Anthropic、Gemini、Bedrock、Vertex AI、DeepSeek、Groq、智谱、月之暗面 ……
- 一个真正的桌面 UI，管渠道、Token、请求日志、完整的请求 / 响应 trace。
- 一个 snippet 生成器，按你选的客户端（Cursor / Cline / Cherry Studio）直接生成可贴的 JSON。

整个东西是一份 binary，不依赖任何外部服务。

## 为什么要写这个

我想让 Cursor 和 Cline 同时能用好几个 provider —— 有些得走代理才通，有些只在某些区域可用 —— 又不想再在我笔记本上常驻一个 Docker 容器。one-hub 已经把"路由 + 多 provider"这件事解决了；Prism 是把 one-hub 包成一个能当主力日常工具的桌面端：UI 不挡道、需要时一键有公网 URL、出问题的时候能直接看到 provider 真正收到的请求和它返回的原文。

provider 偶尔抽风返回个奇怪的 400 时，能不能一秒看到原 wire 上的 payload，差别非常大。

## 安装

去 [Releases](https://github.com/autogame-17/prism/releases) 下：

| 平台 | 文件 |
| --- | --- |
| macOS（Intel + Apple Silicon） | `Prism-vX.Y.Z-darwin-universal.dmg` |
| Windows amd64 | `Prism-vX.Y.Z-windows-amd64-installer.exe`（NSIS）或 `.zip`（解压即用） |
| Linux amd64 | `Prism-vX.Y.Z-linux-amd64.tar.gz` |

构建产物没签名。各平台首次启动注意：

```bash
# macOS：去掉 Gatekeeper 隔离标记
xattr -dr com.apple.quarantine /Applications/Prism.app
```

Windows 第一次会撞 SmartScreen，"More info" → "Run anyway"。Linux 需要 GTK 3 和 `libwebkit2gtk-4.1-0`，Ubuntu 22.04+ / Fedora 39+ / Debian 12+ 默认有。

## 五步上手

1. 启动 Prism。第一次会走一遍 onboarding。
2. **Channels → New** —— 选 provider，贴 API Key。
   - Bedrock 的 Key 拼成：`region|AccessKeyID|SecretAccessKey`。
   - 国内连不通的 provider（直连 Anthropic / Gemini）：在 **Outbound proxy** 里填本机代理，比如 `http://127.0.0.1:7890`。
3. **Tokens → New** —— 起个名，单人用直接选 "Unlimited quota"。
4. **Dashboard → Start tunnel** —— Prism 起 `cloudflared`，几秒后 UI 上会出现公网 URL。（不开 tunnel 也行，本机用 `http://127.0.0.1:<port>` 就够。）
5. 在 Tokens 列表点 snippet 图标，选你的客户端，把生成的 JSON 贴进 Cursor / Cline / Cherry Studio。

## 里面到底有什么

- **内嵌 one-hub**。不是另起进程。Go 二进制把 gateway 当库 import 进来，把 routes 注册到一个 `gin.Engine` 上、监听本地端口。默认 SQLite；要换 MySQL / Postgres 也能继续用（设 `SQL_DSN` 环境变量）。
- **Tunnel 不需要 Cloudflare 账号**。`cloudflared` 按平台 `go:embed`，作为子进程跑；app 解析它的日志拿到公网 URL。不需要写 tunnel config，不需要 Zero Trust 设置。
- **Traces**。一个不大的 Gin 中间件把 request body 和 response body（包括 SSE 流）tee 到一张 SQLite 表。当某 provider 返回 `tools.0.custom.name: Field required`、wrapper 又只甩你一句"Provider API error"的时候，真正的 wire payload 在 Logs → Traces 里点一下就能看到。
- **macOS 菜单栏驻留**。原生 `NSStatusItem`（CGO + Objective-C 手写，没用第三方 systray —— 那些库会和 Wails 自己的 `AppDelegate` 撞符号）。关窗时进程不退、Dock 图标隐去，菜单栏里直接 Start/Stop tunnel + Copy URL。
- **主题与 i18n**。亮 / 暗主题，中英文。

## 相对 one-hub 的修改

vendored 在 `backend/core/`。和上游 `MartialBE/one-api` 比，几处明显的 diff：

- `core/providers/claude/chat.go` —— Cursor 这种客户端会把 Anthropic 风格的 tools 发到 OpenAI 兼容入口，上游会因为 tool schema 缺 `type: "custom"` 而报 `tools.0.custom.name: Field required`。patch 修了类型字段、并把 `input_schema` 正确穿过去。
- `core/providers/gemini/chat.go` —— `system` prompt 不论是顶层字段还是消息数组形式，都会聚合到 Gemini 期待的 `systemInstruction`。
- `core/common/gin.go` —— `ErrorWrapper` 不再把底层网络错误吞掉只甩"请求上游地址失败"。原始 `dial tcp …` / `i/o timeout` 会被拼回响应体，所以 Traces 里看得到。
- `core/providers/bedrock/*` —— 配合上面 Anthropic-on-Bedrock 的 tool schema 修复。
- `backend/prism/capture/middleware.go` —— 新加的。对 `http.Request.Body` 做非破坏式 tee，加一个 `gin.ResponseWriter` proxy 处理流式响应，所有上下行都进 `prism_traces`。

如果上游 PR 把哪条修了，我这边对应的 diff 就消失。

## 自己编

```bash
# 依赖：Go 1.24+、Node 20+、pnpm 9+
go install github.com/wailsapp/wails/v2/cmd/wails@latest

# cloudflared 太大没入库，构建前先 fetch
./scripts/fetch_cloudflared.sh

wails dev                                # 热重载
wails build -platform darwin/universal   # 生产构建
wails build -platform windows/amd64 -nsis
wails build -platform linux/amd64 -tags webkit2_41
```

产物在 `build/bin/`。

每用户配置目录：

- macOS：`~/Library/Application Support/Prism/`
- Windows：`%APPDATA%/Prism/`
- Linux：`~/.config/Prism/`

里面有 `prism.yaml`（运行时配置，secrets 首次启动自动生成）、`prism.db`（SQLite）、`logs/`。

## Release 流程

push 一个 `v*.*.*` tag 会触发 `.github/workflows/release.yml`，在 macOS / Windows / Linux runner 上跑 matrix，把产物上传到对应的 GitHub Release。临时只想出某一个平台：

```bash
gh workflow run release.yml --ref main \
  -f tag=v0.1.1 \
  -f platforms=linux-amd64
```

## 贡献

欢迎 Issue / PR。约定见 [`CONTRIBUTING.md`](./CONTRIBUTING.md)。提 bug 时尽量别贴真 API key —— Prism 的 Traces 视图只在你本机，从那里截图通常比直接复制 JSON 安全。

安全相关：见 [`SECURITY.md`](./SECURITY.md)。

## License

[AGPL-3.0-or-later](./LICENSE)。

为什么用 AGPL：Prism 的形态是一个网络服务 —— 它对远端客户端讲一个 OpenAI 兼容的 HTTP 协议。AGPL 正好是为这种场景写的：你跑了改过的 Prism 还把它接在网络上，就要把对应的源码开放给它的用户。这条 copyleft 也契合内嵌的 [one-hub](https://github.com/MartialBE/one-api)（GPL-3.0）—— 它允许下游在更严格的 copyleft 下重新发布。

## 鸣谢

- [one-hub](https://github.com/MartialBE/one-api) —— 网关核心。Prism 大致等于"one-hub 去掉 docker-compose，再加一个桌面 UI"。
- [Wails](https://wails.io/) —— 让 Go 网关代码塞进原生窗口这件事不痛苦的 Go-React 桌面框架。
- [cloudflared](https://github.com/cloudflare/cloudflared) —— 公网 URL 那一段。
