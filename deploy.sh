#!/usr/bin/env bash
# Развёртывание llama-cpp-webui на сервере.
# Требует sshpass + openssh + go в PATH (запускать внутри nix-shell -p sshpass openssh go).
set -euo pipefail

SSHPW="${SSHPASS:?set SSHPASS=<sudo password> in your environment before deploying}"
USERHOST="evilvl@192.168.1.101"
DIR="$(cd "$(dirname "$0")" && pwd)"
BIN_NAME="llama-cpp-webui"
BIN_LOCAL="$DIR/$BIN_NAME"

echo ">> загрузка бинарника: $BIN_LOCAL"
[ -x "$BIN_LOCAL" ] || { echo "бинарник не собран: $BIN_LOCAL"; exit 1; }
sshpass -e scp -o StrictHostKeyChecking=no -o ConnectTimeout=15 "$BIN_LOCAL" /tmp/$BIN_NAME

echo ">> установка + юнит + запуск (одним удалённым скриптом)"
sshpass -e ssh -o StrictHostKeyChecking=no -o ConnectTimeout=15 "$USERHOST" 'bash -s' <<'REMOTE'
set -euo pipefail
export SUDO_PW="${SSHPASS:?set SSHPASS=<sudo password> in your environment before deploying}"
sudo() { echo "$SUDO_PW" | sudo -S "$@"; }

echo ">> права пользователя: $(whoami); arch: $(uname -m)"
sudo install -m 0755 /tmp/llama-cpp-webui /usr/local/bin/llama-cpp-webui && echo INSTALLED
/usr/local/bin/llama-cpp-webui --web-host 127.0.0.1 --web-port 18099 >/dev/null 2>&1 &
p=$!; sleep 1; curl -s localhost:18099/api/version; echo "  <- версия"; kill $p 2>/dev/null || true

sudo tee /etc/systemd/system/llama-cpp-webui.service >/dev/null <<'UNIT'
[Unit]
Description=llama-cpp-webui (управление llama.cpp через браузер)
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
User=root
ExecStart=/usr/local/bin/llama-cpp-webui --web-host 0.0.0.0 --web-port 8080
Environment=LLAMA_MODEL_ROOT=/opt/models
Environment=LLAMA_SERVER_BIN=/opt/llama-bin/llama-server
Environment=LLAMA_SERVICE_NAME=llama-coder
Environment=LLAMA_CONFIG_DIR=/etc/llama-cpp/configs
Environment=LLAMA_WAIT_TIMEOUT=600
Restart=on-failure
RestartSec=10

[Install]
WantedBy=multi-user.target
UNIT

sudo systemctl daemon-reload
sudo systemctl enable --now llama-cpp-webui.service && echo ENABLED
echo ">> статус:"; systemctl is-active llama-cpp-webui
systemctl status llama-cpp-webui --no-pager -l | head -12
echo ">> API:"
curl -s localhost:8080/api/version; echo
curl -s localhost:8080/api/models; echo
echo ">> Готово"
REMOTE
