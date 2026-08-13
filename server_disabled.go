//go:build server

package main

// Wails' server build tag replaces the native local-only boundary with an
// inbound HTTP/WebSocket surface. Helm deliberately fails closed if someone
// attempts that unsupported mode.
func init() {
	panic("Helm server mode is disabled: use the native tray application")
}
