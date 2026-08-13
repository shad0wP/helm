package main

import (
	"helm/internal/service"
	"helm/internal/update"

	"github.com/wailsapp/wails/v3/pkg/application"
)

// App is the service exposed to the frontend. Every exported method is callable
// from JavaScript through the generated bindings. It is a thin delegation layer
// over the service.ServiceManager and the updater.
type App struct {
	svc     *service.ServiceManager
	updater *updater
	window  application.Window
}

// GetServices returns the current service snapshot.
func (a *App) GetServices() []service.Service { return a.svc.GetServices() }

// Toggle starts or stops the named service and returns the authoritative
// post-operation snapshot so optimistic UI state cannot drift from reality.
func (a *App) Toggle(id string) ([]service.Service, error) {
	err := a.svc.Toggle(id)
	return a.svc.GetServices(), err
}

// StartAll starts every controllable, stopped service.
func (a *App) StartAll() ([]service.Service, error) {
	err := a.svc.StartAll()
	return a.svc.GetServices(), err
}

// StopAll stops every controllable, running service.
func (a *App) StopAll() ([]service.Service, error) {
	err := a.svc.StopAll()
	return a.svc.GetServices(), err
}

// Scan re-runs auto-detection and rebuilds the service list.
func (a *App) Scan() ([]service.Service, error) {
	err := a.svc.Scan()
	return a.svc.GetServices(), err
}

// FreeVRAM unloads all models from the local Ollama instance (frees GPU memory
// without stopping the daemon) and returns how many models were evicted.
func (a *App) FreeVRAM() (int, error) { return a.svc.FreeVRAM() }

// HideWindow hides the popover panel (used by the footer close button).
func (a *App) HideWindow() {
	if a.window != nil {
		a.window.Hide()
	}
}

// GetVersion returns the embedded app version ("dev" for local builds).
func (a *App) GetVersion() string {
	if a.updater == nil {
		return version
	}
	return a.updater.version
}

// CheckForUpdate queries the release source and reports whether a newer version
// exists. A private/empty releases endpoint is reported as up-to-date, not an
// error.
func (a *App) CheckForUpdate() (update.Info, error) { return a.updater.Check() }

// DownloadUpdate downloads the given release asset to ~/Downloads (or temp) and
// verifies it against the release's published SHA256SUMS, returning the saved
// path. Fails closed on any checksum mismatch. It does not modify the running
// install — package-manager-owned installs are never overwritten (self-replace
// is out of scope; this is the safe download-and-reveal core).
func (a *App) DownloadUpdate(assetURL string) (string, error) {
	return a.updater.Download(assetURL)
}

// OpenReleasePage opens the given URL in the system browser.
func (a *App) OpenReleasePage(url string) error {
	return application.Get().Browser.OpenURL(url)
}
