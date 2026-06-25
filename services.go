package main

import (
	"fmt"
	"net"
	"os/exec"
	"runtime"
	"strings"
	"sync"
	"time"
)

// ServiceKind describes how a service is detected and controlled.
type ServiceKind string

const (
	KindSystemctl ServiceKind = "systemctl" // Linux only
	KindDocker    ServiceKind = "docker"    // both platforms
	KindPort      ServiceKind = "port"      // auto-detected via TCP probe, read-only
)

// Service is a single controllable (or observable) local AI service.
type Service struct {
	ID        string      // unique snake_case key e.g. "ollama"
	Name      string      // display name e.g. "Ollama"
	Kind      ServiceKind // how it is detected / controlled
	Unit      string      // systemctl unit name (KindSystemctl only)
	Container string      // docker container name (KindDocker only)
	Port      int         // primary port for display / port probing
	Running   bool        // current observed state
	Icon      string      // Tabler icon name without the "ti-" prefix
	Color     string      // "green" | "blue" | "amber" | "purple" | "gray" | "pink"
	Meta      string      // one-line status shown below the name in the card
	Auto      bool        // true = auto-detected, not in the hardcoded list
}

// AutoPorts is the list of additional ports probed by Scan(). Any port found
// open (and not already owned by a known service) is surfaced as an
// auto-detected, read-only service.
var AutoPorts = []struct {
	Port  int
	Name  string
	Icon  string
	Color string
}{
	{7860, "Gradio / SD WebUI", "photo-ai", "pink"},
	{8188, "ComfyUI", "nodes", "purple"},
	{8888, "Jupyter", "brand-python", "amber"},
	{6006, "TensorBoard", "chart-line", "blue"},
	{5000, "Flask / ML App", "api", "gray"},
	{11435, "Ollama (alt)", "cpu", "green"},
}

// isMacOS reports whether we are running on macOS.
func isMacOS() bool { return runtime.GOOS == "darwin" }

// defaultServices returns the ordered, hardcoded list of known services with
// platform-aware kinds. On macOS, Ollama is observed via its port (the user
// controls it through Ollama.app); on Linux it is a systemd unit.
func defaultServices() []Service {
	ollamaKind := KindSystemctl
	if isMacOS() {
		ollamaKind = KindPort
	}
	return []Service{
		{ID: "ollama", Name: "Ollama", Kind: ollamaKind, Unit: "ollama", Port: 11434, Icon: "cpu", Color: "green"},
		{ID: "open-webui", Name: "Open WebUI", Kind: KindDocker, Container: "open-webui", Port: 3000, Icon: "layout-dashboard", Color: "blue"},
		{ID: "searxng", Name: "SearXNG", Kind: KindDocker, Container: "searxng", Port: 8080, Icon: "search", Color: "amber"},
		{ID: "hermes", Name: "Hermes Agent", Kind: KindPort, Port: 9119, Icon: "robot", Color: "purple"},
	}
}

// isRunningSystemctl returns true if `systemctl is-active <unit>` reports
// "active". (Linux only; on other platforms the binary is absent and this
// returns false.)
func isRunningSystemctl(unit string) bool {
	// is-active exits non-zero when the unit is inactive but still prints the
	// state to stdout, so we inspect the output rather than the exit code.
	out, _ := exec.Command("systemctl", "is-active", unit).Output()
	return strings.TrimSpace(string(out)) == "active"
}

// isRunningDocker returns true if the named container's state is "running".
func isRunningDocker(container string) bool {
	out, err := exec.Command("docker", "inspect", "--format", "{{.State.Status}}", container).Output()
	if err != nil {
		return false
	}
	return strings.TrimSpace(string(out)) == "running"
}

// isRunningPort attempts a TCP dial to 127.0.0.1:<port> with a 300ms timeout.
func isRunningPort(port int) bool {
	conn, err := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", port), 300*time.Millisecond)
	if err != nil {
		return false
	}
	_ = conn.Close()
	return true
}

// checkRunning resolves the live state of a service by its kind.
func checkRunning(s Service) bool {
	switch s.Kind {
	case KindSystemctl:
		return isRunningSystemctl(s.Unit)
	case KindDocker:
		return isRunningDocker(s.Container)
	case KindPort:
		return isRunningPort(s.Port)
	}
	return false
}

// controlService starts or stops a service. Port-kind services are read-only.
func controlService(s Service, start bool) error {
	action := "stop"
	if start {
		action = "start"
	}
	switch s.Kind {
	case KindSystemctl:
		// Requires a NOPASSWD sudoers rule (see README) to run unattended.
		return exec.Command("sudo", "systemctl", action, s.Unit).Run()
	case KindDocker:
		return exec.Command("docker", action, s.Container).Run()
	default:
		return fmt.Errorf("cannot control %s — started externally", s.Name)
	}
}

