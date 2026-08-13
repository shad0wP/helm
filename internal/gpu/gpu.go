// Package gpu reads NVIDIA VRAM usage via nvidia-smi, degrading silently
// when the tool is unavailable — core service toggling must still work on
// machines without NVIDIA tooling.
package gpu

import (
	"context"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

const commandTimeout = 3 * time.Second

// Runner abstracts subprocess execution and PATH lookup for tests.
type Runner interface {
	Output(ctx context.Context, name string, args ...string) (string, error)
	Available(name string) bool
}

// ExecRunner is the production Runner backed by os/exec.
type ExecRunner struct{}

// Output runs the command and returns its stdout.
func (ExecRunner) Output(ctx context.Context, name string, args ...string) (string, error) {
	out, err := exec.CommandContext(ctx, name, args...).Output()
	return string(out), err
}

// Available reports whether the binary exists on PATH.
func (ExecRunner) Available(name string) bool {
	_, err := exec.LookPath(name)
	return err == nil
}

// GPU reads VRAM telemetry over an injectable Runner.
type GPU struct{ R Runner }

// New returns a GPU wired to the real nvidia-smi.
func New() GPU { return GPU{R: ExecRunner{}} }

// UsedMB returns total VRAM in use across all GPUs in MiB. ok=false means
// nvidia-smi is missing or produced unusable output — callers bypass the
// delta report silently in that case.
func (g GPU) UsedMB() (used int, ok bool) {
	if !g.R.Available("nvidia-smi") {
		return 0, false
	}
	ctx, cancel := context.WithTimeout(context.Background(), commandTimeout)
	defer cancel()
	out, err := g.R.Output(ctx, "nvidia-smi", "--query-gpu=memory.used", "--format=csv,noheader,nounits")
	if err != nil {
		return 0, false
	}
	total, lines := 0, 0
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		n, err := strconv.Atoi(line) // one integer per GPU with nounits
		if err != nil || n < 0 {
			return 0, false
		}
		total += n
		lines++
	}
	if lines == 0 {
		return 0, false
	}
	return total, true
}

// FormatDelta renders the delta line per the TDD's exact output format.
// stopped selects which frame a zero delta reports under.
func FormatDelta(before, after int, stopped bool) string {
	delta := before - after
	switch {
	case delta > 0:
		return fmt.Sprintf("VRAM Cleared: %d MB", delta)
	case delta < 0:
		return fmt.Sprintf("VRAM Allocated: %d MB", -delta)
	case stopped:
		return "VRAM Cleared: 0 MB"
	default:
		return "VRAM Allocated: 0 MB"
	}
}
