package main

import "testing"

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
