#!/usr/bin/env bash
#
# Offline integration test for scripts/install.sh.
# Serves a fake GitHub Release tree over HTTP and exercises every code path
# (pinned version, latest, check-only, idempotent update, bad-checksum abort,
# missing-asset abort). Requires curl + python3.
#
# Usage: bash scripts/test_install.sh
#
set -euo pipefail

here="$(cd "$(dirname "$0")/.." && pwd)"
script="$here/scripts/install.sh"
name="llamacpp-launcher"

python3 - <<'PY' || { echo "python3 is required" >&2; exit 1; }
import sys
try:
    import http.server  # noqa: F401
except Exception:
    sys.exit(1)
PY

tmp_root="$(mktemp -d)"
http_port="${PORT:-$(( (RANDOM % 2000) + 8800 ))}"
base="http://127.0.0.1:${http_port}/releases"
http_pid=""
cleanup() {
  [ -n "$http_pid" ] && kill "$http_pid" 2>/dev/null || true
  rm -rf "$tmp_root"
}
trap cleanup EXIT

tree="$tmp_root/fake"
mkdir -p "$tree/releases/latest/download" \
         "$tree/releases/download/v0.1.1" \
         "$tree/releases/download/v0.9.9"

printf 'FAKE-0.1.1-amd64\n' > "$tree/releases/download/v0.1.1/$name-linux-amd64"
printf 'FAKE-latest-amd64\n' > "$tree/releases/latest/download/$name-linux-amd64"
printf 'FAKE-latest-arm64\n' > "$tree/releases/latest/download/$name-linux-arm64"
printf 'FAKE-BAD-amd64\n'    > "$tree/releases/download/v0.9.9/$name-linux-amd64"

( cd "$tree/releases/download/v0.1.1"  && sha256sum "$name-linux-amd64"  > checksums.txt )
( cd "$tree/releases/latest/download"  && sha256sum "$name-linux-amd64" "$name-linux-arm64" > checksums.txt )
echo "deadbeef  $name-linux-amd64" > "$tree/releases/download/v0.9.9/checksums.txt"

python3 -m http.server "$http_port" --directory "$tree" >/tmp/install_test_httpd.log 2>&1 &
http_pid=$!
sleep 1

fail=0
assert_eq() { [ "$1" = "$2" ] || { echo "FAIL: $3 (got '$1', want '$2')"; fail=1; }; }
must_fail() { if RELEASE_BASE_URL="$base" bash "$script" "$@" >/tmp/it.log 2>&1; then echo "FAIL: expected error, succeeded: $*"; cat /tmp/it.log; fail=1; else echo "ok: rejected -> $(tail -1 /tmp/it.log)"; fi; }

echo "== 1) pinned version =="
rm -rf /tmp/it_pinned
RELEASE_BASE_URL="$base" bash "$script" --version v0.1.1 --dest /tmp/it_pinned >/dev/null
assert_eq "$(cat /tmp/it_pinned/$name)" "FAKE-0.1.1-amd64" "pinned content"

echo "== 2) latest =="
rm -rf /tmp/it_latest
RELEASE_BASE_URL="$base" bash "$script" --dest /tmp/it_latest >/dev/null
assert_eq "$(cat /tmp/it_latest/$name)" "FAKE-latest-amd64" "latest content"

echo "== 3) idempotent update"
RELEASE_BASE_URL="$base" bash "$script" --dest /tmp/it_latest >/dev/null
assert_eq "$(cat /tmp/it_latest/$name)" "FAKE-latest-amd64" "updated content"

echo "== 4) check-only"
out="$(RELEASE_BASE_URL="$base" bash "$script" --check-only 2>/dev/null)"
case "$out" in *"OK"*) echo "ok: $out" ;; *) echo "FAIL: check-only"; fail=1 ;; esac

echo "== 5) bad checksum aborts"
must_fail --version v0.9.9 --dest /tmp/it_bad

echo "== 6) missing asset aborts"
must_fail --version v4.4.4 --dest /tmp/it_missing

echo "== 7) help exits 0"
bash "$script" -h >/dev/null 2>&1 || { echo "FAIL: help"; fail=1; }

echo "== 8) PATH appends export to rc file (default ~/.local/bin) =="
fake_home="$(mktemp -d)"
HOME="$fake_home" SHELL=/bin/bash RELEASE_BASE_URL="$base" bash "$script" >/tmp/it_path.log 2>&1
rc="$fake_home/.bashrc"
if [ -f "$rc" ] && grep -qF "$fake_home/.local/bin" "$rc"; then echo "ok: rc updated"; else echo "FAIL: rc not updated"; cat /tmp/it_path.log; fail=1; fi

echo "== 9) PATH idempotent (re-run does not duplicate) =="
HOME="$fake_home" SHELL=/bin/bash RELEASE_BASE_URL="$base" bash "$script" >/dev/null 2>&1
count=$(grep -cF "$fake_home/.local/bin" "$rc" || true)
if [ "$count" -eq 1 ]; then echo "ok: single entry"; else echo "FAIL: $count entries"; fail=1; fi
rm -rf "$fake_home"

[ "$fail" -eq 0 ] && echo "INSTALLER TESTS PASSED" || { echo "INSTALLER TESTS FAILED" >&2; exit 1; }
