package service

import (
	"net"
	"os"
	"testing"
)

// TestResolvePIDsForPortLive exercises the real lsof/ss path against a listener
// owned by this test process. Skips where those tools are unavailable/sandboxed
// rather than failing, so it never flakes in constrained CI.
func TestResolvePIDsForPortLive(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer ln.Close()
	port := ln.Addr().(*net.TCPAddr).Port

	pids := resolvePIDsForPort(port)
	self := os.Getpid()
	for _, p := range pids {
		if p == self {
			return // found our own listener — path works
		}
	}
	t.Skipf("resolvePIDsForPort(%d)=%v; own pid %d not found (lsof/ss unavailable?)", port, pids, self)
}

// TestResolvePIDsForPortNoToolingAvailable forces resolvePIDsForPort down its
// fallback path (and listListeners' own inner fallback) by hiding lsof/ss from
// PATH entirely — the real-world "neither tool is installed" case. It must
// degrade to an empty result, never panic or hang.
func TestResolvePIDsForPortNoToolingAvailable(t *testing.T) {
	t.Setenv("PATH", t.TempDir()) // an empty directory: no lsof, no ss, nothing
	got := resolvePIDsForPort(1)
	if len(got) != 0 {
		t.Errorf("resolvePIDsForPort with no tooling available = %v, want empty", got)
	}
}

func TestParseSSListeners(t *testing.T) {
	out := `LISTEN 0 4096  127.0.0.1:11434 0.0.0.0:* users:(("ollama",pid=612,fd=3))
LISTEN 0 511   [::1]:8188      [::]:*    users:(("python3",pid=999,fd=7))
LISTEN 0 128   0.0.0.0:22      0.0.0.0:*
LISTEN 0 4096  *:5432         *:*        users:(("postgres",pid=1200,fd=5))`
	got := parseSSListeners(out)
	if len(got) != 3 {
		t.Fatalf("got %d listeners, want 3 (line without users:() is skipped): %+v", len(got), got)
	}
	if got[0].Port != 11434 || got[0].PID != 612 || got[0].Command != "ollama" {
		t.Errorf("listener[0] = %+v, want ollama/612/11434", got[0])
	}
	if got[1].Port != 8188 || got[1].PID != 999 || got[1].Command != "python3" {
		t.Errorf("listener[1] = %+v, want python3/999/8188 (IPv6 local addr)", got[1])
	}
	if got[2].Port != 5432 || got[2].Command != "postgres" {
		t.Errorf("listener[2] = %+v, want postgres/5432 (wildcard addr)", got[2])
	}
}

func TestParseSSListenersEmpty(t *testing.T) {
	if got := parseSSListeners(""); len(got) != 0 {
		t.Errorf("parseSSListeners(empty) = %d, want 0", len(got))
	}
	if got := parseSSListeners("garbage\nmore garbage without colon"); len(got) != 0 {
		t.Errorf("parseSSListeners(garbage) = %d, want 0", len(got))
	}
}

func TestParseLsofListeners(t *testing.T) {
	out := `COMMAND   PID USER   FD   TYPE DEVICE SIZE/OFF NODE NAME
ollama    612 kn      3u  IPv4    0x1      0t0  TCP 127.0.0.1:11434 (LISTEN)
python3   999 kn      7u  IPv6    0x2      0t0  TCP [::1]:8188 (LISTEN)`
	got := parseLsofListeners(out)
	if len(got) != 2 {
		t.Fatalf("got %d, want 2: %+v", len(got), got)
	}
	if got[0].Port != 11434 || got[0].PID != 612 || got[0].Command != "ollama" {
		t.Errorf("listener[0] = %+v", got[0])
	}
	if got[1].Port != 8188 || got[1].PID != 999 {
		t.Errorf("listener[1] = %+v (IPv6)", got[1])
	}
}

func TestAttributeCgroup(t *testing.T) {
	tests := []struct {
		name       string
		content    string
		wantMethod string
		wantTarget string
	}{
		{
			"system unit",
			"0::/system.slice/ollama.service\n",
			"systemd", "ollama.service",
		},
		{
			"docker scope",
			"0::/system.slice/docker-3f1a2b3c4d5e6f7a8b9c0d1e2f3a4b5c.scope\n",
			"docker", "3f1a2b3c4d5e",
		},
		{
			"podman libpod scope",
			"0::/machine.slice/libpod-abcdef012345abcdef012345.scope\n",
			"docker", "abcdef012345",
		},
		{
			"user unit",
			"0::/user.slice/user-1000.slice/user@1000.service/app.slice/ollama.service\n",
			"systemd-user", "ollama.service",
		},
		{
			"bare user session (no real unit)",
			"0::/user.slice/user-1000.slice/session-2.scope\n",
			"process", "",
		},
		{
			"unmanaged / unknown",
			"0::/\n",
			"process", "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := attributeCgroup(tt.content)
			if got.Method != tt.wantMethod || got.Target != tt.wantTarget {
				t.Errorf("attributeCgroup = %+v, want method=%q target=%q", got, tt.wantMethod, tt.wantTarget)
			}
		})
	}
}

func TestMatchesInferenceSignature(t *testing.T) {
	yes := []string{
		"ollama", "llama-server", "vllm", "sglang",
		"koboldcpp", "python3 -m vllm.entrypoints.openai.api_server",
		"python -m http.server serve", "tabbyAPI", "text-generation-launcher",
	}
	no := []string{"", "sshd", "postgres", "nginx", "chrome", "code"}
	for _, c := range yes {
		if !matchesInferenceSignature(c) {
			t.Errorf("matchesInferenceSignature(%q) = false, want true", c)
		}
	}
	for _, c := range no {
		if matchesInferenceSignature(c) {
			t.Errorf("matchesInferenceSignature(%q) = true, want false", c)
		}
	}
}
