package service

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// This file implements KindProcess control: resolving which process owns a
// listening TCP port, attributing it to its supervisor (systemd / user
// systemd / docker) via cgroups so we stop it through the supervisor instead
// of racing a respawn, and — for genuinely unmanaged, terminal-launched
// processes — signalling the process group with SIGTERM, escalating to
// SIGKILL if the port stays open.
//
// Note on visibility: `ss`/`lsof` only report socket→PID for processes the
// caller can inspect. Other users' sockets need root, but the target use case
// (the user's own LLM servers, same uid) needs no privileges.

// stopGrace is how long after SIGTERM we wait (re-probing the port) before
// escalating to SIGKILL.
const stopGrace = 5 * time.Second

// listener is one listening TCP socket with its owning process, as reported
// by ss (Linux) or lsof (macOS).
type listener struct {
	Port    int
	PID     int
	Command string
}

// ssListenerRe extracts `users:(("cmd",pid=N,fd=M))` from `ss -tlnpH` output.
var ssListenerRe = regexp.MustCompile(`users:\(\("([^"]+)",pid=(\d+)`)

// parseSSListeners parses `ss -tlnpH` output. Each line looks like:
//
//	LISTEN 0 4096 127.0.0.1:11434 0.0.0.0:* users:(("ollama",pid=612,fd=3))
//	LISTEN 0 511  [::1]:8188      [::]:*    users:(("python3",pid=999,fd=7))
//
// Pure function; unit-tested against captured output.
func parseSSListeners(out string) []listener {
	var res []listener
	for _, line := range strings.Split(out, "\n") {
		fields := strings.Fields(line)
		if len(fields) < 4 {
			continue
		}
		// Local address is the 4th column with -H (no header): host:port.
		local := fields[3]
		idx := strings.LastIndex(local, ":")
		if idx < 0 {
			continue
		}
		port, err := strconv.Atoi(local[idx+1:])
		if err != nil || port < 1 || port > 65535 {
			continue
		}
		m := ssListenerRe.FindStringSubmatch(line)
		if m == nil {
			// Socket owned by another uid: no process info. Skip.
			continue
		}
		pid, err := strconv.Atoi(m[2])
		if err != nil {
			continue
		}
		res = append(res, listener{Port: port, PID: pid, Command: m[1]})
	}
	return res
}

// parseLsofListeners parses `lsof -nP -iTCP -sTCP:LISTEN` output. Lines:
//
//	COMMAND  PID USER FD TYPE DEVICE SIZE/OFF NODE NAME
//	ollama   612 kn   3u IPv4 0x0    0t0      TCP 127.0.0.1:11434 (LISTEN)
//
// Pure function; unit-tested against captured output.
func parseLsofListeners(out string) []listener {
	var res []listener
	for _, line := range strings.Split(out, "\n") {
		fields := strings.Fields(line)
		n := len(fields)
		if n < 9 || fields[0] == "COMMAND" {
			continue
		}
		// COMMAND may itself contain spaces ("LM Studio"), which shifts every
		// column when splitting on whitespace. The 7 columns PID..NODE plus the
		// address are fixed-width single fields, so anchor on the right-hand
		// end instead: NAME is "<addr> (LISTEN)" under -sTCP:LISTEN.
		addrIdx := n - 1
		if fields[n-1] == "(LISTEN)" {
			addrIdx = n - 2
		}
		pidIdx := addrIdx - 7
		if pidIdx < 1 {
			continue
		}
		pid, err := strconv.Atoi(fields[pidIdx])
		if err != nil {
			continue
		}
		name := fields[addrIdx]
		idx := strings.LastIndex(name, ":")
		if idx < 0 {
			continue
		}
		port, err := strconv.Atoi(name[idx+1:])
		if err != nil || port < 1 || port > 65535 {
			continue
		}
		res = append(res, listener{Port: port, PID: pid, Command: strings.Join(fields[:pidIdx], " ")})
	}
	return res
}

