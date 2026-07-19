package gpu

import (
	"errors"
	"testing"
)

// fakeRunner mocks nvidia-smi: fixed output/error, and records whether
// Output was ever called so tests can prove the LookPath short-circuit.
type fakeRunner struct {
	out       string
	err       error
	available bool
	called    bool
}

func (f *fakeRunner) Output(name string, args ...string) (string, error) {
	f.called = true
	return f.out, f.err
}

func (f *fakeRunner) Available(name string) bool { return f.available }

func TestUsedMB(t *testing.T) {
	cases := map[string]struct {
		runner   fakeRunner
		want     int
		wantOK   bool
		wantCall bool
	}{
		"single GPU": {
			fakeRunner{out: "4500\n", available: true}, 4500, true, true,
		},
		"multi GPU sums": {
			fakeRunner{out: "4500\n2048\n", available: true}, 6548, true, true,
		},
		"whitespace tolerated": {
			fakeRunner{out: "  4500  \n\n", available: true}, 4500, true, true,
		},
		"nvidia-smi missing: silent bypass without exec": {
			fakeRunner{out: "4500\n", available: false}, 0, false, false,
		},
		"exec failure": {
			fakeRunner{err: errors.New("boom"), available: true}, 0, false, true,
		},
		"garbage output": {
			fakeRunner{out: "N/A\n", available: true}, 0, false, true,
		},
		"negative value rejected": {
			fakeRunner{out: "-5\n", available: true}, 0, false, true,
		},
		"empty output": {
			fakeRunner{out: "", available: true}, 0, false, true,
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			g := GPU{R: &tc.runner}
			got, ok := g.UsedMB()
			if got != tc.want || ok != tc.wantOK {
				t.Errorf("UsedMB() = %d, %v; want %d, %v", got, ok, tc.want, tc.wantOK)
			}
			if tc.runner.called != tc.wantCall {
				t.Errorf("Output called = %v, want %v (LookPath must gate execution)", tc.runner.called, tc.wantCall)
			}
		})
	}
}

func TestFormatDelta(t *testing.T) {
	cases := []struct {
		before, after int
		stopped       bool
		want          string
	}{
		{10000, 5500, true, "VRAM Cleared: 4500 MB"},
		{5500, 10000, false, "VRAM Allocated: 4500 MB"},
		{8000, 8000, true, "VRAM Cleared: 0 MB"},
		{8000, 8000, false, "VRAM Allocated: 0 MB"},
		// Sign is derived from the actual delta, not the action: a "stop"
		// that somehow grew VRAM must be reported honestly.
		{5000, 6000, true, "VRAM Allocated: 1000 MB"},
	}
	for _, tc := range cases {
		if got := FormatDelta(tc.before, tc.after, tc.stopped); got != tc.want {
			t.Errorf("FormatDelta(%d, %d, %v) = %q, want %q", tc.before, tc.after, tc.stopped, got, tc.want)
		}
	}
}
