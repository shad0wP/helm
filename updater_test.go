package main

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
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
// Stop before Start, and repeated Stop, must remain safe and non-blocking.
func TestUpdaterStopIsIdempotent(t *testing.T) {
	u := newUpdater("dev")
	u.Stop()
	u.Start(nil) // a late start observes the canceled context and exits
	u.Stop()
}

func TestUpdaterStopCancelsInFlightCheck(t *testing.T) {
	started := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(started)
		<-r.Context().Done()
	}))
	defer srv.Close()

	u := newUpdater("dev")
	u.baseURL = srv.URL
	done := make(chan error, 1)
	go func() {
		_, err := u.Check()
		done <- err
	}()
	<-started
	u.Stop()

	select {
	case err := <-done:
		if err == nil {
			t.Fatal("canceled update check returned nil error")
		}
	case <-time.After(time.Second):
		t.Fatal("Stop did not cancel the in-flight update request")
	}
}
