package service

import (
	"net"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

// freePort opens a loopback listener and returns it plus its port. The caller
// closes the listener (immediately, for a guaranteed-closed port).
func freePort(t *testing.T) (net.Listener, int) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("could not open loopback listener: %v", err)
	}
	return ln, ln.Addr().(*net.TCPAddr).Port
}

func TestIsRunningPortIPv6(t *testing.T) {
	// A server bound only to ::1 must be detected — the previous IPv4-only
	// probe missed these.
	ln, err := net.Listen("tcp", "[::1]:0")
	if err != nil {
		t.Skipf("no IPv6 loopback on this host: %v", err)
	}
	defer ln.Close()
	port := ln.Addr().(*net.TCPAddr).Port
	if !isRunningPort(port) {
		t.Errorf("isRunningPort(%d) = false for an IPv6-only listener, want true", port)
	}
}

func TestAggregateState(t *testing.T) {
	mk := func(states ...bool) []Service {
		out := make([]Service, len(states))
		for i, r := range states {
			out[i] = Service{ID: "s" + strconv.Itoa(i), Running: r}
		}
		return out
	}
	tests := []struct {
		name string
		in   []Service
		want string
	}{
		{"nil slice", nil, "none"},
		{"empty slice", []Service{}, "none"},
		{"single stopped", mk(false), "none"},
		{"single running", mk(true), "all"},
		{"all running", mk(true, true, true), "all"},
		{"all stopped", mk(false, false), "none"},
		{"boundary one running", mk(true, false, false), "some"},
		{"boundary one stopped", mk(true, true, false), "some"},
		{"half", mk(true, false), "some"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := AggregateState(tt.in); got != tt.want {
				t.Errorf("AggregateState(%d services) = %q, want %q", len(tt.in), got, tt.want)
			}
		})
	}
}

func TestMetaFor(t *testing.T) {
	tests := []struct {
		name string
		svc  Service
		want string
	}{
		{"docker running", Service{Kind: KindDocker, Port: 3000, Running: true}, "docker · :3000 · running"},
		{"docker stopped", Service{Kind: KindDocker, Port: 3000, Running: false}, "docker · :3000 · stopped"},
		{"systemctl running", Service{Kind: KindSystemctl, Port: 11434, Running: true}, "systemd · :11434 · running"},
		{"port running", Service{Kind: KindPort, Port: 9119, Running: true}, "port · :9119 · running"},
		{"zero port omits port", Service{Kind: KindPort, Port: 0, Running: false}, "port · stopped"},
		{"empty kind defaults to port", Service{Kind: "", Port: 1234, Running: true}, "port · :1234 · running"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := metaFor(tt.svc); got != tt.want {
				t.Errorf("metaFor(%+v) = %q, want %q", tt.svc, got, tt.want)
			}
		})
	}
}

func TestControlServicePortIsReadOnly(t *testing.T) {
	// A port-kind service is read-only and must return an error WITHOUT shelling out.
	err := controlService(Service{ID: "gradio", Name: "Gradio", Kind: KindPort, Port: 7860}, true)
	if err == nil {
		t.Fatal("expected an error controlling a port-kind service, got nil")
	}
	if !strings.Contains(err.Error(), "cannot control") {
		t.Errorf("unexpected error message: %q", err.Error())
	}
}

func TestToggleErrorPaths(t *testing.T) {
	m := &ServiceManager{services: []Service{
		{ID: "hermes", Name: "Hermes", Kind: KindPort, Port: 9119},
	}}
	cases := []struct {
		name string
		id   string
	}{
		{"unknown id", "does-not-exist"},
		{"empty id", ""},
		{"read-only port service", "hermes"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if err := m.Toggle(tc.id); err == nil {
				t.Errorf("Toggle(%q) = nil, want error", tc.id)
			}
		})
	}
}

func TestGetServicesReturnsIndependentCopy(t *testing.T) {
	m := &ServiceManager{services: []Service{
		{ID: "a", Running: true},
		{ID: "b", Running: false},
	}}
	snap := m.GetServices()
	if len(snap) != 2 {
		t.Fatalf("snapshot length = %d, want 2", len(snap))
	}
	// Mutating the returned slice (element + append) must not affect internal state.
	snap[0].Running = false
	snap = append(snap, Service{ID: "c"})
	_ = snap

	again := m.GetServices()
	if !again[0].Running {
		t.Error("internal Service was mutated through the returned snapshot")
	}
	if len(again) != 2 {
		t.Errorf("internal slice length changed to %d via append on the copy", len(again))
	}
}

