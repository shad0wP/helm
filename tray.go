package main

import (
	"log"
	"os"
	"runtime"
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

// menuEntry is a pure description of one per-service tray-menu row.
type menuEntry struct {
	ID      string // service ID passed to Toggle
	Label   string // display label
	Checked bool   // checked = running
	Enabled bool   // enabled = controllable (not a read-only port probe)
}

// menuModel derives the per-service menu rows from a service snapshot.
// Pure logic, unit-tested; the Linux tray menu is rebuilt from this on every
// state change.
func menuModel(services []service.Service) []menuEntry {
	entries := make([]menuEntry, 0, len(services))
	for _, s := range services {
		enabled := s.Kind != service.KindPort
		label := s.Name
		if !enabled {
			label += " (read-only)"
		}
		entries = append(entries, menuEntry{
			ID:      s.ID,
			Label:   label,
			Checked: s.Running,
			Enabled: enabled,
		})
	}
	return entries
}

// setupTray creates the system tray and the platform-appropriate UI around it.
//
// macOS: a frameless, translucent popover anchored under the menu-bar icon
// (AttachWindow), hidden on focus loss — the classic menu-bar-app pattern.
//
// Linux (and anything else): a native StatusNotifierItem menu drives control
// directly, and the webview opens as a normal, decorated, movable window from
// "Show Helm…". Rationale: Wayland forbids client-side absolute window
// positioning and SNI exposes no tray-icon geometry, so an anchored popover is
// not implementable; and on KDE/Wayland a frameless always-on-top window never
// reliably holds focus, so a hide-on-blur popover dies instantly (known Wails
// v3 issue on this stack). The native menu is rendered by the DE at the cursor
// and has neither problem.
func setupTray(app *application.App, svc *service.ServiceManager, bound *App) {
	tray := app.SystemTray.New()
	tray.SetIcon(iconForState(service.AggregateState(svc.GetServices())))
	tray.SetTooltip("Helm — Local AI stack")

	if runtime.GOOS == "darwin" {
		log.Printf("helm: ui mode=popover (GOOS=%s)", runtime.GOOS)
		setupDarwinUI(app, svc, bound, tray)
		return
	}
	log.Printf("helm: ui mode=menu+window (GOOS=%s)", runtime.GOOS)
	setupLinuxUI(app, svc, bound, tray)
}

// setupDarwinUI wires the macOS popover: frameless translucent panel attached
// to the tray icon, positioned below it, dismissed on focus loss.
func setupDarwinUI(app *application.App, svc *service.ServiceManager, bound *App, tray *application.SystemTray) {
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
		Mac: application.MacWindow{
			Backdrop:                application.MacBackdropTranslucent,
			TitleBar:                application.MacTitleBarHiddenInset,
			InvisibleTitleBarHeight: 0,
		},
	})
	bound.window = window

	// Dismiss the panel when it loses focus (clicking outside). This is safe on
	// macOS; on Linux/Wayland the same handler fires immediately (window never
	// holds focus), which is why it is darwin-only.
	window.OnWindowEvent(events.Common.WindowLostFocus, func(_ *application.WindowEvent) {
		window.Hide()
	})

	// Left-click toggles the attached window, positioned just below the icon.
	tray.AttachWindow(window).WindowOffset(6)

	// Right-click menu: Show Helm / Quit.
	menu := application.NewMenu()
	menu.Add("Show Helm").OnClick(func(_ *application.Context) {
		tray.ShowWindow()
	})
	menu.AddSeparator()
	menu.Add("Quit").OnClick(func(_ *application.Context) {
		app.Quit()
	})
	tray.SetMenu(menu)

	// On every state change: recolour the tray and push the snapshot to the
	// frontend so it re-renders without a full reload.
	svc.SetOnChange(func(services []service.Service) {
		tray.SetIcon(iconForState(service.AggregateState(services)))
		app.Event.Emit("services-updated", services)
	})

	svc.StartPolling(5 * time.Second)
}

