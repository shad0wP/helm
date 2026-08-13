package service

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log"
	"net"
	"os/exec"
	"runtime"
	"strings"
	"sync"
	"time"
)

const (
	// probeTimeout bounds read-only state probes (`systemctl is-active`,
	// `docker inspect`) so an unresponsive daemon can never stall the poll loop.
	probeTimeout = 3 * time.Second
	// controlTimeout bounds start/stop commands, which legitimately take longer
	// than a probe but must still not hang the caller forever.
	controlTimeout = 30 * time.Second
)

// ServiceKind describes how a service is detected and controlled.
type ServiceKind string

const (
	KindSystemctl ServiceKind = "systemctl" // Linux only
	KindDocker    ServiceKind = "docker"    // both platforms
	KindPort      ServiceKind = "port"      // TCP probe, read-only by declaration
	KindProcess   ServiceKind = "process"   // TCP probe; stoppable via PID/supervisor
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
// auto-detected process service when its owning PID can be resolved.
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
		// Hermes is a terminal-launched agent: KindProcess makes it stoppable
		// via PID resolution. Port 9119 is a default — override the whole
		// entry in ~/.config/helm/services.json if yours differs.
		{ID: "hermes", Name: "Hermes Agent", Kind: KindProcess, Port: 9119, Icon: "robot", Color: "purple"},
		// OpenClaw is a Node-based agent gateway. Port 18789 is its default
		// gateway port — override in ~/.config/helm/services.json if yours
		// differs; discovery (signatures.json) also finds it on any port.
		{ID: "openclaw", Name: "OpenClaw", Kind: KindProcess, Port: 18789, Icon: "paw", Color: "pink"},
	}
}

// isRunningSystemctl returns true if `systemctl is-active <unit>` reports
// "active". (Linux only; on other platforms the binary is absent and this
// returns false.)
func isRunningSystemctl(unit string) bool {
	ctx, cancel := context.WithTimeout(context.Background(), probeTimeout)
	defer cancel()
	// is-active exits non-zero when the unit is inactive but still prints the
	// state to stdout, so we inspect the output rather than the exit code.
	out, _ := exec.CommandContext(ctx, "systemctl", "is-active", "--", unit).Output()
	return strings.TrimSpace(string(out)) == "active"
}

// isRunningUserSystemctl is the per-user (systemd --user) variant.
func isRunningUserSystemctl(unit string) bool {
	ctx, cancel := context.WithTimeout(context.Background(), probeTimeout)
	defer cancel()
	out, _ := exec.CommandContext(ctx, "systemctl", "--user", "is-active", "--", unit).Output()
	return strings.TrimSpace(string(out)) == "active"
}

// isRunningDocker returns true if the named container's state is "running".
func isRunningDocker(container string) bool {
	ctx, cancel := context.WithTimeout(context.Background(), probeTimeout)
	defer cancel()
	out, err := exec.CommandContext(ctx, "docker", "inspect", "--format", "{{.State.Status}}", "--", container).Output()
	if err != nil {
		return false
	}
	return strings.TrimSpace(string(out)) == "running"
}

// isRunningPort attempts a TCP dial (300ms timeout) to the port on both
// loopback stacks: 127.0.0.1 and [::1]. Either success counts — servers bound
// only to IPv6 loopback were previously invisible.
func isRunningPort(port int) bool {
	for _, host := range []string{"127.0.0.1", "[::1]"} {
		conn, err := net.DialTimeout("tcp", fmt.Sprintf("%s:%d", host, port), 300*time.Millisecond)
		if err == nil {
			_ = conn.Close()
			return true
		}
	}
	return false
}

