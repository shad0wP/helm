package main

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	"helm/internal/service"

	"github.com/wailsapp/wails/v3/pkg/application"
)

// firstRunWizardDelay gives the tray/window a moment to finish appearing
// before a system dialog might pop up, so first launch doesn't feel like it's
// being interrupted mid-render.
const firstRunWizardDelay = 2 * time.Second

// runFirstRunWizard is the one-shot first-run detection wizard: if
// ~/.config/helm/services.json doesn't exist yet, it runs the discovery scan
// exactly once and — only if something was found — offers to add it via a
// native dialog. Accepting, declining, or finding no candidates records
// completion so this automatic scan runs only once. It never blocks app
// startup or the polling loop: call this via `go runFirstRunWizard(...)`
// from main(), the same way the updater's background check is started.
//
// All OS/decision logic (file check, ss/lsof discovery, the propose/skip
// state machine, the atomic services.json write) lives in internal/service
// and is unit-tested there without a Wails runtime; this function is only the
// dialog-presentation glue, which can't be meaningfully unit-tested without a
// live Wails app — consistent with how setupTray/buildLinuxMenu are handled.
func runFirstRunWizard(ctx context.Context, app *application.App, svc *service.ServiceManager) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("helm: recovered from panic during first-run wizard: %v", r)
		}
	}()

	timer := time.NewTimer(firstRunWizardDelay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return
	case <-timer.C:
	}

	configExists := !service.NeedsFirstRunScan()
	var candidates []service.Service
	if !configExists {
		candidates = service.DiscoverFirstRunCandidates()
	}

	switch service.DecideWizardAction(configExists, candidates) {
	case service.WizardSkip, service.WizardNoResults:
		if !configExists {
			if err := service.MarkFirstRunComplete(); err != nil {
				log.Printf("helm: could not record completed first-run scan: %v", err)
			}
		}
		return
	case service.WizardPropose:
		presentWizardDialog(ctx, app, svc, candidates)
	}
}

// presentWizardDialog shows the native "Found: ... Add these?" dialog and
// wires its two buttons. Show() only marshals dialog creation onto the main
// thread and returns immediately — the user's answer arrives later via the
// button's OnClick callback, so this never blocks the caller either.
func presentWizardDialog(ctx context.Context, app *application.App, svc *service.ServiceManager, candidates []service.Service) {
	names := make([]string, 0, len(candidates))
	for _, c := range candidates {
		names = append(names, fmt.Sprintf("%s on :%d", c.Name, c.Port))
	}
	message := fmt.Sprintf("Found: %s. Add these?", strings.Join(names, ", "))

	dlg := app.Dialog.Question().
		SetTitle("Helm found local AI services").
		SetMessage(message)

	add := dlg.AddButton("Add these")
	add.OnClick(func() {
		if ctx.Err() != nil {
			return
		}
		if err := service.SaveFirstRunConfig(candidates); err != nil {
			log.Printf("helm: first-run wizard failed to save services.json: %v", err)
			return
		}
		if err := svc.Scan(); err != nil {
			log.Printf("helm: first-run wizard rescan failed: %v", err)
		}
		log.Printf("helm: first-run wizard added %d service(s)", len(candidates))
	})
	add.SetAsDefault()

	notNow := dlg.AddButton("Not now")
	notNow.OnClick(func() {
		if ctx.Err() != nil {
			return
		}
		if err := service.MarkFirstRunComplete(); err != nil {
			log.Printf("helm: could not record skipped first-run wizard: %v", err)
		}
	})
	notNow.SetAsCancel()

	dlg.Show()
}
