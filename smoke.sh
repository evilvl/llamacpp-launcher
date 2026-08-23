#!/usr/bin/env bash
# End-to-end smoke test: build, run, exercise the save/get round-trip with the
# current API payload (flags map), and verify it lands on disk.
# Run inside:  nix-shell -p go python3 --command './smoke.sh'
set -uo pipefail
nix-shell -p go --command 'go vet ./... && go build -o llama-cpp-webui . && echo BUILD_OK'
cd "$(pwd)"
D=$(mktemp -d)
mkdir -p "$D/models"
printf 'GGUF' > "$D/models/tiny.gguf"
export LLAMA_MODEL_ROOT="$D/models"
export LLAMA_CONFIG_DIR="$D/configs"
export LLAMA_WEB_HOST=127.0.0.1
export LLAMA_WEB_PORT=18079
./llama-cpp-webui >/tmp/llama_smoke.log 2>&1 &
P=$!
sleep 1
echo "=== save (flags map + extra) ==="
curl -s -XPOST localhost:18079/api/config -H 'Content-Type: application/json' \
  -d '{"model":"'$D'/models/tiny.gguf","flags":{"--port":"8090","--api-key":"abc","--ctx-size":"4096"},"extra":"--verbose --n-batch 512"}'
echo
echo "=== config after save (GET) ==="
curl -s "localhost:18079/api/config?model=$D/models/tiny.gguf" | python3 -m json.tool
echo "=== file on disk ==="
cat "$D/configs/tiny.conf"
kill "$P" 2>/dev/null
wait "$P" 2>/dev/null
rm -rf "$D"
echo "=== server log ==="
cat /tmp/llama_smoke.log
echo "smoke done"