// checkRunning resolves the live state of a service by its kind.
//
// KindSystemctl is layered: the system unit, the per-user unit, and finally
// the port are all consulted. A manually-run `ollama serve` (unit inactive,
// port open) therefore reads as running instead of stopped.
func checkRunning(s Service) bool {
	switch s.Kind {
	case KindSystemctl:
		if isRunningSystemctl(s.Unit) || isRunningUserSystemctl(s.Unit) {
			return true
		}
		return s.Port > 0 && isRunningPort(s.Port)
	case KindDocker:
		return isRunningDocker(s.Container)
	case KindPort, KindProcess:
		return isRunningPort(s.Port)
	}
	return false
}

// runControl executes one control command attempt under controlTimeout,
// capturing stderr so failures carry the tool's real complaint.
func runControl(argv []string) error {
	ctx, cancel := context.WithTimeout(context.Background(), controlTimeout)
	defer cancel()
	var stderr bytes.Buffer
	cmd := exec.CommandContext(ctx, argv[0], argv[1:]...)
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		if s := strings.TrimSpace(stderr.String()); s != "" {
			return fmt.Errorf("%s: %w: %s", strings.Join(argv, " "), err, s)
		}
		return fmt.Errorf("%s: %w", strings.Join(argv, " "), err)
	}
	return nil
}

// systemctlAttempts is the ordered argv chain tried for a system-unit action:
// plain systemctl first — polkit-governed, passwordless once the rule from
// `helm-cli setup-polkit` is installed — then non-interactive sudo for hosts
// using the NOPASSWD sudoers setup instead. Both attempts are non-interactive
// (--no-ask-password / -n) so a host with neither configured fails fast with
// a clear error instead of hanging on a hidden prompt.
func systemctlAttempts(action, unit string) [][]string {
	return [][]string{
		{"systemctl", "--no-ask-password", action, "--", unit},
		{"sudo", "-n", "systemctl", action, "--", unit},
	}
}

// CanStart and CanStop describe Helm's actual control capabilities. A raw
// process has no launch command, so it is stop-only while running; a port
// probe is always observational.
func CanStart(s Service) bool {
	return s.Kind == KindSystemctl || s.Kind == KindDocker
}

func CanStop(s Service) bool {
	return s.Kind == KindSystemctl || s.Kind == KindDocker || (s.Kind == KindProcess && s.Running)
}

// CanToggle reports whether the service's current state has a supported
// inverse operation.
func CanToggle(s Service) bool {
	if s.Running {
		return CanStop(s)
	}
	return CanStart(s)
}

// runSystemctl applies a system-unit action via the attempt chain above.
func runSystemctl(action, unit string) error {
	var errs []error
	for _, argv := range systemctlAttempts(action, unit) {
		err := runControl(argv)
		if err == nil {
			return nil
		}
		errs = append(errs, err)
	}
	return fmt.Errorf("%s %s failed (for passwordless control run `sudo helm-cli setup-polkit`, or see the README sudoers setup): %w",
		action, unit, errors.Join(errs...))
}