func TestIsRunningPort(t *testing.T) {
	ln, port := freePort(t)
	defer ln.Close()
	if !isRunningPort(port) {
		t.Errorf("isRunningPort(%d) = false while a listener is active, want true", port)
	}

	// A port with no listener must read as not running.
	ln2, closedPort := freePort(t)
	ln2.Close()
	if isRunningPort(closedPort) {
		t.Errorf("isRunningPort(%d) = true with no listener, want false", closedPort)
	}

	// Port 0 is never a valid running service.
	if isRunningPort(0) {
		t.Error("isRunningPort(0) = true, want false")
	}
}

func TestRefreshReflectsLivePortAndDebounces(t *testing.T) {
	ln, port := freePort(t)
	m := &ServiceManager{services: []Service{
		{ID: "probe", Kind: KindPort, Port: port, Running: false},
	}}

	// stopped -> running
	if !m.refresh() {
		t.Fatal("refresh() = false, want true for stopped->running transition")
	}
	if !m.GetServices()[0].Running {
		t.Error("service not marked running after refresh with a live listener")
	}
	if meta := m.GetServices()[0].Meta; !strings.Contains(meta, "running") {
		t.Errorf("Meta = %q, want it to contain 'running'", meta)
	}

	// no change -> refresh must report false (avoids needless redraws)
	if m.refresh() {
		t.Error("refresh() = true on an unchanged state, want false")
	}

	// running -> stopped
	ln.Close()
	if !m.refresh() {
		t.Fatal("refresh() = false, want true for running->stopped transition")
	}
	if m.GetServices()[0].Running {
		t.Error("service still marked running after the listener was closed")
	}
}

func TestStopAllSkipsReadOnlyServices(t *testing.T) {
	ln, port := freePort(t)
	defer ln.Close()
	m := &ServiceManager{services: []Service{
		{ID: "probe", Kind: KindPort, Port: port, Running: true},
	}}
	// The only running service is read-only; StopAll must skip it and not error.
	if err := m.StopAll(); err != nil {
		t.Errorf("StopAll() = %v, want nil", err)
	}
	if !m.GetServices()[0].Running {
		t.Error("a read-only port service should remain running after StopAll")
	}
}

func TestDefaultServices(t *testing.T) {
	svcs := defaultServices()
	if len(svcs) != 5 {
		t.Fatalf("defaultServices() returned %d services, want 5", len(svcs))
	}
	wantIDs := []string{"ollama", "open-webui", "searxng", "hermes", "openclaw"}
	for i, id := range wantIDs {
		if svcs[i].ID != id {
			t.Errorf("service[%d].ID = %q, want %q (order is significant)", i, svcs[i].ID, id)
		}
		if svcs[i].Auto {
			t.Errorf("known service %q must not be marked Auto", svcs[i].ID)
		}
	}
	// Ollama detection is platform-aware: systemd unit on Linux, port probe on macOS.
	wantKind := KindSystemctl
	if isMacOS() {
		wantKind = KindPort
	}
	if svcs[0].Kind != wantKind {
		t.Errorf("ollama Kind = %q, want %q on %s", svcs[0].Kind, wantKind, runtime.GOOS)
	}
}

func TestScanPopulatesKnownServicesAndNotifies(t *testing.T) {
	m := &ServiceManager{}
	calls := 0
	var got []Service
	m.SetOnChange(func(s []Service) {
		calls++
		got = s
	})

	if err := m.Scan(); err != nil {
		t.Fatalf("Scan() = %v, want nil", err)
	}
	if calls != 1 {
		t.Errorf("onChange invoked %d times, want exactly 1", calls)
	}
	if len(got) == 0 {
		t.Error("onChange received an empty snapshot")
	}

	byID := map[string]Service{}
	for _, s := range m.GetServices() {
		byID[s.ID] = s
	}
	for _, id := range []string{"ollama", "open-webui", "searxng", "hermes", "openclaw"} {
		s, ok := byID[id]
		if !ok {
			t.Errorf("Scan result is missing known service %q", id)
			continue
		}
		if s.Auto {
			t.Errorf("known service %q must not be marked Auto", id)
		}
	}
}

