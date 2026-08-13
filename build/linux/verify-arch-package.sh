#!/usr/bin/env bash

set -euo pipefail

package_dir="${HELM_PACKAGE_DIR:-/work/bin}"
shopt -s nullglob
packages=("${package_dir}"/*.pkg.tar.zst)

if [[ ${#packages[@]} -ne 1 ]]; then
  echo "FAIL: expected exactly one Arch package in ${package_dir}, found ${#packages[@]}" >&2
  exit 1
fi

# Tooling needed to install the package and run the native GUI headlessly.
pacman -Syu --noconfirm --needed \
  dbus iproute2 mesa ttf-dejavu xorg-server-xvfb

pacman -U --noconfirm "${packages[0]}"

echo "--- installed files ---"
pacman -Ql helm | grep -E 'bin/helm|\.desktop|\.png'
echo "--- declared dependencies ---"
pacman -Qi helm | grep -i 'depends on'

binary=/usr/local/bin/helm
test -x "${binary}"
ldd "${binary}"
if ldd "${binary}" | grep -qi 'not found'; then
  echo "FAIL: unresolved shared libraries" >&2
  exit 1
fi
echo "OK: all shared libraries resolved"

export WEBKIT_DISABLE_COMPOSITING_MODE=1
export WEBKIT_DISABLE_DMABUF_RENDERER=1
export LIBGL_ALWAYS_SOFTWARE=1
export GALLIUM_DRIVER=llvmpipe
export GDK_BACKEND=x11

# The single-quoted script is intentionally expanded by the child shell.
# shellcheck disable=SC2016
dbus-run-session -- xvfb-run -a -s "-screen 0 1280x1024x24" bash -c '
  set -euo pipefail

  helm > /tmp/helm.log 2>&1 &
  pid=$!
  cleanup() {
    kill -TERM "${pid}" 2>/dev/null || true
    wait "${pid}" 2>/dev/null || true
  }
  trap cleanup EXIT

  sleep 12
  if ! kill -0 "${pid}" 2>/dev/null; then
    echo "RESULT: FAIL - helm exited prematurely" >&2
    cat /tmp/helm.log || true
    exit 1
  fi

  if ss -ltnp | grep -F "pid=${pid},"; then
    echo "RESULT: FAIL - local-only Helm opened a TCP listener" >&2
    cat /tmp/helm.log || true
    exit 1
  fi

  echo "RESULT: PASS - helm launched and stayed alive for 12s (PID ${pid})"
  cat /tmp/helm.log || true
'
