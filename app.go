package main

import "github.com/wailsapp/wails/v3/pkg/application"

// App is the service exposed to the frontend. Every exported method is callable
// from JavaScript through the generated bindings.
type App struct {
	svc    *ServiceManager
	window application.Window
}

// GetServices returns the current service snapshot.
func (a *App) GetServices() []Service { return a.svc.GetServices() }

// Toggle starts or stops the named service.
func (a *App) Toggle(id string) error { return a.svc.Toggle(id) }

// StartAll starts every controllable, stopped service.
func (a *App) StartAll() error { return a.svc.StartAll() }

// StopAll stops every controllable, running service.
func (a *App) StopAll() error { return a.svc.StopAll() }

// Scan re-runs auto-detection and rebuilds the service list.
func (a *App) Scan() error { return a.svc.Scan() }

// HideWindow hides the popover panel (used by the footer close button).
func (a *App) HideWindow() {
	if a.window != nil {
		a.window.Hide()
	}
}
