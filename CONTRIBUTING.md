# Contributing to Prism

谢谢看到这里。Prism 欢迎 Issue 和 PR。下面是几条让 review 顺畅的约定。

## 报告 Bug / 提交 Issue

1. 确认你用的是最新 release（[Releases](https://github.com/autogame-17/prism/releases)）。
2. 搜一下 [Issues](https://github.com/autogame-17/prism/issues) 看是否已经有人报过。
3. 新开 Issue 时尽量带：操作系统 + 版本、Prism 版本、复现步骤、期望 vs 实际、相关 trace（**自动脱敏**: 提交前请确保不包含 API key 或私人对话内容）。

## 提交 PR

- **小步、聚焦**：一次 PR 解决一件事。多功能就拆多个 PR。
- **改了行为就更新文档**：改 UI 就同步 i18n（`frontend/src/lib/i18n.ts`）；改 binding 就更新 README 相关段落。
- **不引入 emoji**（除了文档里的 `🧬` 这种已有占位）。
- **Wails 绑定返回值用 JSON 友好类型**：避免 `time.Time`，统一用 `int64` Unix millis。
- **i18n key 必须中英双语都加**。
- 提交前本地跑：
  ```
  go build ./...
  cd frontend && pnpm tsc -b && cd ..
  wails build       # 至少能编过
  ```

## 代码风格

- Go：`gofmt` / `go vet` 默认；新增 binding 写在 `backend/bindings/<surface>.go`。
- TS / React：跟项目现有 `tsconfig` + Tailwind 设计语言（见 [`DESIGN.md`](./DESIGN.md)）走，不引入新 UI 库。
- Commit message：第一行 50 字以内简述意图；正文讲"为什么"而不是"做了什么"。

## 涉及 one-hub 上游

`backend/core/` 是 vendored 的 [one-hub](https://github.com/MartialBE/one-api)。要改上游代码请：

1. 优先在上游代码里做最小补丁，写清楚 commit message 标记 "**[upstream patch]**"。
2. 如果是新功能，考虑放在 `backend/prism/`（Prism own code）而不是改 `backend/core/`，方便后续 `scripts/sync_upstream.sh` 拉新版本。

## 安全问题

请**不要**用 GitHub 公开 issue 报安全漏洞，见 [`SECURITY.md`](./SECURITY.md)。

## License

Prism 是 AGPL-3.0-or-later。提交 PR 即代表你同意你的贡献以同样的许可证发布。
