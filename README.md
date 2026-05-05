# Prism (private)

This is the private dev mirror. The public, user-facing READMEs live in:

- [`README.public.md`](./README.public.md) — English. Renamed to `README.md` when published to the public repo.
- [`README.zh-CN.md`](./README.zh-CN.md) — 中文。Mirrored as-is.

Public repo: https://github.com/autogame-17/prism

## Private-only notes

### Repo layout

```
prism-desktop/
  main.go                     Wails entry; macOS NSStatusItem menu-bar mount point
  app.go                      app lifecycle (Startup / Show / Hide / BeforeClose)
  app_menu.go                 Wails application menu
  backend/
    embed/                    one-hub core startup (in-process)
    bindings/                 methods exposed to React (channels/tokens/logs/tunnel/traces/settings)
    tunnel/                   cloudflared child-process manager
    tray/                     macOS menu bar (CGO + Objective-C)
    fakectx/                  gin.Context stub used by upstream one-hub controllers
    prism/
      trace/                  prism_traces gorm model
      capture/                non-destructive request/response body tee middleware
    core/                     vendored one-hub source (with patches — see public README)
  frontend/                   React 18 + Vite + shadcn/ui
  resources/cloudflared/      per-platform cloudflared binaries (not in git, see fetch script)
  scripts/                    fetch_cloudflared / sync_upstream / build_public / publish_public
  build/                      Wails build outputs + platform templates + icons
  docs/                       screenshots, internal design notes
  DESIGN.md                   UI language
  AGENTS.md                   project + code conventions for AI agents
  public.manifest.json        rules for mirroring private → public repo
```

### Public mirror workflow

```bash
# stage private → public tree under dist-public/
./scripts/build_public.sh

# secret-scan, then squash + force-push to autogame-17/prism main
./scripts/publish_public.sh
```

`public.manifest.json` controls what's excluded / renamed / overlaid. The default is **include every git-tracked file**, so new code flows out unless you explicitly exclude it.

### Release workflow

```bash
git tag v0.1.1
git push origin v0.1.1   # triggers .github/workflows/release.yml on the public repo

# or manually, single-platform:
gh workflow run release.yml --repo autogame-17/prism --ref main \
  -f tag=v0.1.1 -f platforms=linux-amd64
```

### Conventions for AI agents

See [`AGENTS.md`](./AGENTS.md). It's the single source of truth for tone, guardrails (no emoji except the explicit one), commit format, and the mandatory post-task review.

## License

[AGPL-3.0-or-later](./LICENSE). Rationale in the public README.
