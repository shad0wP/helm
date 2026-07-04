package main

import (
	"embed"
	"log"

	"helm/internal/service"

	"github.com/wailsapp/wails/v3/pkg/application"
)

// Wails embeds the built frontend (frontend/dist) into the binary.
//
//go:embed all:frontend/dist
var assets embed.FS

// version is the embedded app version, injected at build time via
// -ldflags "-X main.version=<v>" (from version.txt). "dev" for local builds.
var version = "dev"

func main() {
	svc := service.NewServiceManager()
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
			svc.StopPolling()
			updater.Stop()
		},
	})

	setupTray(app, svc, bound)
	// Background update checks emit "update-available" to the frontend and set a
	// tray tooltip suffix; the initial check is delayed so launch isn't slowed.
	updater.Start(app)
	// One-shot first-run discovery wizard (only offers anything the first time
	// ~/.config/helm/services.json doesn't exist yet); runs in its own
	// goroutine so it never blocks startup or the polling loop.
	go runFirstRunWizard(app, svc)

	if err := app.Run(); err != nil {
		log.Fatal(err)
	}
}
