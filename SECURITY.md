# Security Policy

## Supported Versions

只对最新一条 minor release line 提供安全更新。旧版本不会 backport。

| Version    | Supported       |
|------------|-----------------|
| latest     | Yes             |
| < latest   | No              |

最新版见 [Releases](https://github.com/autogame-17/prism/releases)。

## Reporting a Vulnerability

请**不要**为安全漏洞开公开 GitHub Issue。请走以下任一私密渠道。

### 首选：GitHub Private Vulnerability Reporting

提交私密报告：

  https://github.com/autogame-17/prism/security/advisories/new

只有仓库 maintainer 能看到。

### 报告里请尽量包含

- 漏洞描述、影响、可能的攻击场景
- 受影响的版本与运行环境（OS、Go 版本、provider 配置等）
- 复现步骤或最小 PoC
- 任何缓解或修复建议

### 期望节奏

- **48 小时内确认收到**
- **7 天内初步评估**（严重程度、影响范围、缓解计划）
- **修复时间线**：严重问题 14 天内目标发 patch release；其他按正常发布节奏
- **披露**：协调披露。修复发布后会在 GitHub Security Advisory 公开并致谢报告者（除非要求匿名）

### Scope

In scope:

- Prism 桌面 app 主进程（Go + Wails）
- 内嵌 one-hub 核心的 Prism 改动部分（`backend/prism/*`、`backend/core/` 中的 patches）
- Cloudflare Tunnel 子进程的启停控制
- 内嵌的 SQLite store（特别是 traces 捕获涉及的请求/响应 body）

Out of scope:

- 上游 [one-hub](https://github.com/MartialBE/one-api) 自身代码（请直接报到上游）
- Cloudflare Tunnel / cloudflared 自身（报到 Cloudflare）
- 第三方 LLM provider 的 API 行为
- 用户配置错误（如把 Prism 直接暴露到公网而不加 token 鉴权）

## Safe Harbor

按本政策善意做安全研究的人受到保护。我们不会对在以下范围内行动的研究者发起法律行动：

- 仅访问自己的或经合法授权的 Prism 实例
- 不公开未修复的漏洞细节
- 不破坏数据或服务可用性，不影响其他用户
- 提供合理时间让我们修复
