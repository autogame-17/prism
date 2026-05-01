# AGENTS.md

Guidance for AI coding agents working in this repository.

## High-level architecture

Prism is a Wails v2 app (Go + React/Vite). The Go process embeds the `one-hub` core (forked under `backend/core/`) and exposes bindings to the frontend via Wails (no HTTP hop). `cloudflared` is launched as a child process with its binary embedded via `go:embed`.

```
Cursor / Cline → Cloudflare edge → local cloudflared → Gin server (embedded) → one-hub providers
                                                                   ↑
                                Wails UI (React) ⇄ Go bindings (backend/bindings/*)
```

Two communication channels to the UI:

1. Direct Wails bindings (RPC) for queries and mutations.
2. Wails events (`tunnel.url`, `tunnel.status`, `log.system`) for push/streams.

## Code map

- `main.go`, `app.go` — Wails entry, lifecycle, binding registration.
- `backend/embed/boot.go` — one-hub minimal startup sequence (replaces upstream `main.go`). Writes `prism.yaml` + generates secrets on first run.
- `backend/bindings/` — one file per surface: `channels.go`, `tokens.go`, `logs.go`, `settings.go`, `tunnel.go`, `system.go`. Add new RPC surface area here.
- `backend/tunnel/cloudflared.go` — process manager for `cloudflared tunnel --url ...`. `resolver.go` chooses between embedded, PATH, and cache paths.
- `backend/core/` — upstream `one-hub` source. Avoid editing; required patches live in `backend/core-patches/*.patch` (applied by `scripts/sync_upstream.sh`).
- `frontend/src/` — React UI. `lib/wails.ts` is the typed wrapper around generated bindings.
- `Taskfile.yml`, `.github/workflows/build.yml` — cross-platform builds.

## Conventions

- **No emojis** in code/docs output (user preference).
- Do NOT create new docs unless asked; update README/AGENTS.md in place.
- Prefer editing existing files over creating new ones.
- UI text must be localisable. Add keys to `frontend/src/lib/i18n.ts` and reference via `useI18n().t('...')`.
- All Wails bindings must return `json`-friendly structs (no `time.Time` — use `int64` unix millis). Wails' TS generator skips `time.Time`.
- For upstream patches: prefer adding a tiny new exported function to `backend/core/` and leave existing internals untouched so upstream syncs stay clean.

## Running locally

```
wails dev           # hot-reload UI + Go
wails build         # production build to build/bin
pnpm tsc -b         # type-check only
go build ./...      # backend sanity check
```

Frontend bindings are generated under `frontend/wailsjs/go/bindings/*` on each `wails build`. `frontend/src/lib/wails.ts` loads these at runtime with `import()` so the dev server runs even before the first build.

## Common pitfalls

- `wails build` runs `main.go`'s init path during binding generation, which briefly boots the embedded core and creates `prism.yaml` / `prism.db` in the user config dir. This is expected.
- `go:embed` paths are relative to the package directory. Cloudflared binaries are embedded from root `main` package (`embed_cloudflared_*.go`) because nested packages can't `go:embed` up the tree.
- When adding a binding struct containing `time.Time`, the TS generator prints `Not found: time.Time`. Convert to `int64` unix millis before returning.
- Dialog/port confusion: the embedded Gin server listens on `127.0.0.1:<random>`; the cloudflared process connects to that port; the UI talks to Go via Wails bindings (not HTTP). Do not add HTTP fetches to the UI.
- `ChannelsAPI.Test` calls `controller.TestChannelOnce` (we exported upstream's private `testChannel`). If you bump upstream, re-apply the `s/testChannel/TestChannelOnce/` change or add the patch.

## Testing a change end-to-end

1. `go build ./...`
2. `cd frontend && pnpm tsc -b`
3. `wails build -platform darwin/arm64` (or your host platform)
4. Launch `build/bin/Prism.app`, verify onboarding dialog, add a channel, start tunnel, copy snippet.

## Don't do this

- Don't revert to using `one-hub`'s HTTP API from the frontend. We intentionally went native Wails bindings.
- Don't re-introduce MJ/Suno/payment/OIDC/WeChat/Telegram paths — they were removed from `backend/core/` and `embed/boot.go` on purpose.
- Don't force-push to main or amend pushed commits.
