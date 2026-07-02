# Changelog

All notable changes to Helm are documented here. This file is maintained
automatically by [release-please](https://github.com/googleapis/release-please)
from [Conventional Commits](CONTRIBUTING.md) — do not edit released sections by
hand; new entries are prepended on release.

## [0.1.3] - 2026-06-28

macOS deployment readiness for a private, local install.

- **True menu-bar-only app.** `Info.plist` now sets `LSUIElement` — Helm runs as a macOS agent
  with **no Dock icon** and no app-switcher entry, a pure menu-bar experience.
- **Correct app bundle.** Bundle name (`Helm`), identifier, version, and copyright are no longer
  scaffold placeholders; the helm-wheel `icons.icns` is used directly (removed the stale Wails
  asset catalog), so packaging needs no manual icon post-step.
- **Automated ad-hoc signing.** `wails3 package` ad-hoc signs the bundle
  (`codesign --force --deep --sign -`) so it runs locally without a paid Apple Developer account
  or notarization.
- Audit confirmations (already in place): release binaries are stripped (`-ldflags="-s -w"`),
  tray icons are generated in code (cross-platform, no per-OS asset files), and there is no
  public artifact distribution / Homebrew tap (private repo, private releases).

## [0.1.2] - 2026-06-28

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

## [0.1.1] - 2026-06-27

- **Native Linux builds.** Official `linux/amd64` artifacts now ship with every release: an
  Arch / CachyOS package (`.pkg.tar.zst`), a Debian/Ubuntu `.deb`, a Fedora/RHEL `.rpm`, and a
  raw binary tarball — all built natively against GTK4 + WebKitGTK 6.0 in CI.
- **Verified on real Arch Linux.** CI now installs the Arch package with `pacman -U` and launches
  the app headless to confirm it runs end-to-end before a release is published.
- **Cleaner packages.** Corrected the Linux package metadata (maintainer, vendor, homepage,
  description) and gave the distro packages conventional, versioned filenames.

_No application code changed between v0.1.0 and v0.1.1 — this release is entirely about Linux
packaging and release automation._

## [0.1.0] - 2026-06-25

- Initial release: a universal macOS (Apple Silicon + Intel) menu-bar controller for a local AI
  stack, with an iOS-style popover and a real-time green/amber/grey tray indicator.
