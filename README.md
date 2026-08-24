# llama-cpp-launcher

Lightweight, zero-dependency Go web UI to manage a [`llama.cpp`](https://github.com/ggml-org/llama.cpp) server through a single systemd unit. Pick a model, tweak flags in a form generated from the binary's own `--help`, and start / stop / restart / inspect health — all from the browser. Configuration is stored per-model as a `KEY=value` file.
---

## Contents
- [Features](#features)
- [How it works](#how-it-works)
- [Repository layout](#repository-layout)
- [Configuration](#configuration)
- [Build & run](#build--run)
- [API](#api)
- [i18n / Localization](#i18n--localization)
- [Security](#security)
- [Testing](#testing)
- [Deployment](#deployment)
- [Contributing](#contributing)
- [Roadmap](#roadmap)

---

## Features

- **Dynamic flag forms** — flag list, kinds (toggle / enum / value), choices and defaults are parsed at runtime from `llama-server --help`. No code changes when llama.cpp adds a new flag.
- **Per-model config** — model path + a map of active flags, saved as `/etc/llama-cpp/configs/<model>.conf` (or `LLAMA_CONFIG_DIR`).
- **systemd integration** — builds a unit, writes it atomically, `enable --now`, waits for `/health`, then runs an inference health-test.
- **Model presets** — four built-in launch presets (`fast-streaming`, `large-context`, `cpu-only`, `vram-friendly`); one-click form fill from any preset.
- **Health & logs** — polls `/health`, runs a test `POST /v1/chat/completions`, shows `journalctl` output.
- **Bilingual UI** — English (default) and Russian; language is remembered in `localStorage` and can be extended.
- **Single binary, stdlib only** — no `go mod download` of third-party packages, fast builds, trivial to vendor.

---

## How it works

```
browser  ──HTTP──▶  llamacpp-launcher (Go, stdlib)  ──systemctl──▶  llama-coder.service
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
| `cmd/llamacpp-launcher/*.go` | Go source: `main.go` (CLI flags, env wiring, HTTP routes), `config.go` (`Config`, default flags, atomic `save`, `buildExecStart`), `presets.go`, `flags.go` (`--help` parser), `service.go` (systemd unit + start/stop/restart/health), `discover.go` (`.gguf` discovery), `handlers.go` (JSON endpoints) |
| `cmd/llamacpp-launcher/ui/index.html` | embedded single-page UI + i18n table (referenced via `//go:embed ui/index.html`) |
| `flake.nix` | Nix flake: `nix build .` (package), `nix develop` (dev shell), `nix flake check` (go vet + go test + UI/i18n + gofmt), `nix fmt` (nixfmt) |
| `tests/` | `tests/ui_i18n.test.mjs` (Node) — runs the embedded page in a `vm` sandbox; both run under `nix flake check` |

The Go binary lives under `cmd/` (idiomatic entry-point layout); `go.mod` stays at the repo root and defines the module `llamacpp-launcher`.

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

Everything builds and runs inside a Nix flake (`flake.nix`, pinned to `nixpkgs`
`nixos-unstable` so the Go toolchain matches `go.mod`'s `go 1.26`). Enter a dev
shell — it provides go + nodejs with `CGO_ENABLED=0` by default (no gcc needed):

```bash
# Enter the dev shell
nix develop

# Build the web server binary (static, CGO_ENABLED=0)
nix build .          # output: ./result/bin/llamacpp-launcher

# Run (needs read access to MODEL_ROOT and a llama-server binary)
LLAMA_MODEL_ROOT=/opt/models \
LLAMA_SERVER_BIN=/opt/llama-bin/llama-server \
LLAMA_SERVICE_NAME=llama-coder \
./result/bin/llamacpp-launcher --web-host 127.0.0.1 --web-port 8080
```

Open <http://localhost:8080>.

### API

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/` | The web UI (embedded) |
| `GET` | `/api/version` | Version / service name |
| `GET` | `/api/flags` | Full flag list with kinds and defaults |
| `GET` | `/api/presets` | Built-in launch presets |
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
# {"name":"llamacpp-launcher","service":"llama-coder"}

curl -s -XPOST localhost:8080/api/config \
  -H 'Content-Type: application/json' \
  -d '{"model":"/opt/models/qwen2.5-7b-instruct-q4_k_m.gguf","flags":{"--port":"8090","--gpu-layers":"33"}}'
```

---

## i18n / Localization

The UI ships in English by default with a Russian translation. The selected language is persisted in `localStorage` and mirrored to the `lang` attribute of `<html>`; if the browser language is non-English it is used as the default, falling back to English.

Translations live in the `I18N` table in `cmd/llamacpp-launcher/ui/index.html`:

```js
const I18N = { en: { search: "Search models", ... }, ru: { search: "Поиск моделей", ... } };
```

Add a string: put the key in `en`, add a `ru` (and any other language) entry, then reference it in the markup with `t("key")`. The `t()` helper falls back to English, then to the key itself, so new keys work out of the box.

### Tests

Go unit tests (`go test ./...`) cover: shell quoting, config round-trip, model discovery, unit/`buildExecStart` generation, `--help` parsing (kinds / choices / sections), flag sorting, and the HTTP handlers.

UI/i18n is tested in pure Node (no browser, no npm) — `tests/ui_i18n.test.mjs` loads the page, checks the default is English, verifies RU switching + persistence, fallbacks, and the `<html lang="en">` + `<select id="lang">` dropdown.

Run the whole suite with `nix flake check` — it builds the package and runs
`go vet` + `go test ./...`, `node tests/ui_i18n.test.mjs`, and a `gofmt` check:

```bash
nix flake check        # everything
# or individually, inside the dev shell:
nix develop -c go test ./...
nix develop -c go vet ./...
nix develop -c node tests/ui_i18n.test.mjs
```

---

## Security

- **Network exposure.** The web UI binds `127.0.0.1` by default; the model
  service (`--host`) defaults to `0.0.0.0`. Before exposing either to a LAN or
   the internet, put it behind a reverse proxy with authentication and review the
   firewall. The production systemd unit explicitly binds `0.0.0.0` —
   only run it on a trusted host.
- **API keys.** `--api-key` is written into the systemd unit and sent as an
  `Authorization: Bearer` header. Never log it and keep it out of response bodies.
- **Raw extras.** `__extra__` is appended verbatim to `ExecStart` as shell — only
  set it to values you control.

---

## Testing (overview)

All of the below run under `nix flake check` (needs the go + nodejs from `nix
develop`; CGO is disabled by default):

```bash
nix flake check     # go vet, go test, UI/i18n tests, gofmt — all at once
```

Individually, inside `nix develop`:

```bash
go vet ./...        # static analysis
go test ./...       # Go unit tests (36 tests)
node tests/ui_i18n.test.mjs   # UI/i18n tests (26 assertions)
```

---

## Contributing

- Build and test inside the flake dev shell (`nix develop`) — go + nodejs with
  `CGO_ENABLED=0` by default (no gcc needed).
- Run `nix flake check` before finishing, and add tests for new code.
- Run `nix fmt flake.nix` to keep the Nix code nixfmt-formatted.
- Run `nix run .#smoke` for a full build + API round-trip (needs no llama-server).
- Keep the stdlib-only, minimal style; never break the `buildExecStart` ordering invariant.
- Bilingual UI: add every new string to the `I18N` table (`en` + `ru`) or the UI/i18n test breaks.

---

## Roadmap

### Done

- **Unit portability** — NVIDIA settings are now best-effort: detected via `nvidia-smi`, logged if absent, optionally installed. AMD / CPU-only backends work.
- **Model presets / templates** — four built-in presets (`fast-streaming`, `large-context`, `cpu-only`, `vram-friendly`) with one-click form fill.

### Upcoming

- **Config import / export** — share configurations as JSON or `.conf`.
- **Batch model management** — duplicate, rename, and delete configs.
- **Health dashboard** — inference latency and token-speed metrics.
- **More languages** — extend the UI beyond English and Russian.