// listListeners enumerates listening TCP sockets with owning PIDs. Linux fast
// path is ss; lsof is the cross-platform fallback (and the macOS primary).
// Both run under the bounded-timeout pattern; a missing binary degrades to the
// other, then to an empty result.
func listListeners() []listener {
	ctx, cancel := context.WithTimeout(context.Background(), probeTimeout)
	defer cancel()
	if out, err := exec.CommandContext(ctx, "ss", "-tlnpH").Output(); err == nil {
		if ls := parseSSListeners(string(out)); len(ls) > 0 {
			return ls
		}
	}
	ctx2, cancel2 := context.WithTimeout(context.Background(), probeTimeout)
	defer cancel2()
	if out, err := exec.CommandContext(ctx2, "lsof", "-nP", "-iTCP", "-sTCP:LISTEN").Output(); err == nil {
		return parseLsofListeners(string(out))
	}
	return nil
}

// resolvePIDsForPort returns the PIDs listening on the given local TCP port.
// Primary: `lsof -t -iTCP:<port> -sTCP:LISTEN` (bare PIDs, works on macOS and
// most Linux). Fallback: parse `ss -tlnpH` for the port.
func resolvePIDsForPort(port int) []int {
	ctx, cancel := context.WithTimeout(context.Background(), probeTimeout)
	defer cancel()
	if out, err := exec.CommandContext(ctx, "lsof", "-t", fmt.Sprintf("-iTCP:%d", port), "-sTCP:LISTEN").Output(); err == nil {
		var pids []int
		for _, f := range strings.Fields(string(out)) {
			if pid, err := strconv.Atoi(f); err == nil {
				pids = append(pids, pid)
			}
		}
		if len(pids) > 0 {
			return pids
		}
	}
	var pids []int
	for _, l := range listListeners() {
		if l.Port == port {
			pids = append(pids, l.PID)
		}
	}
	return pids
}

// supervisor classifies who owns a process, derived from its cgroup.
type supervisor struct {
	Method string // "systemd" | "systemd-user" | "docker" | "process"
	Target string // unit name or container id ("" for raw process)
}

var (
	dockerScopeRe = regexp.MustCompile(`(?:docker|libpod)-([0-9a-f]{12,64})\.scope`)
	unitServiceRe = regexp.MustCompile(`/([^/]+\.service)`)
	userSliceRe   = regexp.MustCompile(`/user\.slice/`)
	systemSliceRe = regexp.MustCompile(`/system\.slice/`)
)

// attributeCgroup classifies /proc/<pid>/cgroup content (cgroup v2, the
// default on modern Arch). Routing a stop through the supervisor prevents
// "killed it and systemd/docker respawned it". Pure function; unit-tested.
func attributeCgroup(content string) supervisor {
	if m := dockerScopeRe.FindStringSubmatch(content); m != nil {
		// Container id: 12 chars is enough for docker/podman CLIs.
		id := m[1]
		if len(id) > 12 {
			id = id[:12]
		}
		return supervisor{Method: "docker", Target: id}
	}
	if userSliceRe.MatchString(content) {
		if u := lastServiceUnit(content); u != "" {
			return supervisor{Method: "systemd-user", Target: u}
		}
		return supervisor{Method: "process"}
	}
	if systemSliceRe.MatchString(content) {
		if u := lastServiceUnit(content); u != "" {
			return supervisor{Method: "systemd", Target: u}
		}
	}
	return supervisor{Method: "process"}
}

// lastServiceUnit returns the last "*.service" in a cgroup path that is not a
// per-user session manager scope (user@<uid>.service) — i.e. the actual unit
// the process belongs to, which appears after the session scope in the path.
func lastServiceUnit(content string) string {
	matches := unitServiceRe.FindAllStringSubmatch(content, -1)
	for i := len(matches) - 1; i >= 0; i-- {
		if u := matches[i][1]; !strings.HasPrefix(u, "user@") {
			return u
		}
	}
	return ""
}