func TestCheckRunningByKind(t *testing.T) {
	ln, port := freePort(t)
	defer ln.Close()
	tests := []struct {
		name string
		svc  Service
		want bool
	}{
		{"live port", Service{Kind: KindPort, Port: port}, true},
		// Read-only probes for units/containers that cannot exist. `systemctl
		// is-active` and `docker inspect` only read state, never mutate it.
		{"dead systemctl unit", Service{Kind: KindSystemctl, Unit: "helm-nonexistent-xyz.service"}, false},
		{"missing docker container", Service{Kind: KindDocker, Container: "helm-nonexistent-xyz"}, false},
		{"unknown kind", Service{Kind: "bogus"}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := checkRunning(tt.svc); got != tt.want {
				t.Errorf("checkRunning(%s) = %v, want %v", tt.name, got, tt.want)
			}
		})
	}
}

func TestNewServiceManager(t *testing.T) {
	m := NewServiceManager()
	if m == nil {
		t.Fatal("NewServiceManager() = nil")
	}
	svcs := m.GetServices()
	if len(svcs) < 4 {
		t.Fatalf("expected at least the 4 known services, got %d", len(svcs))
	}
	ids := map[string]bool{}
	for _, s := range svcs {
		ids[s.ID] = true
	}
	for _, id := range []string{"ollama", "open-webui", "searxng", "hermes", "openclaw"} {
		if !ids[id] {
			t.Errorf("NewServiceManager() is missing known service %q", id)
		}
	}
}

func TestRefreshAndNotifyOnlyFiresOnChange(t *testing.T) {
	ln, port := freePort(t)
	defer ln.Close()
	m := &ServiceManager{services: []Service{
		{ID: "p", Kind: KindPort, Port: port, Running: false},
	}}
	calls := 0
	m.SetOnChange(func([]Service) { calls++ })

	m.refreshAndNotify() // stopped -> running: must notify
	if calls != 1 {
		t.Errorf("onChange calls = %d after a state change, want 1", calls)
	}
	m.refreshAndNotify() // unchanged: must NOT notify
	if calls != 1 {
		t.Errorf("onChange calls = %d with no change, want 1", calls)
	}
}

func TestStartStopAllReadOnlyAreNoOps(t *testing.T) {
	ln, port := freePort(t)
	defer ln.Close()
	m := &ServiceManager{services: []Service{
		{ID: "p", Kind: KindPort, Port: port, Running: true},
	}}
	if err := m.StartAll(); err != nil {
		t.Errorf("StartAll() = %v, want nil (only a read-only service)", err)
	}
	if err := m.StopAll(); err != nil {
		t.Errorf("StopAll() = %v, want nil (only a read-only service)", err)
	}
}

func TestStartPollingDetectsChange(t *testing.T) {
	ln, port := freePort(t)
	defer ln.Close()
	m := &ServiceManager{services: []Service{
		{ID: "p", Kind: KindPort, Port: port, Running: false},
	}}
	// Non-blocking send so the leaked ticker goroutine never blocks after the test.
	notified := make(chan struct{}, 4)
	m.SetOnChange(func([]Service) {
		select {
		case notified <- struct{}{}:
		default:
		}
	})

	m.StartPolling(10 * time.Millisecond)
	defer m.StopPolling() // graceful shutdown: stop the goroutine when done
	select {
	case <-notified:
		// The first poll observed the stopped->running transition.
	case <-time.After(2 * time.Second):
		t.Fatal("StartPolling did not invoke onChange within 2s")
	}
	if !m.GetServices()[0].Running {
		t.Error("polling did not update the service to running")
	}
}

func TestStopPollingIsSafeAndIdempotent(t *testing.T) {
	m := &ServiceManager{}
	// Safe before StartPolling and when called repeatedly (stopOnce guards close).
	m.StopPolling()
	m.StopPolling()
	// Starting after a stop must not hang or leak: the goroutine sees a closed
	// stop channel and returns immediately.
	m.StartPolling(10 * time.Millisecond)
}

