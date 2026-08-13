package service

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDecideWizardAction(t *testing.T) {
	tests := []struct {
		name         string
		configExists bool
		candidates   []Service
		want         WizardAction
	}{
		{"config exists, no candidates -> skip", true, nil, WizardSkip},
		{"config exists, candidates present -> still skip", true, []Service{{ID: "x"}}, WizardSkip},
		{"no config, no candidates -> no results", false, nil, WizardNoResults},
		{"no config, empty slice -> no results", false, []Service{}, WizardNoResults},
		{"no config, candidates found -> propose", false, []Service{{ID: "proc_8000", Name: "vLLM"}}, WizardPropose},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := DecideWizardAction(tt.configExists, tt.candidates); got != tt.want {
				t.Errorf("DecideWizardAction(%v, %v) = %v, want %v", tt.configExists, tt.candidates, got, tt.want)
			}
		})
	}
}

func TestNeedsFirstRunScan(t *testing.T) {
	t.Run("true when the config file is absent", func(t *testing.T) {
		t.Setenv("XDG_CONFIG_HOME", t.TempDir())
		if !NeedsFirstRunScan() {
			t.Error("NeedsFirstRunScan() = false, want true for a fresh config dir")
		}
	})

	t.Run("false once the config file exists", func(t *testing.T) {
		dir := t.TempDir()
		t.Setenv("XDG_CONFIG_HOME", dir)
		path := filepath.Join(dir, "helm", "services.json")
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(`{"services":[]}`), 0o644); err != nil {
			t.Fatal(err)
		}
		if NeedsFirstRunScan() {
			t.Error("NeedsFirstRunScan() = true, want false once services.json exists")
		}
	})

	t.Run("false after the wizard was skipped", func(t *testing.T) {
		t.Setenv("XDG_CONFIG_HOME", t.TempDir())
		if err := MarkFirstRunComplete(); err != nil {
			t.Fatal(err)
		}
		if NeedsFirstRunScan() {
			t.Error("NeedsFirstRunScan() = true after completion marker")
		}
	})
}

