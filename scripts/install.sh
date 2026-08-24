#!/usr/bin/env bash
#
# Install / update llamacpp-launcher from a GitHub Release.
#
#   curl -fsSL https://raw.githubusercontent.com/evilvl/llamacpp-launcher/HEAD/scripts/install.sh | bash
#
# Re-running the same command updates to the latest release (idempotent).
#
# Options:
#   --version X       Install a specific release tag (default: latest)
#   --dest DIR        Install directory (default: ~/.local/bin, sudo if unwritable)
#   --check-only      Download + verify checksum, do not install
#   -h, --help        Show this help
#
set -euo pipefail

repo="evilvl/llamacpp-launcher"
bin_name="llamacpp-launcher"
default_base="https://github.com/${repo}/releases"
base="${RELEASE_BASE_URL:-$default_base}"

dest_dir=""
want_version=""
check_only=0

err() { printf 'install: %s\n' "$*" >&2; exit 1; }

usage() {
  sed -n '2,14p' "$0" | sed 's/^# \{0,1\}//'
  exit 0
}

while [ $# -gt 0 ]; do
  case "$1" in
    -h|--help) usage ;;
    --version) want_version="${2:-}"; [ -n "$want_version" ] || err "--version needs an argument"; shift ;;
    --dest) dest_dir="${2:-}"; [ -n "$dest_dir" ] || err "--dest needs an argument"; shift ;;
    --check-only) check_only=1 ;;
    --) shift; break ;;
    *) err "unknown argument: $1" ;;
  esac
  shift
done

uname_os() {
  case "$(uname -s)" in
    Linux) printf 'linux' ;;
    Darwin) printf 'darwin' ;;
    *) err "unsupported OS: $(uname -s)" ;;
  esac
}

uname_arch() {
  case "$(uname -m)" in
    x86_64|amd64) printf 'amd64' ;;
    aarch64|arm64) printf 'arm64' ;;
    *) err "unsupported arch: $(uname -m)" ;;
  esac
}

sha256_of() {
  if command -v sha256sum >/dev/null 2>&1; then
    sha256sum "$1" | awk '{print $1}'
  else
    shasum -a 256 "$1" | awk '{print $1}'
  fi
}

die_missing() {
  case "$1" in
    curl) err "curl is required but not installed" ;;
    *) err "$1 is required but not installed" ;;
  esac
}

for tool in curl; do
  command -v "$tool" >/dev/null 2>&1 || die_missing "$tool"
done

os="$(uname_os)"
arch="$(uname_arch)"
asset="${bin_name}-${os}-${arch}"
version="${want_version:-latest}"

# GitHub layout: /releases/latest/download/<asset> or /releases/download/<tag>/<asset>.
if [ "$version" = "latest" ]; then
  asset_url="${base}/latest/download/${asset}"
  checksums_url="${base}/latest/download/checksums.txt"
else
  asset_url="${base}/download/${version%/}/${asset}"
  checksums_url="${base}/download/${version%/}/checksums.txt"
fi

tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT

printf 'fetching %s from %s\n' "$asset" "$asset_url"
curl -fsSL "$asset_url" -o "$tmp/$asset"
curl -fsSL "$checksums_url" -o "$tmp/checksums.txt"

printf 'verifying checksums\n'
if [ -f "$tmp/checksums.txt" ]; then
  expected="$(awk -v f="$asset" '$2 == f {print $1; exit}' "$tmp/checksums.txt")"
  actual="$(sha256_of "$tmp/$asset")"
  [ -n "$expected" ] || err "no checksum entry for $asset in checksums.txt"
  [ "$expected" = "$actual" ] || err "checksum verification failed for $asset"
  printf '  %s OK\n' "$asset"
else
  printf '  (no checksums.txt in release — skipping verification)\n'
fi

if [ "$check_only" -eq 1 ]; then
  printf 'check-only: %s OK (%d bytes)\n' "$asset" "$(wc -c < "$tmp/$asset")"
  exit 0
fi

if [ -n "$dest_dir" ]; then
  install_dir="$dest_dir"
elif [ -n "${DEST_DIR:-}" ]; then
  install_dir="$DEST_DIR"
else
  install_dir="${HOME:-/root}/.local/bin"
fi

mkdir -p "$install_dir"
target="${install_dir%/}/$bin_name"

if [ -w "$install_dir" ]; then
  install -m 0755 "$tmp/$asset" "$target"
else
  printf 'installing to %s with sudo\n' "$install_dir"
  sudo install -m 0755 "$tmp/$asset" "$target"
fi

printf 'installed %s -> %s\n' "$bin_name" "$target"
case "$install_dir" in
  */.local/bin|/root/.local/bin)
    printf 'make sure %s is on your PATH\n' "$install_dir" ;;
esac
