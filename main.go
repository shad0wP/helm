package main

import (
	"context"
	"embed"
	"log"
	"sync"

	"helm/internal/service"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"
)

// Wails embeds the built frontend (frontend/dist) into the binary.
//
//go:embed all:frontend/dist
var assets embed.FS

// version is the embedded app version, injected at build time via
// -ldflags "-X main.version=<v>" (from version.txt). "dev" for local builds.
var version = "dev"

func main() {
	lifecycleCtx, cancelLifecycle := context.WithCancel(context.Background())
	var startupWG sync.WaitGroup
	started := make(chan struct{})
	var startedOnce sync.Once
	svc := service.NewDeferredServiceManager()
	updater := newUpdater(version)
	bound := &App{svc: svc, updater: updater}

	app := application.New(application.Options{
		Name:        "Helm",
		Description: "Local AI stack controller",
		Services: []application.Service{
			application.NewService(bound),
		},
		Assets: application.AssetOptions{
			Handler: application.AssetFileServerFS(assets),
		},
		Mac: application.MacOptions{
			// Keep running in the menu bar when the popover is dismissed.
			ApplicationShouldTerminateAfterLastWindowClosed: false,
		},
		// Gracefully stop background goroutines on shutdown.
		OnShutdown: func() {
			cancelLifecycle()
			svc.StopPolling()
			updater.Stop()
			startupWG.Wait()
		},
	})

	setupTray(app, svc, bound)
	// Starts only a timer; the first network request is delayed by 30 seconds,
	// while starting before Run makes shutdown joining deterministic.
	updater.Start(app)
	startupWG.Add(1)
	go func() {
		defer startupWG.Done()
		select {
		case <-lifecycleCtx.Done():
			return
		case <-started:
		}

		var jobs sync.WaitGroup
		jobs.Add(2)
		go func() {
			defer jobs.Done()
			if err := svc.Scan(); err != nil {
				log.Printf("helm: initial service scan failed: %v", err)
			}
		}()
		go func() {
			defer jobs.Done()
			runFirstRunWizard(lifecycleCtx, app, svc)
		}()
		jobs.Wait()
	}()

	app.Event.OnApplicationEvent(events.Common.ApplicationStarted, func(*application.ApplicationEvent) {
		startedOnce.Do(func() { close(started) })
	})

	if err := app.Run(); err != nil {
		log.Fatal(err)
	}
}