// TestSystemctlAttemptsOrder pins the privilege chain for system units:
// plain polkit-governed systemctl first, non-interactive sudo second — and
// both must be non-interactive so a misconfigured host fails fast instead of
// hanging on a hidden password prompt.
func TestSystemctlAttemptsOrder(t *testing.T) {
	got := systemctlAttempts("stop", "ollama")
	want := [][]string{
		{"systemctl", "--no-ask-password", "stop", "ollama"},
		{"sudo", "-n", "systemctl", "stop", "ollama"},
	}
	if len(got) != len(want) {
		t.Fatalf("attempts = %v, want %v", got, want)
	}
	for i := range want {
		if strings.Join(got[i], " ") != strings.Join(want[i], " ") {
			t.Errorf("attempt[%d] = %v, want %v", i, got[i], want[i])
		}
	}
}

// TestBulkAggregatesAllFailures: a partial failure must name every service
// that misbehaved, not just the first (errors.Join). Two stoppable process
// services with no lsof/ss on PATH deterministically fail to stop.
func TestBulkAggregatesAllFailures(t *testing.T) {
	t.Setenv("PATH", t.TempDir()) // no lsof, no ss -> stopByPort fails for both
	m := &ServiceManager{services: []Service{
		{ID: "a", Name: "Alpha", Kind: KindProcess, Port: 1, Running: true},
		{ID: "b", Name: "Beta", Kind: KindProcess, Port: 2, Running: true},
	}}
	err := m.StopAll()
	if err == nil {
		t.Fatal("StopAll() = nil, want an aggregated error")
	}
	for _, name := range []string{"Alpha", "Beta"} {
		if !strings.Contains(err.Error(), name) {
			t.Errorf("aggregated error %q does not mention %s", err.Error(), name)
		}
	}
}

// TestPollOnceSurvivesPanickingCallback: the recover() in pollOnce exists so a
// bad onChange callback can never kill the long-lived poll loop. Prove not
// just that the panic is swallowed, but that the manager stays fully usable
// afterwards — state updates and later notifications must still work.
func TestPollOnceSurvivesPanickingCallback(t *testing.T) {
	ln, port := freePort(t)
	m := &ServiceManager{services: []Service{
		{ID: "p", Kind: KindPort, Port: port, Running: false},
	}}
	m.SetOnChange(func([]Service) { panic("callback exploded") })

	m.pollOnce() // stopped->running fires the panicking callback; must not propagate
	if !m.GetServices()[0].Running {
		t.Fatal("state update was lost when the callback panicked")
	}

	calls := 0
	m.SetOnChange(func([]Service) { calls++ })
	ln.Close()
	m.pollOnce() // running->stopped: the replacement callback must fire normally
	if calls != 1 {
		t.Errorf("onChange calls after recovery = %d, want 1", calls)
	}
	if m.GetServices()[0].Running {
		t.Error("service still marked running after the listener closed")
	}
}

// TestServiceManagerConcurrentAccess hammers every public entry point from
// multiple goroutines while the poller runs, under -race. This is the guard
// against locking regressions in refresh/GetServices/SetOnChange/find; the
// final assertion proves the snapshot survives uncorrupted.
func TestServiceManagerConcurrentAccess(t *testing.T) {
	ln, port := freePort(t)
	ln.Close() // guaranteed-closed port: probes fail fast, no state flapping
	m := &ServiceManager{services: []Service{
		{ID: "x", Kind: KindPort, Port: port},
	}}
	m.SetOnChange(func([]Service) {})
	m.StartPolling(time.Millisecond)

	var wg sync.WaitGroup
	for w := 0; w < 8; w++ {
		wg.Add(1)
		go func(w int) {
			defer wg.Done()
			for i := 0; i < 200; i++ {
				switch (w + i) % 5 {
				case 0:
					_ = m.GetServices()
				case 1:
					m.SetOnChange(func([]Service) {})
				case 2:
					_ = m.refresh()
				case 3:
					_, _ = m.find("x")
				case 4:
					_ = m.Toggle("missing") // error path only; no side effects
				}
			}
		}(w)
	}
	wg.Wait()
	m.StopPolling()

	got := m.GetServices()
	if len(got) != 1 || got[0].ID != "x" {
		t.Errorf("snapshot corrupted after concurrent access: %+v", got)
	}
}
