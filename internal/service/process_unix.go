//go:build unix

package service

import (
	"fmt"
	"os"
	"syscall"
)

// signalGroup sends sig to pid's process group, falling back to the bare PID
// when the pgid lookup fails. Signalling the group ensures child workers
// (uvicorn/gunicorn/vLLM) die with the parent.
func signalGroup(pid int, sig syscall.Signal) error {
	if pgid, err := syscall.Getpgid(pid); err == nil && pgid > 0 {
		selfPGID, selfErr := syscall.Getpgid(os.Getpid())
		if selfErr == nil && pgid == selfPGID {
			return fmt.Errorf("refusing to signal shared process group %d for pid %d", pgid, pid)
		}
		return syscall.Kill(-pgid, sig)
	}
	return syscall.Kill(pid, sig)
}

func terminateGroup(pid int) error { return signalGroup(pid, syscall.SIGTERM) }
func killGroup(pid int) error      { return signalGroup(pid, syscall.SIGKILL) }