// attributePID resolves the supervisor for a live PID. On Linux it reads
// /proc/<pid>/cgroup; anywhere that file is unavailable (macOS, cgroup v1
// oddities) it falls back to raw process signalling.
func attributePID(pid int) supervisor {
	content, err := os.ReadFile(fmt.Sprintf("/proc/%d/cgroup", pid))
	if err != nil {
		return supervisor{Method: "process"}
	}
	return attributeCgroup(string(content))
}

// Process-group signalling is platform-specific (see process_unix.go /
// process_other.go): terminateGroup sends SIGTERM and killGroup sends SIGKILL
// to the process group so child workers (uvicorn/gunicorn/vLLM) die with the
// parent.

// stopByPort stops whatever owns the given local TCP port:
//  1. resolve the owning PID(s);
//  2. attribute each to its supervisor and stop through it
//     (systemctl / systemctl --user / docker stop);
//  3. for unmanaged processes, SIGTERM the process group, re-probe the port
//     for stopGrace, then escalate to SIGKILL;
//  4. confirm by re-probing the port.
func stopByPort(port int, name string) error {
	pids := resolvePIDsForPort(port)
	if len(pids) == 0 {
		return fmt.Errorf("cannot stop %s: no process found listening on port %d (already stopped, or owned by another user)", name, port)
	}

	signalled := false
	for _, pid := range pids {
		sup := attributePID(pid)
		switch sup.Method {
		case "systemd":
			// Same polkit-first / sudo-fallback chain as controlService.
			if err := runSystemctl("stop", sup.Target); err != nil {
				return fmt.Errorf("stopping %s via systemd unit %s: %w", name, sup.Target, err)
			}
		case "systemd-user":
			if err := runControl([]string{"systemctl", "--user", "stop", sup.Target}); err != nil {
				return fmt.Errorf("stopping %s via user unit %s: %w", name, sup.Target, err)
			}
		case "docker":
			if err := runControl([]string{"docker", "stop", sup.Target}); err != nil {
				return fmt.Errorf("stopping %s via container %s: %w", name, sup.Target, err)
			}
		default:
			if err := terminateGroup(pid); err != nil {
				return fmt.Errorf("signalling %s (pid %d): %w", name, pid, err)
			}
			signalled = true
		}
	}

	if signalled {
		// Grace window: wait for the port to close, then escalate.
		deadline := time.Now().Add(stopGrace)
		for time.Now().Before(deadline) {
			if !isRunningPort(port) {
				return nil
			}
			time.Sleep(250 * time.Millisecond)
		}
		if isRunningPort(port) {
			for _, pid := range resolvePIDsForPort(port) {
				_ = killGroup(pid)
			}
			time.Sleep(300 * time.Millisecond)
		}
	}

	if isRunningPort(port) {
		return fmt.Errorf("stop of %s did not release port %d", name, port)
	}
	return nil
}

// discoverProcesses enumerates listening sockets and returns a KindProcess
// service for every process matching a known inference signature (see
// signatures.go / signatures.json), skipping ports already claimed by the
// given set. Matched services use the signature's display name/icon/color;
// the port is always the one the listener was actually found on.
func discoverProcesses(claimed map[int]bool) []Service {
	var out []Service
	seen := map[int]bool{}
	for _, l := range listListeners() {
		if claimed[l.Port] || seen[l.Port] {
			continue
		}
		sig, ok := matchSignature(l.Command)
		if !ok {
			continue
		}
		seen[l.Port] = true
		s := Service{
			ID:      fmt.Sprintf("proc_%d", l.Port),
			Name:    fmt.Sprintf("%s (:%d)", sig.Name, l.Port),
			Kind:    KindProcess,
			Port:    l.Port,
			Running: true,
			Icon:    sig.Icon,
			Color:   sig.Color,
			Auto:    true,
		}
		s.Meta = metaFor(s)
		out = append(out, s)
	}
	return out
}
