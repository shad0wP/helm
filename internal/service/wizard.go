package service

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// First-run detection wizard: on a launch where ~/.config/helm/services.json
// doesn't exist yet, Helm runs the discovery scan once and offers to save
// whatever it found. This file holds every piece of that logic that doesn't
// touch the GUI — the OS-facing (file existence, discovery, atomic write) and
// pure decision-making pieces — so it's testable without a Wails runtime. The
// thin dialog-presentation shell lives in the main package's wizard.go, which
// calls these functions in sequence.

// NeedsFirstRunScan reports whether the user config file is absent — the
// trigger condition for offering the first-run wizard. Safe to call more than
// once (it just re-checks the filesystem); callers decide how often to ask.
func NeedsFirstRunScan() bool {
	path := userConfigPath()
	if path == "" {
		return false
	}
	if _, err := os.Stat(firstRunMarkerPath()); err == nil {
		return false
	}
	_, err := os.Stat(path)
	return os.IsNotExist(err)
}

func firstRunMarkerPath() string {
	path := userConfigPath()
	if path == "" {
		return ""
	}
	return filepath.Join(filepath.Dir(path), ".first-run-complete")
}

// MarkFirstRunComplete records that the one-time discovery was completed or
// explicitly skipped without inventing another service configuration format.
func MarkFirstRunComplete() error {
	path := firstRunMarkerPath()
	if path == "" {
		return fmt.Errorf("could not determine the user config path (no $HOME)")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("creating first-run state directory: %w", err)
	}
	if err := os.WriteFile(path, []byte("completed\n"), 0o600); err != nil {
		return fmt.Errorf("recording first-run completion: %w", err)
	}
	return nil
}

// DiscoverFirstRunCandidates runs the same ss/lsof discovery scan Scan() uses
// and returns any recognized local-inference process not already covered by a
// running built-in service. This shells out (bounded by the existing
// probeTimeout pattern throughout listListeners/checkRunning) and must only
// ever be called from a background goroutine, never the UI/main thread.
func DiscoverFirstRunCandidates() []Service {
	claimed := map[int]bool{}
	for _, s := range defaultServices() {
		if s.Port > 0 && checkRunning(s) {
			claimed[s.Port] = true
		}
	}
	return discoverProcesses(claimed)
}

// WizardAction is what the first-run wizard should do next, given the current
// state — computed before any Wails/OS side effects happen.
type WizardAction int

const (
	// WizardSkip: a user config already exists; never offer the wizard.
	WizardSkip WizardAction = iota
	// WizardNoResults: no config yet, but nothing recognizable was found.
	WizardNoResults
	// WizardPropose: no config yet, and candidates were found — show the dialog.
	WizardPropose
)

// DecideWizardAction is the wizard's whole state machine, reduced to a pure
// function of (does a config exist, what did discovery find). Pure function;
// unit-tested for all three transitions without touching Wails or the OS.
func DecideWizardAction(configExists bool, candidates []Service) WizardAction {
	if configExists {
		return WizardSkip
	}
	if len(candidates) == 0 {
		return WizardNoResults
	}
	return WizardPropose
}

// SaveFirstRunConfig writes the accepted candidates to
// ~/.config/helm/services.json, in exactly the schema parseUserConfig reads —
// no new config format. Existing on-disk entries (if a config file was
// created between the initial check and the user's answer) are preserved via
// the same mergeServices used everywhere else; a candidate only overwrites an
// existing entry if their ids happen to collide. The write is atomic (temp
// file in the same directory, then rename) so a crash or concurrent read
// never observes a half-written file.
func SaveFirstRunConfig(candidates []Service) error {
	if len(candidates) == 0 {
		return nil
	}
	path := userConfigPath()
	if path == "" {
		return fmt.Errorf("could not determine the user config path (no $HOME)")
	}

	existing, err := loadUserServices() // nil, nil if the file is still absent
	if err != nil {
		return fmt.Errorf("reading existing config before merge: %w", err)
	}
	merged := mergeServices(existing, candidates)

	cfg := userConfig{Services: make([]userService, 0, len(merged))}
	for _, s := range merged {
		cfg.Services = append(cfg.Services, userService{
			ID:        s.ID,
			Name:      s.Name,
			Kind:      string(s.Kind),
			Unit:      s.Unit,
			Container: s.Container,
			Port:      s.Port,
			Icon:      s.Icon,
			Color:     s.Color,
		})
	}

	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("encoding services.json: %w", err)
	}

	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("creating %s: %w", dir, err)
	}
	tmp, err := os.CreateTemp(dir, "services.json.tmp-*")
	if err != nil {
		return fmt.Errorf("creating temp file: %w", err)
	}
	tmpPath := tmp.Name()
	defer func() {
		_ = tmp.Close()
		_ = os.Remove(tmpPath)
	}()
	if _, err := tmp.Write(data); err != nil {
		return fmt.Errorf("writing temp file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("closing temp file: %w", err)
	}
	if err := os.Rename(tmpPath, path); err != nil {
		return fmt.Errorf("finalizing %s: %w", path, err)
	}
	return nil
}
