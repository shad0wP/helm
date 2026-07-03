//go:build !unix

package service

import "errors"

// On non-Unix platforms (Windows) process-group signalling is unsupported.
// Helm targets macOS and Linux; these stubs keep the package compilable
// everywhere for tooling/analysis.

var errUnsupported = errors.New("process signalling is not supported on this platform")

func terminateGroup(pid int) error { return errUnsupported }
func killGroup(pid int) error      { return errUnsupported }
