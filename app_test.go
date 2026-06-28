package main

import (
	"testing"

	"helm/internal/service"
)

// The App is a thin delegation layer; these tests verify each bound method
// forwards to the ServiceManager. The manager's own edge-case behaviour
// (read-only services, unknown ids, …) is white-box-tested in internal/service.

func TestAppGetServicesAndToggleDelegate(t *testing.T) {
	a := &App{svc: &service.ServiceManager{}}

	got := a.GetServices()
	if got == nil {
		t.Error("App.GetServices() returned nil, want an (empty) slice")
	}
	if len(got) != 0 {
		t.Errorf("App.GetServices() on an empty manager = %d, want 0", len(got))
	}
	// Toggle forwards to the manager, which rejects an unknown id.
	if err := a.Toggle("missing"); err == nil {
		t.Error("App.Toggle on an unknown id = nil, want error")
	}
}

// TestHideWindowWithoutWindowIsSafe guards the nil-window branch (null input).
func TestHideWindowWithoutWindowIsSafe(t *testing.T) {
	a := &App{svc: &service.ServiceManager{}}
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("HideWindow panicked with no window attached: %v", r)
		}
	}()
	a.HideWindow() // window is nil -> must be a no-op
}

// TestAppStartStopScanDelegate exercises the remaining bound methods against an
// empty manager, so no external command is ever run.
func TestAppStartStopScanDelegate(t *testing.T) {
	a := &App{svc: &service.ServiceManager{}}
	if err := a.StartAll(); err != nil {
		t.Errorf("App.StartAll() = %v, want nil", err)
	}
	if err := a.StopAll(); err != nil {
		t.Errorf("App.StopAll() = %v, want nil", err)
	}
	if err := a.Scan(); err != nil {
		t.Errorf("App.Scan() = %v, want nil", err)
	}
}
