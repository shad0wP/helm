# Changelog

All notable changes to Helm are documented here. This file is maintained
automatically by [release-please](https://github.com/googleapis/release-please)
from [Conventional Commits](CONTRIBUTING.md) — do not edit released sections by
hand; new entries are prepended on release.

## [0.3.1](https://github.com/shad0wP/helm/compare/v0.3.0...v0.3.1) (2026-08-13)


### Bug Fixes

* harden service control and release integrity ([c7fbde0](https://github.com/shad0wP/helm/commit/c7fbde017f85ccce11eb9a8e0006fcb0190cfa23))

## [0.3.0](https://github.com/shad0wP/helm/compare/v0.2.0...v0.3.0) (2026-07-19)


### Features

* **cli:** helm-cli — terminal face of the same product; OpenClaw; polkit ([ab5242c](https://github.com/shad0wP/helm/commit/ab5242cf68622cb3bd3d7a68de04bd60e0309fed))


### Bug Fixes

* **service:** harden parsers against malformed input; adversarial test sweep ([9f3bb94](https://github.com/shad0wP/helm/commit/9f3bb940ccff800c62a94102fe38a2f3f28ff1e6))

## [0.2.0](https://github.com/shad0wP/helm/compare/v0.1.3...v0.2.0) (2026-07-04)


### Features

* **install:** one-line installer script for macOS and Linux ([0478fe2](https://github.com/shad0wP/helm/commit/0478fe2e497997ec45fbef49f52700a44f22835d))
* **service:** embedded signature registry + first-run detection wizard ([d00217b](https://github.com/shad0wP/helm/commit/d00217bab35cbd466a435ed699fb3adaac23e3bb))
* **service:** stop terminal-launched LLMs; robust detection ([4505f20](https://github.com/shad0wP/helm/commit/4505f200b3eef86a9eadeb23898604588c1136c0))
* **update:** in-app update check, verified download, version embed ([c23508f](https://github.com/shad0wP/helm/commit/c23508fcdb0a6f06dc48e8bf705ce4533ab9dcd9))


### Bug Fixes

* **build:** suppress staticcheck false positive on iOS scaffold ([67db87a](https://github.com/shad0wP/helm/commit/67db87a2cc96718498b837702fe1ad92d49ca4c3))
* dedupe toast helpers in main.ts, resolve DefaultReleasesURL TODO ([5790315](https://github.com/shad0wP/helm/commit/5790315fb628f8bb4c1b3f7369795abb1e62c177))
* **install:** authenticate GitHub API metadata call when GITHUB_TOKEN is set ([5cb200c](https://github.com/shad0wP/helm/commit/5cb200c9915cebf4cb1bd88c94b18c616f00d655))
* **install:** skip sudo when already root; fix fallback-path CI test ([067dac7](https://github.com/shad0wP/helm/commit/067dac7fa6d57249122661be06fb424f0b126eb0))
* **release:** stop demoting feat commits to patch bumps ([788d40e](https://github.com/shad0wP/helm/commit/788d40edefa46c5386221fd7c909ba11c547f597))
* **tray:** native SNI menu + normal window on Linux; keep macOS popover ([a100ee6](https://github.com/shad0wP/helm/commit/a100ee69372eb5e823ab051640c70e135aa09b40))

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
