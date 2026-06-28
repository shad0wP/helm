package main

import (
	"net"
	"runtime"
	"strconv"
	"strings"
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
	if len(svcs) != 4 {
		t.Fatalf("defaultServices() returned %d services, want 4", len(svcs))
	}
	wantIDs := []string{"ollama", "open-webui", "searxng", "hermes"}
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

func TestIsMacOS(t *testing.T) {
	if got, want := isMacOS(), runtime.GOOS == "darwin"; got != want {
		t.Errorf("isMacOS() = %v, want %v (GOOS=%s)", got, want, runtime.GOOS)
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
	for _, id := range []string{"ollama", "open-webui", "searxng", "hermes"} {
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
	for _, id := range []string{"ollama", "open-webui", "searxng", "hermes"} {
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
