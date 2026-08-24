# llamacpp-launcher

Lightweight, zero-dependency Go web UI to manage a [`llama.cpp`](https://github.com/ggml-org/llama.cpp) server through a single systemd unit. Pick a model, tweak flags in a form generated from the binary's own `--help`, and start / stop / restart / inspect health — all from the browser. Configuration is stored per-model as a `KEY=value` file.

## Features

- **Dynamic flag forms** — flag kinds (toggle / enum / value), choices and defaults are parsed at runtime from `llama-server --help`; no code changes for new flags.
- **Per-model config** — model path + a map of active flags saved as `/etc/llama-cpp/configs/<model>.conf` (or `LLAMA_CONFIG_DIR`).
- **systemd integration** — builds a unit, writes it atomically, `enable --now`, waits for `/health`, then runs an inference health-test.
- **Model presets** — four launch presets (`fast-streaming`, `large-context`, `cpu-only`, `vram-friendly`) with one-click form fill.
- **Health & logs** — polls `/health`, runs a test chat completion, shows `journalctl` output.
- **Bilingual UI** — English and Russian, persisted in `webui-settings.json`.
- **Single binary, stdlib only** — no third-party Go modules, fast builds.

## Installation

The quickest way to install or update is the one-line installer. It downloads the prebuilt binary for your platform from the latest release, verifies it against that release's `checksums.txt`, and places it in `~/.local/bin` (re-run the same command to update):

```bash
curl -fsSL https://raw.githubusercontent.com/evilvl/llamacpp-launcher/HEAD/scripts/install.sh | bash
```

Pin a release (other flags `--dest` and `--check-only` are documented in the script):

```bash
RELEASE_BASE_URL=https://github.com/evilvl/llamacpp-launcher/releases \
  bash scripts/install.sh --version v0.2.0
```

Or build from source:

```bash
# From source (Go 1.26)
go build -o llamacpp-launcher ./cmd/llamacpp-launcher

# Via Nix
nix build .
./result/bin/llamacpp-launcher
```

## Quick start

Everything builds and runs inside the Nix flake (`flake.nix`); the dev shell provides Go + Node with `CGO_ENABLED=0` (no gcc needed):

```bash
nix develop            # enter the dev shell
nix build .            # build: ./result/bin/llamacpp-launcher

LLAMA_MODEL_ROOT=/opt/models \
LLAMA_SERVER_BIN=/opt/llama-bin/llama-server \
LLAMA_SERVICE_NAME=llama-coder \
./result/bin/llamacpp-launcher --web-host 127.0.0.1 --web-port 8080
```

Open <http://localhost:8080>.

## Configuration

All settings are environment variables:

| Var | Default | Description |
|-----|---------|-------------|
| `LLAMA_MODEL_ROOT` | `/opt/models` | Directory scanned for `.gguf` models (recursive) |
| `LLAMA_SERVER_BIN` | `/opt/llama-bin/llama-server` | `llama-server` binary; its `--help` populates the flag form |
| `LLAMA_SERVICE_NAME` | `llama-coder` | Target systemd unit name |
| `LLAMA_CONFIG_DIR` | `/etc/llama-cpp/configs` | Where per-model `*.conf` files live |
| `LLAMA_WAIT_TIMEOUT` | `600` | Seconds to wait for `/health` after start |
| `LLAMA_WEB_HOST` | `127.0.0.1` | Bind address of the web UI |
| `LLAMA_WEB_PORT` | `8080` | Bind port of the web UI (`0` = auto-pick a free port) |

Web host, port, and interface language are set live from the UI (Settings) via `POST /api/settings`; the model directory and llama-server path from the same panel via `POST /api/app/model-dir` and `POST /api/app/llama-server`. All values are persisted to `webui-settings.json`. A host/port change replaces the process in place; the others apply immediately. If the port is in use, the server falls back to a free port and logs the actual address.

Default flags applied to every new model (overridable per config): `--host`, `--port`, `--threads`/`--threads-batch` (= nproc), `--ctx-size 256000`, `--fit on`, `--fit-target 256`, `--fit-ctx 256000`, `--flash-attn on`, `--numa distribute`, `--kv-offload 1`, `--load-mode mlock`.

## API

All endpoints are served under the web UI root:

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/` | Web UI |
| `GET` | `/api/version` | Version / service name |
| `GET` | `/api/settings` | Web settings + app config (`modelDir`, `llamaServer`, `lang`) |
| `POST` | `/api/settings` | Set web host/port/language; persisted; host/port change replaces the process |
| `GET` | `/api/flags` | Flag list with kinds and defaults |
| `GET` | `/api/presets` | Built-in launch presets |
| `GET` | `/api/models` | Available `.gguf` models |
| `GET` | `/api/config?model=...` | Current config (model + flags + extra) for a model |
| `POST` | `/api/config` | Save config: `{"model","flags","extra"}` |
| `POST` | `/api/app/model-dir` | Set the model directory (reloads the model list) |
| `POST` | `/api/app/llama-server` | Set the llama-server binary path |
| `GET` | `/api/status` | Service state (`systemctl show`) |
| `POST` | `/api/start` | Enable + start + wait for `/health` |
| `POST` | `/api/stop` | Stop the service |
| `POST` | `/api/restart` | Restart the service |
| `GET` | `/api/logs?lines=200` | `journalctl -u <service> -n N` output |
| `POST` | `/api/health-test` | Test inference (`/v1/chat/completions`) |

```bash
curl -s localhost:8080/api/version
# {"name":"llamacpp-launcher","service":"llama-coder"}

curl -s -XPOST localhost:8080/api/config \
  -H 'Content-Type: application/json' \
  -d '{"model":"/opt/models/qwen2.5-7b-instruct-q4_k_m.gguf","flags":{"--port":"8090","--gpu-layers":"33"}}'
```

## Security

- **Network exposure.** The web UI binds `127.0.0.1` by default; the model service (`--host`) defaults to `0.0.0.0`. Proxy both behind authentication before exposing to a LAN or the internet, and review the firewall.
- **API keys.** `--api-key` is written into the systemd unit and sent as an `Authorization: Bearer` header — never log it.
- **Raw extras.** `__extra__` is appended verbatim to `ExecStart` as shell — only set it to values you control.

## Roadmap

- **Config import / export** — share configurations as JSON or `.conf`.
- **Batch model management** — duplicate, rename, and delete configs.
- **Health dashboard** — inference latency and token-speed metrics.
- **More languages** — extend the UI beyond English and Russian.

## Development

Build, test, and format everything through the flake:

```bash
nix flake check        # build + go vet + tests + UI/i18n + gofmt
nix run .#smoke         # build + HTTP round-trip (needs no llama-server)
```
