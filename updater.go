package main

import (
	"context"
	"errors"
	"log"
	"os"
	"path/filepath"
	"sync"
	"time"

	"helm/internal/update"

	"github.com/wailsapp/wails/v3/pkg/application"
)

const (
	// updateInitialDelay defers the first check so launch isn't slowed.
	updateInitialDelay = 30 * time.Second
	// updateCheckInterval is the background re-check cadence.
	updateCheckInterval = 6 * time.Hour
)

// updater runs periodic update checks and exposes on-demand check/download to
// the App. It follows the service poller's lifecycle pattern (cancellable
// context + sync.Once + panic recovery).
type updater struct {
	version string
	baseURL string

	mu   sync.RWMutex
	last update.Info // most recent successful check
	tray *application.SystemTray

	ctx       context.Context
	cancel    context.CancelFunc
	startOnce sync.Once
	wg        sync.WaitGroup
}

func newUpdater(version string) *updater {
	ctx, cancel := context.WithCancel(context.Background())
	return &updater{
		version: version,
		baseURL: update.DefaultReleasesURL,
		ctx:     ctx,
		cancel:  cancel,
	}
}

// Start launches the background check loop and remembers the app for event
// emission and tray tooltip updates.
func (u *updater) Start(app *application.App) {
	u.startOnce.Do(func() {
		u.wg.Go(func() {
			timer := time.NewTimer(updateInitialDelay)
			defer timer.Stop()
			for {
				select {
				case <-u.ctx.Done():
					return
				case <-timer.C:
					u.checkOnce(app)
					timer.Reset(updateCheckInterval)
				}
			}
		})
	})
}

// Stop terminates the background loop (idempotent).
func (u *updater) Stop() {
	u.cancel()
	u.wg.Wait()
}

// checkOnce performs one background check, emitting update-available and
// updating the tray tooltip when a newer release exists. Panics are recovered
// so one bad cycle can't kill the loop.
func (u *updater) checkOnce(app *application.App) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("helm: recovered from panic during update check: %v", r)
		}
	}()
	info, err := u.Check()
	if err != nil {
		log.Printf("helm: update check failed: %v", err)
		return
	}
	if info.UpdateAvailable {
		app.Event.Emit("update-available", info)
		u.mu.RLock()
		tray := u.tray
		u.mu.RUnlock()
		if tray != nil {
			tray.SetTooltip("Helm — Local AI stack · update available")
		}
		log.Printf("helm: update available: %s (current %s)", info.LatestVersion, info.CurrentVersion)
	}
}

// setTray lets setupTray register the tray for tooltip updates. Guarded by
// u.mu: the background check goroutine reads u.tray, so the write must not
// rely on setupTray happening to run before Start's first timer fires.
func (u *updater) setTray(tray *application.SystemTray) {
	u.mu.Lock()
	u.tray = tray
	u.mu.Unlock()
}

// Check runs an update check now and caches the result.
func (u *updater) Check() (update.Info, error) {
	info, err := update.Check(u.ctx, u.baseURL, u.version)
	if err != nil {
		return info, err
	}
	u.mu.Lock()
	u.last = info
	u.mu.Unlock()
	return info, nil
}

// Download fetches assetURL to ~/Downloads (or temp), verifying it against the
// SHA256SUMS URL captured by the most recent check. Fails closed if no check
// has cached a checksums URL yet.
func (u *updater) Download(assetURL string) (string, error) {
	u.mu.RLock()
	sums := u.last.ChecksumsURL
	u.mu.RUnlock()
	if sums == "" {
		return "", errNoChecksums
	}
	return update.Download(u.ctx, assetURL, sums, downloadDir())
}

var errNoChecksums = errors.New("no published checksums available — run a check first")

// downloadDir returns ~/Downloads if it exists, else the OS temp dir.
func downloadDir() string {
	if home, err := os.UserHomeDir(); err == nil {
		d := filepath.Join(home, "Downloads")
		if fi, err := os.Stat(d); err == nil && fi.IsDir() {
			return d
		}
	}
	return os.TempDir()
}
