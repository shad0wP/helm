package update

import "strings"

// Channel is how this build was installed, which decides whether an in-place
// self-update is safe.
type Channel string

const (
	// ChannelAppImage: a Linux AppImage (self-updatable).
	ChannelAppImage Channel = "appimage"
	// ChannelManaged: installed by a system package manager under a system
	// prefix — never self-replace (the package manager owns those files).
	ChannelManaged Channel = "managed"
	// ChannelMacApp: a macOS .app bundle (download + reveal; no in-place swap
	// without notarization).
	ChannelMacApp Channel = "macapp"
	// ChannelStandalone: a writable standalone binary/tarball (swap-on-restart).
	ChannelStandalone Channel = "standalone"
)

// managedPrefixes are system locations owned by package managers.
var managedPrefixes = []string{"/usr/bin/", "/usr/local/bin/", "/usr/sbin/", "/opt/"}

// DetectChannel classifies the install from the executable path, the APPIMAGE
// env var, and GOOS. Pure; unit-tested. Callers pass runtime values so tests
// can exercise every branch.
func DetectChannel(exePath, appImageEnv, goos string) Channel {
	if appImageEnv != "" {
		return ChannelAppImage
	}
	for _, p := range managedPrefixes {
		if strings.HasPrefix(exePath, p) {
			return ChannelManaged
		}
	}
	if goos == "darwin" && strings.Contains(exePath, ".app/Contents/MacOS/") {
		return ChannelMacApp
	}
	return ChannelStandalone
}

// CanSelfReplace reports whether the channel supports swapping the running
// binary in place. Managed and macOS-app installs must not.
func (c Channel) CanSelfReplace() bool {
	return c == ChannelAppImage || c == ChannelStandalone
}
