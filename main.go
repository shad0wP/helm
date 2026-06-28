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

func main() {
	svc := service.NewServiceManager()
	bound := &App{svc: svc}

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
		// Gracefully stop the polling goroutine on shutdown.
		OnShutdown: func() { svc.StopPolling() },
	})

	setupTray(app, svc, bound)

	if err := app.Run(); err != nil {
		log.Fatal(err)
	}
}
