package main

import (
	"errors"
	"testing"
)

// TestUpdaterDownloadFailsClosedBeforeAnyCheck: Download verifies against the
// checksums URL captured by the most recent Check. Before any check has run
// there is nothing to verify against, and the updater must refuse — never
// fall back to an unverified download.
func TestUpdaterDownloadFailsClosedBeforeAnyCheck(t *testing.T) {
	u := newUpdater("dev")
	if _, err := u.Download("http://example.invalid/helm.tar.gz"); !errors.Is(err, errNoChecksums) {
		t.Errorf("Download before any check = %v, want errNoChecksums", err)
	}
}

// TestUpdaterStopIsIdempotent mirrors the service poller's lifecycle guarantee:
// Stop before Start, and repeated Stop, must never panic (double close).
func TestUpdaterStopIsIdempotent(t *testing.T) {
	u := newUpdater("dev")
	u.Stop()
	u.Stop()
}
