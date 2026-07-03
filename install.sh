#!/usr/bin/env sh
# Helm installer — detects your OS (and, on Linux, your package manager),
# downloads the correct release asset, verifies it against the published
# SHA256SUMS, and installs it.
#
#   curl -fsSL https://raw.githubusercontent.com/shad0wP/helm/main/install.sh | sh
#
# Read before piping to sh — this script is plain, unobfuscated POSIX sh:
# https://github.com/shad0wP/helm/blob/main/install.sh
#
# Prefer to inspect first? Download and run it yourself instead:
#   curl -fsSL -o helm-install.sh https://raw.githubusercontent.com/shad0wP/helm/main/install.sh
#   less helm-install.sh && sh helm-install.sh
#
# No third-party tools required beyond curl and (for Linux checksum
# verification) sha256sum/shasum, which every supported distro already ships.

set -eu

REPO="shad0wP/helm"
VERSION="${HELM_VERSION:-latest}"

log() { printf '\033[1;34m==>\033[0m %s\n' "$1" >&2; }
err() { printf '\033[1;31merror:\033[0m %s\n' "$1" >&2; exit 1; }
need() { command -v "$1" >/dev/null 2>&1 || err "required tool '$1' not found"; }

# maybe_sudo runs its arguments under sudo, unless we're already root (common
# in minimal containers, which often omit sudo entirely since it would be a
# no-op there) or sudo simply isn't installed.
maybe_sudo() {
  if [ "$(id -u)" = "0" ]; then
    "$@"
  elif command -v sudo >/dev/null 2>&1; then
    sudo "$@"
  else
    err "this step requires root privileges, and sudo is not available — re-run as root"
  fi
}

need curl

if [ "$VERSION" = "latest" ]; then
  API_URL="https://api.github.com/repos/$REPO/releases/latest"
else
  API_URL="https://api.github.com/repos/$REPO/releases/tags/$VERSION"
fi

log "Fetching release metadata ($VERSION)..."
RELEASE_JSON=$(curl -fsSL -H "Accept: application/vnd.github+json" "$API_URL") \
  || err "could not fetch release metadata from $API_URL"

TAG=$(printf '%s\n' "$RELEASE_JSON" | grep -m1 '"tag_name":' | sed -E 's/.*"tag_name": *"([^"]+)".*/\1/')
[ -n "$TAG" ] || err "could not determine the release tag from GitHub's response"
log "Release: $TAG"

WORKDIR=$(mktemp -d)
trap 'rm -rf "$WORKDIR"' EXIT
cd "$WORKDIR"

# asset_url PATTERN — finds the browser_download_url whose filename contains PATTERN.
asset_url() {
  printf '%s\n' "$RELEASE_JSON" \
    | grep '"browser_download_url":' \
    | sed -E 's/.*"browser_download_url": *"([^"]+)".*/\1/' \
    | grep -F "$1" \
    | head -n1
}

# download PATTERN — downloads the matching asset, prints its filename on stdout.
download() {
  url=$(asset_url "$1")
  [ -n "$url" ] || err "no release asset matching '$1' was found for $TAG"
  filename=$(basename "$url")
  log "Downloading $filename..."
  curl -fsSL -o "$filename" "$url"
  printf '%s' "$filename"
}

# verify FILE CHECKSUMS_PATTERN — verifies FILE against the matching SHA256SUMS asset.
verify() {
  file="$1"
  sums_url=$(asset_url "$2")
  [ -n "$sums_url" ] || err "no checksums asset matching '$2' was found for $TAG"
  curl -fsSL -o SUMS.txt "$sums_url"
  grep -F "$file" SUMS.txt > this.sum || err "no checksum entry for $file in $2"
  if command -v sha256sum >/dev/null 2>&1; then
    sha256sum -c this.sum || err "checksum verification FAILED for $file — refusing to install"
  elif command -v shasum >/dev/null 2>&1; then
    shasum -a 256 -c this.sum || err "checksum verification FAILED for $file — refusing to install"
  else
    err "neither sha256sum nor shasum is available to verify the download"
  fi
  log "Checksum verified."
}

OS=$(uname -s)
ARCH=$(uname -m)

case "$OS" in
  Darwin)
    ASSET=$(download "macos-universal.app.zip")
    verify "$ASSET" "SHA256SUMS-macos.txt"
    need unzip
    log "Installing to /Applications..."
    unzip -q -o "$ASSET" -d extracted
    [ -d extracted/Helm.app ] || err "unexpected archive layout: Helm.app not found after unzip"
    rm -rf "/Applications/Helm.app"
    mv extracted/Helm.app /Applications/
    # Ad-hoc signed, not notarized — clear quarantine so Gatekeeper doesn't
    # block the first launch. Best-effort: if xattr fails, the user just gets
    # the normal "unidentified developer" prompt on first open instead.
    xattr -dr com.apple.quarantine /Applications/Helm.app 2>/dev/null || true
    log "Installed. Launching Helm..."
    open /Applications/Helm.app
    log "Done — Helm is running in your menu bar."
    ;;

  Linux)
    case "$ARCH" in
      x86_64 | amd64) ;;
      *) err "unsupported architecture: $ARCH (Helm only publishes linux/amd64 binaries)" ;;
    esac

    if command -v pacman >/dev/null 2>&1; then
      ASSET=$(download "linux-x86_64.pkg.tar.zst")
      verify "$ASSET" "SHA256SUMS-linux.txt"
      log "Installing via pacman (gtk4 + webkitgtk-6.0 are pulled in automatically)..."
      maybe_sudo pacman -U --noconfirm "$ASSET"
    elif command -v apt-get >/dev/null 2>&1; then
      ASSET=$(download "linux-amd64.deb")
      verify "$ASSET" "SHA256SUMS-linux.txt"
      log "Installing via apt..."
      maybe_sudo apt-get install -y "./$ASSET"
    elif command -v dnf >/dev/null 2>&1; then
      ASSET=$(download "linux-x86_64.rpm")
      verify "$ASSET" "SHA256SUMS-linux.txt"
      log "Installing via dnf..."
      maybe_sudo dnf install -y "./$ASSET"
    else
      log "No supported package manager found (pacman/apt/dnf) — installing the raw binary instead."
      ASSET=$(download "linux-amd64.tar.gz")
      verify "$ASSET" "SHA256SUMS-linux.txt"
      mkdir -p "$HOME/.local/bin"
      tar -xzf "$ASSET" -C "$HOME/.local/bin"
      log "Installed to $HOME/.local/bin/helm"
      log "NOTE: this requires gtk4 and webkitgtk-6.0 to already be installed —"
      log "the distro packages (pacman/apt/dnf) pull these in for you automatically;"
      log "this raw-binary fallback does not."
      case ":$PATH:" in
        *":$HOME/.local/bin:"*) ;;
        *) log "Add $HOME/.local/bin to your PATH, then run: helm" ;;
      esac
    fi
    ;;

  *)
    err "unsupported OS: $OS (Helm supports macOS and Linux only)"
    ;;
esac
