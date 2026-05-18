# LocalCLI Gateway

Prism v0.1.5 adds a new channel type, **"LocalCLI Gateway"** (`type=57`), that
turns any locally-installed LLM CLI — Claude Code, Gemini CLI, OpenAI Codex,
Qwen Code, or a generic shell wrapper — into an OpenAI-compatible upstream
that flows through Prism's existing routing, quota, logging, and trace UI.

LocalCLI itself is a thin OpenAI-compatible client. The actual CLI invocation
happens inside a small Python sidecar called
[`cli2api`](https://github.com/anoxis/CLI2API). Prism talks to the sidecar
over plain HTTP, exactly like it would talk to OpenAI.

## Architecture

```
Cursor / Cline / curl ──► Prism (127.0.0.1:3000)
                                  │
                                  └── LocalCLI channel (type=57)
                                          │
                                          ▼
                          cli2api sidecar (127.0.0.1:8000)
                                          │
                                          ├── spawns `claude  -p ...`
                                          ├── spawns `gemini  -p ...`
                                          ├── spawns `codex   -p ...`
                                          ├── spawns `qwen    -p ...`
                                          └── spawns any generic shell command
```

Why a separate sidecar:

- The CLIs each have their own auth state (`~/.claude`, `~/.gemini`, …)
  that Prism does not want to touch.
- The CLIs each have wildly different argv shapes and output formats; pinning
  that mess inside an isolated process keeps Prism's Go binary free of
  Python / Node CLI integration code.
- The sidecar can be reused with one-api / one-hub / any other OpenAI client.

## 1. Install the sidecar

Pick whichever flavor matches your environment.

### Docker (recommended)

```bash
git clone https://github.com/anoxis/CLI2API.git
cd CLI2API
docker build -t cli2api -f Dockerfile.cli2api .

docker run -d --name cli2api-claude \
  -p 8000:8000 \
  -e CLI2API_PROVIDER=claude \
  -e CLI2API_CLI_PATH=/usr/local/bin/claude \
  -e CLI2API_DEFAULT_TIMEOUT=600 \
  -v $HOME/.claude:/home/cli2api/.claude \
  -v /usr/local/bin/claude:/usr/local/bin/claude:ro \
  cli2api
```

For Gemini, swap `CLI2API_PROVIDER=gemini`, mount `~/.gemini`, etc.

### Native Python (no Docker)

```bash
git clone https://github.com/anoxis/CLI2API.git
cd CLI2API
python3 -m venv venv && source venv/bin/activate
pip install -e .

CLI2API_PROVIDER=claude \
CLI2API_CLI_PATH=$(which claude) \
CLI2API_DEFAULT_TIMEOUT=600 \
uvicorn cli2api.main:app --host 127.0.0.1 --port 8000
```

The sidecar will refuse to start if `CLI2API_CLI_PATH` does not point at an
executable that the current user can run.

## 2. Add a LocalCLI channel in Prism

1. Open Prism. Go to **Channels → New channel**.
2. **Type**: choose `LocalCLI Gateway`.
3. **Name**: anything — e.g. `Claude Code (local)`.
4. **Base URL**: where Prism should send requests.
   - Sidecar on the same host: `http://127.0.0.1:8000`
   - Sidecar in Docker network: `http://cli2api-claude:8000`
5. **Key**: anything non-empty (cli2api ignores it; we keep the field
   non-empty so one-hub does not short-circuit).
6. **Models**: pick from the dropdown (`sonnet`, `opus`, `haiku`,
   `gemini-2.5-pro`, …) or type any custom model name. Whatever you type is
   handed to the sidecar verbatim.
7. (Optional) **Model redirects** — translate client-side names to whatever
   the sidecar expects:

   ```json
   {"claude-3-5-sonnet": "sonnet", "gemini-1.5-pro": "gemini-2.5-pro"}
   ```

8. Save → click **Test**. A 200 response means the wire is live.

## 3. Use it from Cursor / Cline / curl

LocalCLI looks exactly like an OpenAI endpoint to the client. With Prism's
default port (`3000`) and a token from the **Tokens** tab:

```bash
curl http://127.0.0.1:3000/v1/chat/completions \
  -H "Authorization: Bearer $PRISM_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "sonnet",
    "stream": true,
    "messages": [{"role":"user","content":"hello"}]
  }'
```

Cursor / Cline configuration: paste the snippet from
**Dashboard → Quick-start → OpenAI-compatible client**, change the model
name to one of your LocalCLI models, done.

## 4. Long requests and timeouts

CLIs can take 30+ seconds to first byte. Two knobs matter:

- **Prism**: `RELAY_TIMEOUT` defaults to 60s. Bump to `600` if you see
  premature `context deadline exceeded` errors. Set it in
  `prism.yaml`:

  ```yaml
  relay_timeout: 600
  ```

  or by environment variable when launching Prism in CLI mode.

- **cli2api**: `CLI2API_DEFAULT_TIMEOUT=600` (passed when starting the
  sidecar; see above).

## 5. Billing and accounting

CLIs do not return real token counts. cli2api forwards `prompt_tokens` and
`completion_tokens` estimated by character / word heuristics. Prism's log
entries will show these estimates; the dollar amount on the dashboard uses
them as if they were real OpenAI tokens.

If accuracy matters, pin a **fixed unit price** for your LocalCLI models in
**Settings → Pricing**, or set `model_ratio` to `0` and rely purely on
request counts.

## 6. Known limits

- Tool / function calling is fully validated for the `claude` provider only.
  Gemini, Codex, and Qwen function-calling support depends on the
  cli2api version you run.
- cli2api does not authenticate the caller; firewall the sidecar to
  loopback or a private network.
- The sidecar is an external process — Prism's "one-click desktop"
  promise only covers Prism itself, not the cli2api dependency.

## 7. Troubleshooting

| Symptom | Likely cause |
|---------|--------------|
| Test button returns `connection refused` | sidecar not running or wrong port |
| Test returns `401 Unauthorized` from sidecar | sidecar started with `CLI2API_REQUIRE_API_KEY=1` and Prism's empty-key passthrough was overridden in custom-headers plugin |
| Streaming hangs after first token | `RELAY_TIMEOUT` too low |
| Empty completion + 200 | CLI exited 0 but produced no stdout; check sidecar logs |
| `permission denied` running CLI | wrong mount or missing `+x` on `CLI2API_CLI_PATH` |

For deeper debugging, set `CLI2API_LOG_LEVEL=DEBUG` and tail the sidecar
logs while replaying the request from Prism's **Logs → Replay** view.
