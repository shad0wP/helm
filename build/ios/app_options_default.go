//go:build !ios

package main

import "github.com/wailsapp/wails/v3/pkg/application"

// modifyOptionsForIOS is a no-op on non-iOS platforms. It is wired up by the
// Wails iOS build tooling (not present in this checkout, since Helm targets
// macOS and Linux only), so it is unreferenced here by design.
//
//lint:ignore U1000 Wails-generated iOS scaffold; called by iOS-only build tooling not in this checkout
func modifyOptionsForIOS(opts *application.Options) {
	// No modifications needed for non-iOS platforms
}
