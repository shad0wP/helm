# Helm

A native menu bar / system tray app for **macOS** and **Linux** (KDE / Arch) that
controls your local AI stack. Click the tray icon and a frameless popover slides open, listing
every detected local AI service. Flip a toggle to start or stop a
service; the tray icon turns **green** (all running), **amber** (some running), or **grey**
(all stopped) in real time.

Built with [Wails v3](https://v3.wails.io) — Go backend, vanilla HTML/CSS/JS frontend, zero
third-party Go dependencies beyond Wails itself.

## Screenshot

> _Screenshot placeholder — click the tray icon to reveal the 300×540 popover._
>
> ```
> ┌──────────────────────────────┐
> │ ⎈ Helm        [Start][Stop]   │
> │ ● 2 of 4 running              │
> ├──────────────────────────────┤
> │ SERVICES                      │
> │ 🖥  Ollama          [ ●——]     │
> │ ▦  Open WebUI       [——● ]     │
> │ 🔍 SearXNG          [ ●——]     │
> │ 🤖 Hermes Agent     [——● ]🔒   │
> │ ── AUTO-DETECTED ──           │
> │ 🐍 Jupyter          [——● ]🔒   │
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

### Download a release (recommended)

Prebuilt, signed-where-possible binaries are attached to the
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

Helm runs in the **menu bar / system tray** (no Dock or taskbar window). The Linux tray uses
StatusNotifierItem (SNI), supported natively by KDE Plasma.

### Build from source

Install the Wails v3 CLI, then the platform build dependencies:

```bash
go install github.com/wailsapp/wails/v3/cmd/wails3@latest
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

## Linux: passwordless service control (sudoers)

`systemctl`-managed services (Ollama, Docker) require `sudo`. To let Helm start/stop them
without a password prompt, install a sudoers drop-in **manually**:

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

| Service       | Linux       | macOS  | Unit / Container | Port  |
|---------------|-------------|--------|------------------|-------|
| Ollama        | systemctl   | port   | `ollama`         | 11434 |
| Open WebUI    | docker      | docker | `open-webui`     | 3000  |
| SearXNG       | docker      | docker | `searxng`        | 8080  |
| Hermes Agent  | port        | port   | —                | 9119  |

## Auto-detected ports

Click **Scan for services** to probe these additional ports. Any that respond are listed,
read-only, under **Auto-detected** (you can't toggle a process Helm didn't start):

| Port  | Service           | Icon           | Color  |
|-------|-------------------|----------------|--------|
| 7860  | Gradio / SD WebUI | `photo-ai`     | pink   |
| 8188  | ComfyUI           | `nodes`        | purple |
| 8888  | Jupyter           | `brand-python` | amber  |
| 6006  | TensorBoard       | `chart-line`   | blue   |
| 5000  | Flask / ML App    | `api`          | gray   |
| 11435 | Ollama (alt)      | `cpu`          | green  |

## How it works

- **Detection:** `systemctl is-active <unit>` (Linux), `docker inspect` container status, or a
  300 ms TCP dial to `127.0.0.1:<port>`. External commands run with a bounded timeout so an
  unresponsive daemon can never stall the app.
- **Polling:** every service is re-checked every **5 seconds**; the tray icon and popover only
  redraw when something actually changed.
- **Control:** `sudo systemctl start|stop` (Linux units) or `docker start|stop` (containers).
  Port-detected services are read-only and show _"Cannot control — started externally."_

## Changelog

### v0.1.2

Hardening, quality, and structure — no user-facing behaviour change.

- **Security.** Bundled the Tabler icon font locally (removed the runtime jsDelivr CDN
  dependency); added a Content-Security-Policy locking the webview to local origins; eliminated
  a latent DOM-XSS sink in the renderer (`innerHTML` → safe DOM construction + sanitisation);
  bumped `golang.org/x/sys` to clear advisory GO-2026-5024 / CVE-2026-39824; added a CI security
  gate that runs `govulncheck` + `npm audit` on every push and weekly.
- **Reliability.** All external `systemctl` / `docker` commands now run with bounded timeouts so
  an unresponsive daemon can never stall the app; the 5-second polling loop recovers from
  callback panics and shuts down gracefully.
- **Frontend.** Rewritten in strict TypeScript with explicit types and handled promise
  rejections.
- **Quality & structure.** Added a unit-test suite (~83% coverage) for the core logic, and
  consolidated the domain logic into `internal/service` and `internal/icon` packages.
- **Dependencies.** Updated to current versions (TypeScript 6, Tabler Icons 3.44, plus Go patch
  bumps).
- **Docs.** Rewrote the install section with prebuilt-release download steps and accurate
  build-from-source dependencies.

### v0.1.1

- **Native Linux builds.** Official `linux/amd64` artifacts now ship with every release: an
  Arch / CachyOS package (`.pkg.tar.zst`), a Debian/Ubuntu `.deb`, a Fedora/RHEL `.rpm`, and a
  raw binary tarball — all built natively against GTK4 + WebKitGTK 6.0 in CI.
- **Verified on real Arch Linux.** CI now installs the Arch package with `pacman -U` and launches
  the app headless to confirm it runs end-to-end before a release is published.
- **Cleaner packages.** Corrected the Linux package metadata (maintainer, vendor, homepage,
  description) and gave the distro packages conventional, versioned filenames.

_No application code changed between v0.1.0 and v0.1.1 — this release is entirely about Linux
packaging and release automation._

### v0.1.0

- Initial release: a universal macOS (Apple Silicon + Intel) menu-bar controller for a local AI
  stack, with an iOS-style popover and a real-time green/amber/grey tray indicator.

## License

MIT