// controlService starts or stops a service. Port-kind services are read-only;
// process-kind services can be stopped (via PID/supervisor attribution) but
// not started — Helm has no way to know their launch command.
func controlService(s Service, start bool) error {
	action := "stop"
	if start {
		action = "start"
	}
	switch s.Kind {
	case KindSystemctl:
		if start {
			return runSystemctl(action, s.Unit)
		}
		// Stop routes to whoever actually runs it: system unit, user unit, or
		// — for a manually-launched process detected via the port — the owning
		// process itself (attributed through its cgroup before signalling).
		if isRunningSystemctl(s.Unit) {
			return runSystemctl("stop", s.Unit)
		}
		if isRunningUserSystemctl(s.Unit) {
			return runControl([]string{"systemctl", "--user", "stop", "--", s.Unit})
		}
		if s.Port > 0 && isRunningPort(s.Port) {
			return stopByPort(s.Port, s.Name)
		}
		return nil // nothing is running; stopping is a no-op
	case KindDocker:
		return runControl([]string{"docker", action, "--", s.Container})
	case KindProcess:
		if start {
			return fmt.Errorf("cannot start %s — Helm does not know its launch command; start it manually", s.Name)
		}
		return stopByPort(s.Port, s.Name)
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
	case KindProcess:
		proto = "process"
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
	mu        sync.RWMutex
	opMu      sync.Mutex
	services  []Service       // current snapshot; index stable between rebuilds
	onChange  func([]Service) // invoked after any poll cycle that changes state
	stop      chan struct{}   // closed to stop the polling goroutine
	startOnce sync.Once       // starts at most one polling goroutine
	stopOnce  sync.Once       // guards a single close of stop
	pollWG    sync.WaitGroup
}

// NewServiceManager builds the manager with an initial, fully-probed snapshot
// (known services plus any auto-detected ports).
func NewServiceManager() *ServiceManager {
	m := &ServiceManager{}
	m.rebuild()
	return m
}

// NewDeferredServiceManager returns the configured built-in/user service list
// immediately, without shelling out or probing sockets. The GUI uses this so
// the tray appears promptly, then calls Scan in a background goroutine. The
// CLI keeps using NewServiceManager because each invocation needs a complete
// synchronous snapshot before selecting an action.
func NewDeferredServiceManager() *ServiceManager {
	known := defaultServices()
	if user, err := loadUserServices(); err != nil {
		log.Printf("helm: ignoring user config: %v", err)
	} else if len(user) > 0 {
		known = mergeServices(known, user)
	}
	for i := range known {
		known[i].Meta = metaFor(known[i])
	}
	return &ServiceManager{services: known}
}

// SetOnChange registers the callback fired whenever the service state changes.
func (m *ServiceManager) SetOnChange(fn func([]Service)) {
	m.mu.Lock()
	m.onChange = fn
	m.mu.Unlock()
}

// rebuild reconstructs the service list from scratch: built-in defaults merged
// with the user's ~/.config/helm/services.json, plus auto-detection (the fixed
// AutoPorts probe list and live process discovery via listener enumeration).
func (m *ServiceManager) rebuild() {
	known := defaultServices()
	if user, err := loadUserServices(); err != nil {
		log.Printf("helm: ignoring user config: %v", err)
	} else if len(user) > 0 {
		known = mergeServices(known, user)
	}

	rebuilt := make([]Service, 0, len(known)+len(AutoPorts))
	// claimed suppresses auto-detection only for ports whose known service is
	// actually running — a known-but-stopped service must not mask a live
	// listener on its port (e.g. ollama unit down, manual `ollama serve` up).
	claimed := map[int]bool{}
	var probeWG sync.WaitGroup
	for i := range known {
		probeWG.Add(1)
		go func(index int) {
			defer probeWG.Done()
			known[index].Running = checkRunning(known[index])
		}(i)
	}
	probeWG.Wait()
	for i := range known {
		known[i].Meta = metaFor(known[i])
		if known[i].Port > 0 && known[i].Running {
			claimed[known[i].Port] = true
		}
		rebuilt = append(rebuilt, known[i])
	}

	// Fixed probe list. Discovered listeners are KindProcess — stoppable —
	// because an open port means a live local process.
	autoRunning := make([]bool, len(AutoPorts))
	for i, ap := range AutoPorts {
		if claimed[ap.Port] {
			continue
		}
		probeWG.Add(1)
		go func(index, port int) {
			defer probeWG.Done()
			autoRunning[index] = isRunningPort(port)
		}(i, ap.Port)
	}
	probeWG.Wait()
	for i, ap := range AutoPorts {
		if claimed[ap.Port] {
			continue
		}
		if autoRunning[i] {
			claimed[ap.Port] = true
			s := Service{
				ID:      fmt.Sprintf("auto_%d", ap.Port),
				Name:    ap.Name,
				Kind:    KindProcess,
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

	// Real discovery: enumerate listening sockets and surface any process
	// matching a known local-inference signature.
	rebuilt = append(rebuilt, discoverProcesses(claimed)...)

	m.mu.Lock()
	m.services = rebuilt
	m.mu.Unlock()
}

// refresh re-checks the running state of the current list in place and returns
// true if anything changed.
func (m *ServiceManager) refresh() bool {
	snapshot := m.GetServices()
	states := make([]bool, len(snapshot))
	var wg sync.WaitGroup
	for i := range snapshot {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			states[index] = checkRunning(snapshot[index])
		}(i)
	}
	wg.Wait()
	running := make(map[string]bool, len(snapshot))
	for i, s := range snapshot {
		running[s.ID] = states[i]
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	changed := false
	for i := range m.services {
		r, exists := running[m.services[i].ID]
		if !exists {
			continue
		}
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

// ensureStop lazily creates the stop channel and returns it. Callers must hold
// no lock; it manages m.mu internally.
func (m *ServiceManager) ensureStop() chan struct{} {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.stop == nil {
		m.stop = make(chan struct{})
	}
	return m.stop
}

// StartPolling re-checks every service on the given interval, firing onChange
// only on cycles where the state actually changed. The goroutine exits when
// StopPolling is called.
func (m *ServiceManager) StartPolling(interval time.Duration) {
	if interval <= 0 {
		log.Printf("helm: refusing non-positive polling interval %s", interval)
		return
	}
	stop := m.ensureStop()
	m.startOnce.Do(func() {
		m.pollWG.Add(1)
		go func() {
			defer m.pollWG.Done()
			ticker := time.NewTicker(interval)
			defer ticker.Stop()
			for {
				select {
				case <-stop:
					return
				case <-ticker.C:
					m.pollOnce()
				}
			}
		}()
	})
}

// StopPolling stops the polling goroutine for a graceful shutdown. It is safe to
// call multiple times and before StartPolling.
func (m *ServiceManager) StopPolling() {
	stop := m.ensureStop()
	m.stopOnce.Do(func() { close(stop) })
	m.pollWG.Wait()
}

// pollOnce performs a single poll cycle. It recovers from any panic in the
// state-change callback so one bad cycle can never silently kill the long-lived
// polling goroutine and freeze all future updates.
func (m *ServiceManager) pollOnce() {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("helm: recovered from panic during poll cycle: %v", r)
		}
	}()
	m.refreshAndNotify()
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
	m.opMu.Lock()
	defer m.opMu.Unlock()

	s, ok := m.find(id)
	if !ok {
		return fmt.Errorf("unknown service: %s", id)
	}
	if !CanToggle(s) {
		if s.Kind == KindProcess && !s.Running {
			return fmt.Errorf("cannot start %s — Helm does not know its launch command; start it manually", s.Name)
		}
		return fmt.Errorf("cannot control %s — started externally", s.Name)
	}
	if err := controlService(s, !s.Running); err != nil {
		return err
	}
	m.refresh()
	m.notify()
	return nil
}

// StartAll starts every controllable service that is currently stopped.
func (m *ServiceManager) StartAll() error { return m.bulk(true) }

// StopAll stops every controllable service that is currently running.
func (m *ServiceManager) StopAll() error { return m.bulk(false) }

func (m *ServiceManager) bulk(start bool) error {
	m.opMu.Lock()
	defer m.opMu.Unlock()

	// Every failure is reported (joined), not just the first — a partial
	// failure must name exactly which services misbehaved. No rollback.
	var errs []error
	for _, s := range m.GetServices() {
		if s.Running == start || (start && !CanStart(s)) || (!start && !CanStop(s)) {
			continue
		}
		if err := controlService(s, start); err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", s.Name, err))
		}
	}
	m.refresh()
	m.notify()
	return errors.Join(errs...)
}

// Scan re-runs auto-detection and rebuilds the services list, then notifies.
func (m *ServiceManager) Scan() error {
	m.opMu.Lock()
	defer m.opMu.Unlock()
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
