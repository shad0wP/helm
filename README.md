# Helm

A native menu bar / system tray app for **macOS** and **Linux** (KDE / CachyOS / Arch) that
controls your local AI stack. Click the tray icon and a frameless popover slides open, listing
every detected local AI service with an iOS-style toggle. Flip a toggle to start or stop a
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

- **Go 1.25+**
- **Wails v3 CLI** (`wails3`)
- **Node.js / npm** (the frontend is bundled with Vite)
- **Linux only:** GTK4 + WebKitGTK 6.0
- **Docker** (optional) — required only to control the Docker-based services
- **macOS:** Ollama is observed via its port (control it through Ollama.app)

## Install

### CachyOS / Arch (Linux)

```bash
# System dependencies
sudo pacman -S --needed webkit2gtk-4.1 webkitgtk-6.0 gtk4 base-devel go npm

# Wails v3 CLI
go install github.com/wailsapp/wails/v3/cmd/wails3@latest

# Verify
wails3 doctor

# Build
git clone <your-repo-url> helm && cd helm
wails3 build
./bin/helm
```

The system tray uses StatusNotifierItem (SNI), supported natively by KDE Plasma.

### macOS

```bash
# Toolchain (Homebrew)
brew install go
go install github.com/wailsapp/wails/v3/cmd/wails3@latest

# Verify
wails3 doctor

# Build the binary…
git clone <your-repo-url> helm && cd helm
wails3 build
./bin/helm

# …or build a double-clickable .app bundle:
wails3 package
open ./bin/helm.app
```

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

## Build from source

```bash
# Development (hot-reload, opens the popover and a live frontend dev server)
wails3 dev

# Production binary  →  ./bin/helm   (Linux & macOS)
wails3 build

# macOS .app bundle  →  ./bin/helm.app
wails3 package

# Optional Linux AppImage
wails3 task linux:create:appimage
```

> **Note:** Wails v3 emits the binary under `./bin/`. The frontend is compiled by Vite into
> `frontend/dist` and embedded into the binary, and the Go↔JS bindings are generated into
> `frontend/bindings` at build time.

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
  300 ms TCP dial to `127.0.0.1:<port>`.
- **Polling:** every service is re-checked every **5 seconds**; the tray icon and popover only
  redraw when something actually changed.
- **Control:** `sudo systemctl start|stop` (Linux units) or `docker start|stop` (containers).
  Port-detected services are read-only and show _"Cannot control — started externally."_

## License

MIT
