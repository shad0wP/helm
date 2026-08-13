# Helm

[![Release](https://img.shields.io/github/v/release/shad0wP/helm?sort=semver)](https://github.com/shad0wP/helm/releases/latest)

A native menu bar / system tray app for **macOS** and **Linux** (KDE / Arch) that
controls your local AI stack. On macOS, clicking the menu-bar icon opens an anchored popover;
on Linux, KDE renders a native StatusNotifierItem menu at the cursor and **Show Helm…** opens a
normal movable window. Toggle systemd, Docker, and running local-inference processes while the
tray icon reports aggregate state in real time.

Built with [Wails v3](https://v3.wails.io) — Go backend, vanilla HTML/CSS/JS frontend, zero
third-party Go dependencies beyond Wails itself.

## Screenshot

> _Screenshot placeholder — macOS uses the compact popover; Linux uses a native tray menu and
> an optional decorated window._
>
> ```
> ┌──────────────────────────────┐
> │ ⎈ Helm        [Start][Stop]   │
> │ ● 2 of 5 running              │
> ├──────────────────────────────┤
> │ SERVICES                      │
> │ 🖥  Ollama          [ ●——]     │
> │ ▦  Open WebUI       [——● ]     │
> │ 🔍 SearXNG          [ ●——]     │
> │ 🤖 Hermes Agent     [ ●——]     │
> │ 🐾 OpenClaw         [——● ]     │
> │ ── AUTO-DETECTED ──           │
> │ 🐍 Jupyter          [——● ]     │
> ├──────────────────────────────┤
> │ ◎ Scan for services        ✕  │
> └──────────────────────────────┘
> ```

## Requirements

- **Running a prebuilt release** needs only the runtime libraries:
  - **Linux:** GTK4 + WebKitGTK 6.0 (`gtk4`, `webkitgtk-6.0`) — the distro packages pull these in automatically.
  - **macOS:** 12 (Monterey) or newer.
- **Building from source** additionally needs **Go 1.25+**, **Node.js / npm** (the frontend is bundled with Vite), and the **Wails v3 CLI** (`wails3`).
- **Docker** (optional) — only required to control the Docker-based services.
- On **macOS**, Ollama is observed via its port (control it through Ollama.app).

## Install

### One-line install (recommended)

```bash
curl -fsSL https://raw.githubusercontent.com/shad0wP/helm/main/install.sh | sh
```

Detects your OS — and, on Linux, your package manager (`pacman` / `apt` / `dnf`, falling back to
a raw binary if none is found) — downloads the matching asset from the
[latest release](https://github.com/shad0wP/helm/releases/latest), **verifies it against the
published `SHA256SUMS` before installing anything**, then installs and launches Helm. On Linux the
distro packages pull in GTK4 + WebKitGTK 6.0 automatically; on macOS it installs to
`/Applications` and clears the quarantine flag so Gatekeeper doesn't block the first launch.

The script is plain, unobfuscated POSIX `sh` — read it before piping to `sh` if you'd rather:
[`install.sh`](install.sh). To inspect first instead of piping directly:

```bash
curl -fsSL -o helm-install.sh https://raw.githubusercontent.com/shad0wP/helm/main/install.sh
less helm-install.sh && sh helm-install.sh
```

Helm runs in the **menu bar / system tray** (no Dock or taskbar window). The Linux tray uses
StatusNotifierItem (SNI), supported natively by KDE Plasma.

### Install manually

Prefer to pick the file yourself? Prebuilt, signed-where-possible binaries are attached to the
[latest GitHub release](https://github.com/shad0wP/helm/releases/latest). No build toolchain required.

**Linux (x86_64)** — pick the package for your distribution:

```bash
# Arch / CachyOS        (pulls in gtk4 + webkitgtk-6.0 automatically)
sudo pacman -U helm-*-linux-x86_64.pkg.tar.zst

# Debian / Ubuntu
sudo apt install ./helm-*-linux-amd64.deb

# Fedora / RHEL
sudo dnf install ./helm-*-linux-x86_64.rpm

# Any distro — raw binary (requires gtk4 + webkitgtk-6.0 already installed)
tar -xzf helm-*-linux-amd64.tar.gz && ./helm
```

**macOS (universal — Apple Silicon + Intel)**:

```bash
unzip Helm-*-macos-universal.app.zip
mv Helm.app /Applications/
# The build is ad-hoc signed (not notarized); clear the quarantine flag on first launch:
xattr -dr com.apple.quarantine /Applications/Helm.app
open /Applications/Helm.app
```

Verify any download against the published checksums:

```bash
sha256sum -c SHA256SUMS-linux.txt      # Linux
shasum -a 256 -c SHA256SUMS-macos.txt  # macOS
```

### Build from source

Install the Wails v3 CLI, then the platform build dependencies:

```bash
go install github.com/wailsapp/wails/v3/cmd/wails3@v3.0.0-alpha2.106
```

```bash
# Arch / CachyOS
sudo pacman -S --needed gtk4 webkitgtk-6.0 base-devel go npm

# Debian / Ubuntu (24.04+)
sudo apt install libgtk-4-dev libwebkitgtk-6.0-dev build-essential pkg-config golang npm

# macOS
brew install go node
```

Then build:

```bash
git clone https://github.com/shad0wP/helm && cd helm
wails3 doctor          # verify the toolchain (must report no errors)
wails3 build           # production binary → ./bin/helm
./bin/helm

wails3 dev             # …or run in dev mode with hot-reload
```

### Package for distribution

```bash
# macOS  → ./bin/Helm.app (ad-hoc signed)
wails3 package
# (universal arm64 + amd64 bundle: wails3 task darwin:package:universal)

# Linux  → ./bin/ : AppImage + .deb + .rpm + Arch .pkg.tar.zst
wails3 package
```

> **Note:** Wails v3 emits artifacts under `./bin/`. The frontend is compiled by Vite into
> `frontend/dist` and embedded into the binary; the Go↔JS bindings are generated into
> `frontend/bindings` at build time.

## The CLI: `helm-cli`

The same product, terminal-shaped. `helm-cli` ships alongside the tray app in every Linux
package and macOS tarball — same service engine, same defaults, same
`~/.config/helm/services.json`, same auto-discovery — so the stack can be toggled from
scripts, ssh sessions, and keybindings:

```
helm-cli               # toggle: stop if a startable service runs; otherwise start the stack
helm-cli on | off      # explicit start/stop of every controllable service
helm-cli status        # per-service state, aggregate state, VRAM in use
helm-cli free-vram     # evict all loaded Ollama models without stopping the daemon
helm-cli setup-polkit  # passwordless systemd control via polkit (run with sudo, Linux)
```

When `nvidia-smi` is present, toggling reports the VRAM delta (e.g. `VRAM Cleared: 4500 MB`);
without it, toggling works the same and the report is skipped.

## Linux: passwordless service control

For `systemctl`-managed system services (Ollama), Helm tries polkit-governed plain
`systemctl` first and falls back to non-interactive `sudo`. Pick **one** of these setups:

### Option A: polkit rule (recommended)

```bash
sudo helm-cli setup-polkit
```

This installs `/etc/polkit-1/rules.d/99-helm.rules`, a deliberately narrow rule: only the
`org.freedesktop.systemd1.manage-units` action, only the standard `wheel` or `sudo`
administrator groups, only the units in your config, and only `start`/`stop`/`restart`. The Linux packages install it automatically for
the default services; re-run the command after editing `services.json`. Removing the package
removes the rule.

### Option B: sudoers drop-in (manual)

Install a sudoers drop-in **manually**:

```bash
sudo nano /etc/sudoers.d/helm
```

Paste exactly (replace `<your-username>` with your login name, e.g. the output of `whoami`):

```
<your-username> ALL=(ALL) NOPASSWD: /usr/bin/systemctl start ollama, \
/usr/bin/systemctl stop ollama, \
/usr/bin/systemctl start docker, \
/usr/bin/systemctl stop docker
```

Save, then run `sudo visudo -c` to validate the file. Helm never writes this file for you.

## Services

Helm ships with a hardcoded, ordered list of known services. On macOS, Ollama is detected by
its port (rather than `systemctl`) and is therefore read-only.

| Service       | Linux       | macOS   | Unit / Container | Port  |
|---------------|-------------|---------|------------------|-------|
| Ollama        | systemctl   | port    | `ollama`         | 11434 |
| Open WebUI    | docker      | docker  | `open-webui`     | 3000  |
| SearXNG       | docker      | docker  | `searxng`        | 8080  |
| Hermes Agent  | process     | process | —                | 9119  |
| OpenClaw      | process     | process | —                | 18789 |

Hermes Agent and OpenClaw default to their conventional ports — override the entries in
`~/.config/helm/services.json` if yours differ; discovery also finds them on any port they
actually listen on.

## Auto-detected ports

Click **Scan for services** to probe these additional ports. Any listener that can be attributed
to a local process is listed under **Auto-detected** and can be stopped. Helm intentionally cannot
restart an unmanaged process because it does not know the original launch command.

| Port  | Service           | Icon           | Color  |
|-------|-------------------|----------------|--------|
| 7860  | Gradio / SD WebUI | `photo-ai`     | pink   |
| 8188  | ComfyUI           | `nodes`        | purple |
| 8888  | Jupyter           | `brand-python` | amber  |
| 6006  | TensorBoard       | `chart-line`   | blue   |
| 5000  | Flask / ML App    | `api`          | gray   |
| 11435 | Ollama (alt)      | `cpu`          | green  |

## Configuring your own services

Helm ships with sensible defaults, but you can declare your own local servers in
`~/.config/helm/services.json` (honours `$XDG_CONFIG_HOME`). JSON is used rather than TOML to
keep the zero-Go-dependency rule. Entries whose `id` matches a built-in override it; new ids are
appended.

```json
{
  "services": [
    { "id": "hermes", "name": "Hermes Agent", "kind": "process", "port": 9119, "icon": "robot", "color": "purple" },
    { "id": "my-vllm", "name": "vLLM", "kind": "process", "port": 8000 },
    { "id": "my-unit", "name": "Custom LLM", "kind": "systemctl", "unit": "myllm.service" }
  ]
}
```

`kind` is one of `systemctl` (needs `unit`), `docker` (needs `container`), `port` (read-only
probe), or `process` (probe **and** stop by resolving the owning PID). A `process` service you
started from a terminal can be stopped from Helm; a `port` service is display-only.

**Discovery.** "Scan for services" enumerates listening sockets (`ss` / `lsof`) and surfaces any
process matching a known inference signature (ollama, llama-server, vllm, sglang, koboldcpp,
tabbyAPI, text-generation, …) as a stoppable `process` service — so servers you launched by hand
show up without any config.

**Free VRAM.** The footer/menu "Free VRAM" action unloads all resident Ollama models
(`keep_alive: 0`) to reclaim GPU memory **without** stopping the daemon.

## Updates

Helm checks for new releases in the background (first check ~30 s after launch, then every 6 h)
and shows a dismissible banner when a newer version is published; there's also a **Check for
updates** button, and the installed version is shown in the footer. Downloads are **verified
against the release `SHA256SUMS`** and fail closed on any mismatch.

The update source is `https://api.github.com/repos/shad0wP/helm/releases/latest` — this repo is
**public**, so that's a plain unauthenticated GET; no token is ever embedded in the distributed
binary.

Updates are conservative: package-manager installs and the macOS `.app` are **never overwritten**.
Helm downloads the verified asset to `~/Downloads`; installation remains an explicit user action.

## How it works

- **Detection:** `systemctl is-active <unit>` (Linux), `docker inspect` container status, or a
  300 ms TCP dial to both `127.0.0.1:<port>` and `[::1]:<port>`. External commands run with a bounded timeout so an
  unresponsive daemon can never stall the app.
- **Polling:** every service is re-checked every **5 seconds**; the tray icon and UI only
  redraw when something actually changed.
- **Control:** polkit-governed `systemctl` with a non-interactive sudo fallback, Docker
  `start|stop`, or supervisor-aware process termination. Raw processes are stop-only; pure port
  probes remain read-only.

## Changelog

See [CHANGELOG.md](CHANGELOG.md). It is maintained automatically by release-please from
[Conventional Commits](CONTRIBUTING.md); every merge to `main` updates a Release PR, and merging
that PR tags the version and publishes a GitHub Release with artifacts.

## License

MIT
