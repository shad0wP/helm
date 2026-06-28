package main

import (
	"net"
	"testing"
)

// TestAppDelegatesToManager checks the bound App methods forward to the
// ServiceManager and that HideWindow is safe when no window is attached.
func TestAppDelegatesToManager(t *testing.T) {
	m := &ServiceManager{services: []Service{
		{ID: "hermes", Name: "Hermes", Kind: KindPort, Port: 9119, Running: false},
	}}
	a := &App{svc: m}

	got := a.GetServices()
	if len(got) != 1 || got[0].ID != "hermes" {
		t.Errorf("App.GetServices() = %+v, want one service 'hermes'", got)
	}

	if err := a.Toggle("hermes"); err == nil {
		t.Error("App.Toggle on a read-only port service = nil, want error")
	}
	if err := a.Toggle("missing"); err == nil {
		t.Error("App.Toggle on an unknown id = nil, want error")
	}
}

// TestHideWindowWithoutWindowIsSafe guards the nil-window branch (null input).
func TestHideWindowWithoutWindowIsSafe(t *testing.T) {
	a := &App{svc: &ServiceManager{}}
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("HideWindow panicked with no window attached: %v", r)
		}
	}()
	a.HideWindow() // window is nil -> must be a no-op
}

// TestAppStartStopScanDelegate exercises the remaining bound methods against a
// read-only service so no external command is ever run.
func TestAppStartStopScanDelegate(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer ln.Close()
	port := ln.Addr().(*net.TCPAddr).Port

	a := &App{svc: &ServiceManager{services: []Service{
		{ID: "p", Kind: KindPort, Port: port, Running: true},
	}}}
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