// setupLinuxUI wires the Linux experience: control lives in the native SNI
// tray menu (rebuilt on every state change); the webview is an optional,
// normal, decorated, movable window opened from "Show Helm…". Closing the
// window hides it — the app keeps running in the tray.
func setupLinuxUI(app *application.App, svc *service.ServiceManager, bound *App, tray *application.SystemTray) {
	window := app.Window.NewWithOptions(application.WebviewWindowOptions{
		Name:   "helm-panel",
		Title:  "Helm",
		Width:  340,
		Height: 560,
		// A real WM window: decorated (movable), not always-on-top, and no
		// hide-on-blur — it persists until the user closes it.
		Frameless:        false,
		AlwaysOnTop:      false,
		Hidden:           true,
		DisableResize:    false,
		BackgroundColour: application.NewRGBA(255, 255, 255, 255),
		URL:              "/",
	})
	bound.window = window

	// Close button hides the window instead of destroying it, so "Show Helm…"
	// keeps working and the app stays resident in the tray.
	window.RegisterHook(events.Common.WindowClosing, func(e *application.WindowEvent) {
		e.Cancel()
		window.Hide()
	})

	rebuildMenu := func(services []service.Service) {
		tray.SetMenu(buildLinuxMenu(app, svc, window, services))
	}
	rebuildMenu(svc.GetServices())

	// KDE opens SNI menus on right-click natively; make left-click do the same
	// so a single click always presents the controls.
	tray.OnClick(func() {
		tray.OpenMenu()
	})

	// On every state change: recolour the tray, rebuild the native menu so
	// checkmarks track reality, and push the snapshot to the frontend.
	svc.SetOnChange(func(services []service.Service) {
		tray.SetIcon(iconForState(service.AggregateState(services)))
		rebuildMenu(services)
		app.Event.Emit("services-updated", services)
	})

	svc.StartPolling(5 * time.Second)

	// Defensive fallback for desktops without an SNI tray (e.g. stock GNOME
	// without the AppIndicator extension): Wails offers no way to detect a
	// missing tray host, so HELM_FORCE_WINDOW=1 opens the window at startup to
	// guarantee the app is never running headless-and-invisible.
	if os.Getenv("HELM_FORCE_WINDOW") != "" {
		log.Printf("helm: HELM_FORCE_WINDOW set — showing window at startup")
		window.Show().Focus()
	}
}

// buildLinuxMenu constructs the native tray menu for the given snapshot:
// one checkable row per service (checked = running; read-only probes are
// disabled), bulk actions, and window/app controls.
func buildLinuxMenu(app *application.App, svc *service.ServiceManager, window application.Window, services []service.Service) *application.Menu {
	menu := application.NewMenu()

	for _, entry := range menuModel(services) {
		item := menu.AddCheckbox(entry.Label, entry.Checked)
		if !entry.Enabled {
			item.SetEnabled(false)
			continue
		}
		id := entry.ID // capture per-iteration
		item.OnClick(func(_ *application.Context) {
			// Toggle shells out (up to controlTimeout); never block the UI
			// thread. The poll/refresh path rebuilds the menu afterwards.
			go func() {
				if err := svc.Toggle(id); err != nil {
					log.Printf("helm: toggle %s failed: %v", id, err)
				}
			}()
		})
	}

	menu.AddSeparator()
	menu.Add("Start all").OnClick(func(_ *application.Context) {
		go func() {
			if err := svc.StartAll(); err != nil {
				log.Printf("helm: start all failed: %v", err)
			}
		}()
	})
	menu.Add("Stop all").OnClick(func(_ *application.Context) {
		go func() {
			if err := svc.StopAll(); err != nil {
				log.Printf("helm: stop all failed: %v", err)
			}
		}()
	})
	menu.Add("Scan for services").OnClick(func(_ *application.Context) {
		go func() {
			if err := svc.Scan(); err != nil {
				log.Printf("helm: scan failed: %v", err)
			}
		}()
	})

	menu.AddSeparator()
	menu.Add("Show Helm…").OnClick(func(_ *application.Context) {
		window.Show().Focus()
	})
	menu.Add("Quit").OnClick(func(_ *application.Context) {
		app.Quit()
	})

	return menu
}
