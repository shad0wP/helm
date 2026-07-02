//go:build unix

package service

import "syscall"

// signalGroup sends sig to pid's process group, falling back to the bare PID
// when the pgid lookup fails. Signalling the group ensures child workers
// (uvicorn/gunicorn/vLLM) die with the parent.
func signalGroup(pid int, sig syscall.Signal) error {
	if pgid, err := syscall.Getpgid(pid); err == nil && pgid > 0 {
		return syscall.Kill(-pgid, sig)
	}
	return syscall.Kill(pid, sig)
}

func terminateGroup(pid int) error { return signalGroup(pid, syscall.SIGTERM) }
func killGroup(pid int) error      { return signalGroup(pid, syscall.SIGKILL) }