// metaFor builds the one-line status string shown under a service name.
func metaFor(s Service) string {
	proto := "port"
	switch s.Kind {
	case KindDocker:
		proto = "docker"
	case KindSystemctl:
		proto = "systemd"
	}
	state := "stopped"
	if s.Running {
		state = "running"
	}
	if s.Port > 0 {
		return fmt.Sprintf("%s · :%d · %s", proto, s.Port, state)
	}
	return fmt.Sprintf("%s · %s", proto, state)
}

// ServiceManager owns the live snapshot of services and the change callback.
type ServiceManager struct {
	mu       sync.RWMutex
	services []Service       // current snapshot; index stable between rebuilds
	onChange func([]Service) // invoked after any poll cycle that changes state
}

// NewServiceManager builds the manager with an initial, fully-probed snapshot
// (known services plus any auto-detected ports).
func NewServiceManager() *ServiceManager {
	m := &ServiceManager{}
	m.rebuild()
	return m
}

// SetOnChange registers the callback fired whenever the service state changes.
func (m *ServiceManager) SetOnChange(fn func([]Service)) {
	m.mu.Lock()
	m.onChange = fn
	m.mu.Unlock()
}

// rebuild reconstructs the service list from scratch: every known service plus
// any open auto-detect port not already owned by a known service.
func (m *ServiceManager) rebuild() {
	known := defaultServices()
	rebuilt := make([]Service, 0, len(known)+len(AutoPorts))
	knownPorts := map[int]bool{}
	for i := range known {
		known[i].Running = checkRunning(known[i])
		known[i].Meta = metaFor(known[i])
		if known[i].Port > 0 {
			knownPorts[known[i].Port] = true
		}
		rebuilt = append(rebuilt, known[i])
	}
	for _, ap := range AutoPorts {
		if knownPorts[ap.Port] {
			continue
		}
		if isRunningPort(ap.Port) {
			s := Service{
				ID:      fmt.Sprintf("auto_%d", ap.Port),
				Name:    ap.Name,
				Kind:    KindPort,
				Port:    ap.Port,
				Running: true,
				Icon:    ap.Icon,
				Color:   ap.Color,
				Auto:    true,
			}
			s.Meta = metaFor(s)
			rebuilt = append(rebuilt, s)
		}
	}
	m.mu.Lock()
	m.services = rebuilt
	m.mu.Unlock()
}

// refresh re-checks the running state of the current list in place and returns
// true if anything changed.
func (m *ServiceManager) refresh() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	changed := false
	for i := range m.services {
		r := checkRunning(m.services[i])
		if r != m.services[i].Running {
			m.services[i].Running = r
			m.services[i].Meta = metaFor(m.services[i])
			changed = true
		}
	}
	return changed
}

// notify pushes the current snapshot to the registered callback, if any.
func (m *ServiceManager) notify() {
	m.mu.RLock()
	fn := m.onChange
	m.mu.RUnlock()
	if fn != nil {
		fn(m.GetServices())
	}
}

// refreshAndNotify re-checks state and fires onChange only when something moved.
func (m *ServiceManager) refreshAndNotify() {
	if m.refresh() {
		m.notify()
	}
}

// StartPolling re-checks every service on the given interval, firing onChange
// only on cycles where the state actually changed.
func (m *ServiceManager) StartPolling(interval time.Duration) {
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for range ticker.C {
			m.refreshAndNotify()
		}
	}()
}

// GetServices returns a thread-safe copy of the current snapshot.
func (m *ServiceManager) GetServices() []Service {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]Service, len(m.services))
	copy(out, m.services)
	return out
}

// find returns a copy of the service with the given ID.
func (m *ServiceManager) find(id string) (Service, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	for _, s := range m.services {
		if s.ID == id {
			return s, true
		}
	}
	return Service{}, false
}

// Toggle starts or stops the named service based on its current state.
func (m *ServiceManager) Toggle(id string) error {
	s, ok := m.find(id)
	if !ok {
		return fmt.Errorf("unknown service: %s", id)
	}
	if s.Kind == KindPort {
		return fmt.Errorf("cannot control %s — started externally", s.Name)
	}
	if err := controlService(s, !s.Running); err != nil {
		return err
	}
	m.refreshAndNotify()
	return nil
}

// StartAll starts every controllable service that is currently stopped.
func (m *ServiceManager) StartAll() error { return m.bulk(true) }

// StopAll stops every controllable service that is currently running.
func (m *ServiceManager) StopAll() error { return m.bulk(false) }

func (m *ServiceManager) bulk(start bool) error {
	var firstErr error
	for _, s := range m.GetServices() {
		if s.Kind == KindPort || s.Running == start {
			continue
		}
		if err := controlService(s, start); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	m.refreshAndNotify()
	return firstErr
}

// Scan re-runs auto-detection and rebuilds the services list, then notifies.
func (m *ServiceManager) Scan() error {
	m.rebuild()
	m.notify()
	return nil
}

// AggregateState returns "all", "some", or "none" describing how many services
// are running.
func AggregateState(services []Service) string {
	total, running := 0, 0
	for _, s := range services {
		total++
		if s.Running {
			running++
		}
	}
	switch {
	case total == 0 || running == 0:
		return "none"
	case running == total:
		return "all"
	default:
		return "some"
	}
}
