#!/bin/sh
# PG-BASE binary installer (Sprint 0a) — the composable path for agents and
# bare-metal users. Downloads a release binary for the current platform from
# GitHub Releases and verifies it against the release checksums before
# extracting (fork policy: checksum-verified downloads).
#
#   curl -fsSL https://raw.githubusercontent.com/arief-fajri/pgbase/main/install.sh | sh
#
# Optional env overrides:
#   PGBASE_VERSION     a specific release (e.g. v0.5.2); default: latest
#   PGBASE_INSTALL_DIR destination dir (default: /usr/local/bin, sudo if needed)
#
# Requires: curl; one of: unzip, python3, or bsdtar.
# The binary talks to any PostgreSQL 16+ — bring your own database, or use
# the Docker quickstart: https://arief-fajri.github.io/pgbase/agents
set -eu

REPO="arief-fajri/pgbase"

log() { printf '==> %s\n' "$1"; }
die() { printf 'error: %s\n' "$1" >&2; exit 1; }

command -v curl >/dev/null 2>&1 || die "curl is required"

# --- platform detection ------------------------------------------------------
case "$(uname -s)" in
    Darwin) os="darwin" ;;
    Linux) os="linux" ;;
    *) die "unsupported OS '$(uname -s)' — download a zip from https://github.com/$REPO/releases" ;;
esac

case "$(uname -m)" in
    x86_64 | amd64) arch="amd64" ;;
    aarch64 | arm64) arch="arm64" ;;
    armv7l | armv6l) arch="armv7" ;;
    s390x) arch="s390x" ;;
    ppc64le) arch="ppc64le" ;;
    *) die "unsupported architecture '$(uname -m)' — see https://github.com/$REPO/releases" ;;
esac

# --- resolve version ---------------------------------------------------------
if [ -n "${PGBASE_VERSION:-}" ]; then
    # normalize to a leading 'v'
    case "$PGBASE_VERSION" in
        v*) tag="$PGBASE_VERSION" ;;
        *) tag="v$PGBASE_VERSION" ;;
    esac
else
    log "resolving the latest release"
    tag="$(curl -fsSL "https://api.github.com/repos/$REPO/releases/latest" \
        | sed -n 's/.*"tag_name": *"\([^"]*\)".*/\1/p' | head -n 1)"
    [ -n "$tag" ] || die "could not resolve the latest release (API rate limit?) — set PGBASE_VERSION, e.g. PGBASE_VERSION=v0.5.2 sh install.sh"
fi
ver="${tag#v}" # asset names drop the leading 'v'

asset="pgbase_${ver}_${os}_${arch}.zip"
base_url="https://github.com/$REPO/releases/download/$tag"

# --- download + verify -------------------------------------------------------
tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT

log "downloading $asset"
curl -fsSL "$base_url/$asset" -o "$tmp/$asset"

log "verifying checksum"
curl -fsSL "$base_url/checksums.txt" -o "$tmp/checksums.txt"
expected="$(sed -n "s/^\([0-9a-fA-F]*\).*$asset$/\1/p" "$tmp/checksums.txt" | tr -d '[:space:]')"
[ -n "$expected" ] || die "no checksum entry for $asset — report this at https://github.com/$REPO/issues"
if command -v sha256sum >/dev/null 2>&1; then
    actual="$(sha256sum "$tmp/$asset" | cut -d' ' -f1)"
elif command -v shasum >/dev/null 2>&1; then
    actual="$(shasum -a 256 "$tmp/$asset" | cut -d' ' -f1)"
else
    die "neither sha256sum nor shasum is available — verify $tmp/$asset manually against $base_url/checksums.txt"
fi
[ "$actual" = "$expected" ] || die "checksum mismatch for $asset — refusing to install"

# --- extract -----------------------------------------------------------------
log "extracting"
if command -v unzip >/dev/null 2>&1; then
    unzip -oq "$tmp/$asset" -d "$tmp/out"
elif command -v python3 >/dev/null 2>&1; then
    python3 -m zipfile -e "$tmp/$asset" "$tmp/out"
elif tar -tf "$tmp/$asset" >/dev/null 2>&1; then
    mkdir -p "$tmp/out" && tar -xf "$tmp/$asset" -C "$tmp/out"
else
    die "no unzip/python3/bsdtar available to extract the zip"
fi
[ -f "$tmp/out/pgbase" ] || die "archive did not contain a 'pgbase' binary"
chmod +x "$tmp/out/pgbase"

# --- install -----------------------------------------------------------------
dest_dir="${PGBASE_INSTALL_DIR:-/usr/local/bin}"
dest="$dest_dir/pgbase"
if [ -w "$dest_dir" ] 2>/dev/null; then
    cp "$tmp/out/pgbase" "$dest"
elif command -v sudo >/dev/null 2>&1; then
    sudo cp "$tmp/out/pgbase" "$dest"
else
    cp "$tmp/out/pgbase" ./pgbase
    dest="$(pwd)/pgbase"
    log "no write access to $dest_dir and no sudo — installed to $dest"
fi

# --- summary -----------------------------------------------------------------
printf '\n'
printf 'Installed: %s (PG-BASE %s, %s/%s)\n\n' "$dest" "$tag" "$os" "$arch"
printf 'Next steps — the binary talks to any PostgreSQL 16+:\n\n'
printf '  %s serve --http 127.0.0.1:8090 \\\n    --pg-host <host> --pg-user <user> --pg-password <pass> --pg-dbname <db>\n\n' "$dest"
printf '  # first superuser\n  %s superuser upsert admin@example.com <strong-password>\n\n' "$dest"
printf 'Docs: https://arief-fajri.github.io/pgbase/ — API is PocketBase-compatible\n'
printf '(fork deltas: https://arief-fajri.github.io/pgbase/fork-deltas)\n'
