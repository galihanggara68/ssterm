#!/bin/sh
# ssterm installer script
# Usage:
#   curl -fsSL https://raw.githubusercontent.com/galihanggara68/ssterm/main/install.sh | sh
#   curl -fsSL https://raw.githubusercontent.com/galihanggara68/ssterm/main/install.sh | VERSION=v0.1.0-beta.1 INSTALL_DIR=~/.local/bin sh

set -eu

REPO="${REPO:-galihanggara68/ssterm}"
BINARY="ssterm"
VERSION="${VERSION:-}"
INSTALL_DIR="${INSTALL_DIR:-}"

log() {
  printf "==> %s\n" "$*"
}

err() {
  printf "error: %s\n" "$*" >&2
  exit 1
}

need_cmd() {
  command -v "$1" >/dev/null 2>&1 || err "required command '$1' not found"
}

detect_os() {
  os="$(uname -s | tr '[:upper:]' '[:lower:]')"
  case "$os" in
    linux*)  echo "linux" ;;
    darwin*) echo "darwin" ;;
    msys*|mingw*|cygwin*) echo "windows" ;;
    *) err "unsupported operating system: $os" ;;
  esac
}

detect_arch() {
  arch="$(uname -m)"
  case "$arch" in
    x86_64|amd64) echo "amd64" ;;
    aarch64|arm64) echo "arm64" ;;
    *) err "unsupported architecture: $arch" ;;
  esac
}

fetch() {
  url="$1"
  out="$2"
  if command -v curl >/dev/null 2>&1; then
    curl -fsSL "$url" -o "$out"
  elif command -v wget >/dev/null 2>&1; then
    wget -qO "$out" "$url"
  else
    err "either 'curl' or 'wget' is required"
  fi
}

resolve_version() {
  if [ -n "$VERSION" ]; then
    echo "$VERSION"
    return
  fi
  tmp_json="$(mktemp)"
  trap 'rm -f "$tmp_json"' EXIT
  # Try latest stable release first; fall back to most recent pre-release.
  if fetch "https://api.github.com/repos/${REPO}/releases/latest" "$tmp_json" 2>/dev/null; then
    tag="$(sed -n 's/.*"tag_name":[[:space:]]*"\([^"]*\)".*/\1/p' "$tmp_json" | head -n 1)"
    if [ -n "$tag" ]; then
      rm -f "$tmp_json"
      echo "$tag"
      return
    fi
  fi
  fetch "https://api.github.com/repos/${REPO}/releases?per_page=1" "$tmp_json"
  tag="$(sed -n 's/.*"tag_name":[[:space:]]*"\([^"]*\)".*/\1/p' "$tmp_json" | head -n 1)"
  rm -f "$tmp_json"
  [ -n "$tag" ] || err "could not determine latest release tag from https://github.com/${REPO}/releases"
  echo "$tag"
}

verify_checksum() {
  dir="$1"
  archive="$2"
  if [ ! -f "${dir}/checksums.txt" ]; then
    return 0
  fi
  expected="$(grep "  ${archive}\$" "${dir}/checksums.txt" | awk '{print $1}')"
  [ -n "$expected" ] || err "checksum for ${archive} not found in checksums.txt"
  if command -v sha256sum >/dev/null 2>&1; then
    actual="$(sha256sum "${dir}/${archive}" | awk '{print $1}')"
  elif command -v shasum >/dev/null 2>&1; then
    actual="$(shasum -a 256 "${dir}/${archive}" | awk '{print $1}')"
  else
    log "warning: neither sha256sum nor shasum found; skipping checksum verification"
    return 0
  fi
  [ "$expected" = "$actual" ] || err "SHA-256 checksum mismatch for ${archive} (expected ${expected}, got ${actual})"
}

choose_install_dir() {
  if [ -n "$INSTALL_DIR" ]; then
    echo "$INSTALL_DIR"
    return
  fi
  if [ -w "/usr/local/bin" ]; then
    echo "/usr/local/bin"
    return
  fi
  if [ -d "$HOME/.local/bin" ]; then
    case ":${PATH}:" in
      *":$HOME/.local/bin:"*)
        echo "$HOME/.local/bin"
        return
        ;;
    esac
  fi
  echo "/usr/local/bin"
}

main() {
  os="$(detect_os)"
  arch="$(detect_arch)"
  version="$(resolve_version)"

  ext="tar.gz"
  bin_name="${BINARY}"
  if [ "$os" = "windows" ]; then
    ext="zip"
    bin_name="${BINARY}.exe"
  fi

  folder="ssterm_${version}_${os}_${arch}"
  archive="${folder}.${ext}"
  base_url="https://github.com/${REPO}/releases/download/${version}"

  tmp_dir="$(mktemp -d)"
  trap 'rm -rf "$tmp_dir"' EXIT

  log "Downloading ${BINARY} ${version} (${os}/${arch})..."
  fetch "${base_url}/${archive}" "${tmp_dir}/${archive}"
  fetch "${base_url}/checksums.txt" "${tmp_dir}/checksums.txt" || true

  log "Verifying SHA-256 checksum..."
  verify_checksum "$tmp_dir" "$archive"

  if [ "$ext" = "zip" ]; then
    need_cmd unzip
    unzip -q "${tmp_dir}/${archive}" -d "$tmp_dir"
  else
    need_cmd tar
    tar -xzf "${tmp_dir}/${archive}" -C "$tmp_dir"
  fi

  src_bin="${tmp_dir}/${folder}/${bin_name}"
  [ -f "$src_bin" ] || err "binary not found in archive at ${src_bin}"
  chmod +x "$src_bin"

  dest_dir="$(choose_install_dir)"
  mkdir -p "$dest_dir" 2>/dev/null || true
  dest_path="${dest_dir}/${bin_name}"

  if [ -w "$dest_dir" ]; then
    mv "$src_bin" "$dest_path"
  else
    log "Installing to ${dest_dir} (requires sudo)..."
    sudo mkdir -p "$dest_dir"
    sudo mv "$src_bin" "$dest_path"
  fi

  log "Installed ${BINARY} ${version} to ${dest_path}"
}

main "$@"
