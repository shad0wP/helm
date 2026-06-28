package main

import (
	"time"

	"helm/internal/icon"
	"helm/internal/service"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"
)

// iconForState maps an aggregate state to its tray icon bytes.
func iconForState(state string) []byte {
	switch state {
	case "all":
		return icon.IconGreen
	case "some":
		return icon.IconAmber
	default:
		return icon.IconGray
	}
}

// setupTray creates the frameless popover window, wires it to a system tray
// icon, and starts the polling loop that keeps both the tray colour and the
// frontend in sync with the live service state.
func setupTray(app *application.App, svc *service.ServiceManager, bound *App) {
	// 1. The popover panel: frameless, fixed size, always on top, hidden on
	//    start. The native tray attachment dismisses it on focus loss.
	window := app.Window.NewWithOptions(application.WebviewWindowOptions{
		Name:             "helm-panel",
		Title:            "",
		Width:            300,
		Height:           540,
		Frameless:        true,
		AlwaysOnTop:      true,
		Hidden:           true,
		DisableResize:    true,
		BackgroundColour: application.NewRGBA(255, 255, 255, 240),
		URL:              "/",
		Windows:          application.WindowsWindow{},
		Mac: application.MacWindow{
			Backdrop:                application.MacBackdropTranslucent,
			TitleBar:                application.MacTitleBarHiddenInset,
			InvisibleTitleBarHeight: 0,
		},
	})
	bound.window = window

	// Dismiss the panel when it loses focus (clicking outside).
	window.OnWindowEvent(events.Common.WindowLostFocus, func(_ *application.WindowEvent) {
		window.Hide()
	})

	// 2. The system tray icon, starting grey and reflecting current state.
	tray := app.SystemTray.New()
	tray.SetIcon(iconForState(service.AggregateState(svc.GetServices())))
	tray.SetTooltip("Helm — Local AI stack")

	// Left-click toggles the attached window, positioned just below the icon.
	tray.AttachWindow(window).WindowOffset(6)

	// Right-click (and Linux) menu: Show Helm / Quit.
	menu := application.NewMenu()
	menu.Add("Show Helm").OnClick(func(_ *application.Context) {
		tray.ShowWindow()
	})
	menu.AddSeparator()
	menu.Add("Quit").OnClick(func(_ *application.Context) {
		app.Quit()
	})
	tray.SetMenu(menu)

	// 3. On every state change: recolour the tray and push the snapshot to the
	//    frontend so it re-renders without a full reload.
	svc.SetOnChange(func(services []service.Service) {
		tray.SetIcon(iconForState(service.AggregateState(services)))
		app.Event.Emit("services-updated", services)
	})

	// 4. Begin polling every 5 seconds.
	svc.StartPolling(5 * time.Second)
}