func TestSaveFirstRunConfig(t *testing.T) {
	t.Run("no candidates is a no-op, no file created", func(t *testing.T) {
		dir := t.TempDir()
		t.Setenv("XDG_CONFIG_HOME", dir)
		if err := SaveFirstRunConfig(nil); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if _, err := os.Stat(filepath.Join(dir, "helm", "services.json")); !os.IsNotExist(err) {
			t.Error("SaveFirstRunConfig(nil) should not create a file")
		}
	})

	t.Run("writes a valid, round-trippable services.json", func(t *testing.T) {
		dir := t.TempDir()
		t.Setenv("XDG_CONFIG_HOME", dir)

		candidates := []Service{
			{ID: "proc_8000", Name: "vLLM (:8000)", Kind: KindProcess, Port: 8000, Icon: "cpu", Color: "purple"},
		}
		if err := SaveFirstRunConfig(candidates); err != nil {
			t.Fatalf("SaveFirstRunConfig: %v", err)
		}

		path := filepath.Join(dir, "helm", "services.json")
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("reading written config: %v", err)
		}
		got, err := parseUserConfig(raw)
		if err != nil {
			t.Fatalf("written config does not parse: %v", err)
		}
		if len(got) != 1 || got[0].ID != "proc_8000" || got[0].Kind != KindProcess || got[0].Port != 8000 {
			t.Errorf("round-tripped config = %+v", got)
		}

		// NeedsFirstRunScan must now report false — the whole point of the wizard.
		if NeedsFirstRunScan() {
			t.Error("NeedsFirstRunScan() = true after SaveFirstRunConfig wrote the file")
		}
	})

	t.Run("errors instead of overwriting a malformed existing config", func(t *testing.T) {
		dir := t.TempDir()
		t.Setenv("XDG_CONFIG_HOME", dir)
		path := filepath.Join(dir, "helm", "services.json")
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(`{not valid json`), 0o644); err != nil {
			t.Fatal(err)
		}

		candidates := []Service{{ID: "proc_8000", Name: "vLLM (:8000)", Kind: KindProcess, Port: 8000}}
		if err := SaveFirstRunConfig(candidates); err == nil {
			t.Error("expected an error when the existing config is malformed, got nil")
		}
	})

	t.Run("errors when services.json exists as a directory", func(t *testing.T) {
		dir := t.TempDir()
		t.Setenv("XDG_CONFIG_HOME", dir)
		path := filepath.Join(dir, "helm", "services.json")
		if err := os.MkdirAll(path, 0o755); err != nil { // the config PATH is a directory
			t.Fatal(err)
		}
		candidates := []Service{{ID: "proc_8000", Name: "vLLM (:8000)", Kind: KindProcess, Port: 8000}}
		if err := SaveFirstRunConfig(candidates); err == nil {
			t.Error("expected an error when services.json is a directory, got nil")
		}
	})

	t.Run("errors when the config directory path is blocked by a file", func(t *testing.T) {
		dir := t.TempDir()
		t.Setenv("XDG_CONFIG_HOME", dir)
		if err := os.WriteFile(filepath.Join(dir, "helm"), []byte("not a dir"), 0o644); err != nil {
			t.Fatal(err)
		}
		candidates := []Service{{ID: "proc_8000", Name: "vLLM (:8000)", Kind: KindProcess, Port: 8000}}
		if err := SaveFirstRunConfig(candidates); err == nil {
			t.Error("expected an error when ~/.config/helm is a regular file, got nil")
		}
	})

	t.Run("merges with an existing user config instead of clobbering it", func(t *testing.T) {
		dir := t.TempDir()
		t.Setenv("XDG_CONFIG_HOME", dir)
		path := filepath.Join(dir, "helm", "services.json")
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		existing := `{"services":[{"id":"my-unit","name":"Custom LLM","kind":"systemctl","unit":"myllm.service"}]}`
		if err := os.WriteFile(path, []byte(existing), 0o644); err != nil {
			t.Fatal(err)
		}

		candidates := []Service{
			{ID: "proc_8000", Name: "vLLM (:8000)", Kind: KindProcess, Port: 8000, Icon: "cpu", Color: "purple"},
		}
		if err := SaveFirstRunConfig(candidates); err != nil {
			t.Fatalf("SaveFirstRunConfig: %v", err)
		}

		got, err := loadUserServices()
		if err != nil {
			t.Fatalf("loadUserServices: %v", err)
		}
		if len(got) != 2 {
			t.Fatalf("got %d services after merge, want 2 (1 existing + 1 new): %+v", len(got), got)
		}
		ids := map[string]bool{}
		for _, s := range got {
			ids[s.ID] = true
		}
		if !ids["my-unit"] || !ids["proc_8000"] {
			t.Errorf("expected both my-unit and proc_8000 present, got %+v", got)
		}
	})
}

// TestWizardWithoutAnyHome: with neither XDG_CONFIG_HOME nor HOME resolvable
// there is no config path at all. The wizard must degrade safely — never offer
// itself (NeedsFirstRunScan false) and refuse to save rather than writing to a
// junk location.
func TestWizardWithoutAnyHome(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", "")
	t.Setenv("HOME", "")
	if NeedsFirstRunScan() {
		t.Error("NeedsFirstRunScan() = true with no resolvable home, want false (fail safe)")
	}
	candidates := []Service{{ID: "proc_8000", Name: "vLLM (:8000)", Kind: KindProcess, Port: 8000}}
	if err := SaveFirstRunConfig(candidates); err == nil {
		t.Error("SaveFirstRunConfig with no resolvable home = nil, want error")
	}
}

// TestDiscoverFirstRunCandidates is a light smoke test (like the rest of the
// discovery layer, this shells out to ss/lsof and can't be fully mocked
// without fighting the OS) — it must run without panicking or erroring and
// return a slice, never nil-panic on an empty host.
func TestDiscoverFirstRunCandidates(t *testing.T) {
	got := DiscoverFirstRunCandidates()
	if got == nil {
		// nil is a perfectly valid "found nothing" result; just confirming the
		// call completes without panicking is the point of this test.
		return
	}
	for _, s := range got {
		if s.Kind != KindProcess {
			t.Errorf("candidate %+v has Kind %q, want %q", s, s.Kind, KindProcess)
		}
	}
}
