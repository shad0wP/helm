package main

import (
	"testing"

	"helm/internal/service"
)

// The CLI is a thin routing layer over service.ServiceManager (which has its
// own suite); these tests pin the routing contract without touching systemctl.
func TestRunRouting(t *testing.T) {
	for _, args := range [][]string{{"help"}, {"-h"}, {"--help"}, {"version"}} {
		if code := run(args); code != 0 {
			t.Errorf("run(%v) = %d, want 0", args, code)
		}
	}
	if code := run([]string{"bogus"}); code != 2 {
		t.Errorf("run(bogus) = %d, want 2 (unknown command)", code)
	}
}

func TestShouldStartIgnoresReadOnlyAndStopOnlyServices(t *testing.T) {
	services := []service.Service{
		{Kind: service.KindPort, Running: true},
		{Kind: service.KindProcess, Running: true},
		{Kind: service.KindDocker, Running: false},
	}
	if !shouldStart(services) {
		t.Fatal("read-only/stop-only services prevented a stopped startable stack from starting")
	}
	services[2].Running = true
	if shouldStart(services) {
		t.Fatal("running Docker service did not select the stop action")
	}
}
