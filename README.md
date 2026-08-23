# llama-cpp-launcher

Lightweight, zero-dependency Go web UI to manage a [`llama.cpp`](https://github.com/ggml-org/llama.cpp) server through a single systemd unit. Pick a model, tweak flags in a form generated from the binary's own `--help`, and start / stop / restart / inspect health — all from the browser. Configuration is stored per-model as a `KEY=value` file.

> Альтернатива `configure-llama-cpp` на Go: один веб-интерфейс, ноль внешних зависимостей (только stdlib), флаги подставляются из `llama-server --help`.

---

## Contents
- [Features](#features)
- [How it works](#how-it-works)
- [Repository layout](#repository-layout)
- [Configuration](#configuration)
- [Build & run](#build--run)
- [API](#api)
- [i18n / Localization](#i18n--localization)
- [Testing](#testing)
- [Deployment](#deployment)

---

## Features

- **Dynamic flag forms** — flag list, kinds (toggle / enum / value), choices and defaults are parsed at runtime from `llama-server --help`. No code changes when llama.cpp adds a new flag.
- **Per-model config** — model path + a map of active flags, saved as `/etc/llama-cpp/configs/<model>.conf` (or `LLAMA_CONFIG_DIR`).
- **systemd integration** — builds a unit, writes it atomically, `enable --now`, waits for `/health`, then runs an inference health-test.
- **Health & logs** — polls `/health`, runs a test `POST /v1/chat/completions`, shows `journalctl` output.
- **Bilingual UI** — English (default) and Russian; language is remembered in `localStorage` and can be extended.
- **Single binary, stdlib only** — no `go mod download` of third-party packages, fast builds, trivial to vendor.

---

## How it works

```
browser  ──HTTP──▶  llama-cpp-webui (Go, stdlib)  ──systemctl──▶  llama-coder.service
                              │
                              └──── parses `llama-server --help` ─▶  flag form
                              │
                              └──── writes unit + config files ─▶  /opt/models, /etc/llama-cpp/configs
```

The service unit `ExecStart` is built as ordered segments: `binary`, `-m model`, `--host`, `--port`, then sorted `flag value` pairs. Toggle flags (`--no-perf`) are emitted bare when truthy. Arbitrary arguments that don't fit the form are appended under the `__extra__` flag.

---

## Repository layout

| File | Purpose |
|------|---------|
| `main.go` | CLI flags, env wiring, HTTP routes, embedded UI |
| `config.go` | `Config`, default flags, `parseConfigFile`, atomic `save`, `buildExecStart` |
| `flags.go` | `--help` parser: `Kind` (toggle/enum/value), choices, sorting |
| `service.go` | unit generation, start/stop/restart, health wait, inference test |
| `discover.go` | recursive `.gguf` discovery in `MODEL_ROOT`, human-readable sizes |
| `handlers.go` | JSON endpoints for models / config / flags / status / logs |
| `ui/index.html` | embedded single-page UI + i18n table |
| `deploy.sh` | remote deployment (scp + remote systemd unit) |
| `smoke.sh` | local build + run + curl smoke test |
| `tests/` | Go unit tests + `tests/ui_i18n.test.mjs` (Node) + `tests/run.sh` runner |

---

## Configuration

All settings are environment variables (CLI flags for the web server only):

| Var | Default | Description |
|-----|---------|-------------|
| `LLAMA_MODEL_ROOT` | `/opt/models` | Directory scanned for `.gguf` models (recursive) |
| `LLAMA_SERVER_BIN` | `/opt/llama-bin/llama-server` | `llama-server` binary; its `--help` populates the flag form |
| `LLAMA_SERVICE_NAME` | `llama-coder` | Target systemd unit name |
| `LLAMA_CONFIG_DIR` | `/etc/llama-cpp/configs` | Where per-model `*.conf` files live |
| `LLAMA_WAIT_TIMEOUT` | `600` | Seconds to wait for `/health` after start |
| `LLAMA_WEB_HOST` | `127.0.0.1` | Bind address of the web UI |
| `LLAMA_WEB_PORT` | `8080` | Bind port of the web UI |

Default flags applied to every new model (overridable per config): `--host`, `--port`, `--threads`/`--threads-batch` (= nproc), `--ctx-size 256000`, `--fit on`, `--fit-target 256`, `--fit-ctx 256000`, `--flash-attn on`, `--numa distribute`, `--kv-offload 1`, `--load-mode mlock`.

---

## Build & run

```bash
# Build the web server binary
nix-shell -p go --command 'go build -o llama-cpp-webui .'

# Run (needs read access to MODEL_ROOT and a llama-server binary)
LLAMA_MODEL_ROOT=/opt/models \
LLAMA_SERVER_BIN=/opt/llama-bin/llama-server \
LLAMA_SERVICE_NAME=llama-coder \
./llama-cpp-webui --web-host 127.0.0.1 --web-port 8080
```

Open <http://localhost:8080>.

### API

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/` | The web UI (embedded) |
| `GET` | `/api/version` | Version / service name |
| `GET` | `/api/flags` | Full flag list with kinds and defaults |
| `GET` | `/api/models` | Available `.gguf` models |
| `GET` | `/api/config?model=...` | Current config (model + flags + extra) for a model |
| `POST` | `/api/config` | Save config: `{"model","flags","extra"}` |
| `GET` | `/api/status` | Service state (`systemctl show`) |
| `POST` | `/api/start` | Enable + start + wait for `/health` |
| `POST` | `/api/stop` | Stop the service |
| `POST` | `/api/restart` | Restart the service |
| `GET` | `/api/logs?lines=200` | `journalctl -u <service> -n N` output |
| `POST` | `/api/health-test` | Test inference (`/v1/chat/completions`) |

Example:

```bash
curl -s localhost:8080/api/version
# {"name":"llama-cpp-webui","service":"llama-coder"}

curl -s -XPOST localhost:8080/api/config \
  -H 'Content-Type: application/json' \
  -d '{"model":"/opt/models/qwen2.5-7b-instruct-q4_k_m.gguf","flags":{"--port":"8090","--gpu-layers":"33"}}'
```

---

## i18n / Localization

The UI ships in English by default with a Russian translation. The selected language is persisted in `localStorage` and mirrored to the `lang` attribute of `<html>`; if the browser language is non-English it is used as the default, falling back to English.

Translations live in the `I18N` table in `ui/index.html`:

```js
const I18N = { en: { search: "Search models", ... }, ru: { search: "Поиск моделей", ... } };
```

Add a string: put the key in `en`, add a `ru` (and any other language) entry, then reference it in the markup with `t("key")`. The `t()` helper falls back to English, then to the key itself, so new keys work out of the box.

### Tests

Go unit tests (`go test ./...`) cover: shell quoting, config round-trip, model discovery, unit/`buildExecStart` generation, `--help` parsing (kinds / choices / sections), flag sorting, and the HTTP handlers.

UI/i18n is tested in pure Node (no browser, no npm) — `tests/ui_i18n.test.mjs` loads the page, checks the default is English, verifies RU switching + persistence, fallbacks, and the `<html lang="en">` + `<select id="lang">` dropdown:

```bash
# everything at once
nix-shell -p go nodejs --command './tests/run.sh'

# or individually
nix-shell -p go  --command 'go test ./...'
nix-shell -p nodejs --command 'node tests/ui_i18n.test.mjs'
```

---

## Deployment

`deploy.sh` uploads the built binary and installs a systemd unit on a remote host. It requires `sshpass` + `openssh` + `go` in `PATH` (use `nix-shell -p sshpass openssh go`).

```bash
nix-shell -p sshpass openssh go --command './deploy.sh'
```

The unit (see `deploy.sh`) runs `llama-cpp-webui` with:

```ini
Environment=LLAMA_MODEL_ROOT=/opt/models
Environment=LLAMA_SERVER_BIN=/opt/llama-bin/llama-server
Environment=LLAMA_SERVICE_NAME=llama-coder
Environment=LLAMA_CONFIG_DIR=/etc/llama-cpp/configs
Environment=LLAMA_WAIT_TIMEOUT=600
```

Exposing the web UI to a network requires a reverse proxy / firewall review — the default binds `127.0.0.1` only.

---

## Testing (overview)

```bash
go vet ./...        # static analysis
go test ./...       # Go unit tests (22 tests)
node tests/ui_i18n.test.mjs   # UI/i18n tests (26 assertions)
```
