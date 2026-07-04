package service

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestLoadUserServices(t *testing.T) {
	t.Run("absent file returns nil, nil (not an error)", func(t *testing.T) {
		t.Setenv("XDG_CONFIG_HOME", t.TempDir())
		got, err := loadUserServices()
		if got != nil || err != nil {
			t.Errorf("loadUserServices() = %v, %v; want nil, nil", got, err)
		}
	})

	t.Run("valid file is parsed", func(t *testing.T) {
		dir := t.TempDir()
		t.Setenv("XDG_CONFIG_HOME", dir)
		path := filepath.Join(dir, "helm", "services.json")
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(`{"services":[{"id":"x","name":"X","kind":"port","port":1}]}`), 0o644); err != nil {
			t.Fatal(err)
		}
		got, err := loadUserServices()
		if err != nil || len(got) != 1 || got[0].ID != "x" {
			t.Errorf("loadUserServices() = %+v, %v", got, err)
		}
	})

	t.Run("unreadable file is a real error, not treated as absent", func(t *testing.T) {
		if runtime.GOOS == "windows" {
			t.Skip("POSIX permission bits don't apply on Windows")
		}
		dir := t.TempDir()
		t.Setenv("XDG_CONFIG_HOME", dir)
		path := filepath.Join(dir, "helm", "services.json")
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(`{"services":[]}`), 0o000); err != nil {
			t.Fatal(err)
		}
		defer os.Chmod(path, 0o644) // restore so TempDir cleanup can remove it
		if os.Geteuid() == 0 {
			t.Skip("root ignores permission bits, so this case can't be exercised")
		}
		got, err := loadUserServices()
		if err == nil {
			t.Error("expected a real error for a permission-denied config file, got nil")
		}
		if got != nil {
			t.Errorf("expected nil services on error, got %+v", got)
		}
	})
}

func TestParseUserConfig(t *testing.T) {
	t.Run("valid multi-kind config", func(t *testing.T) {
		raw := []byte(`{
			"services": [
				{"id":"hermes","name":"Hermes Agent","kind":"process","port":9119,"icon":"robot","color":"purple"},
				{"id":"my-unit","name":"My Unit","kind":"systemctl","unit":"myllm.service"},
				{"id":"my-ctr","name":"My Container","kind":"docker","container":"llm"},
				{"id":"bare","name":"Bare Port","port":1234}
			]
		}`)
		got, err := parseUserConfig(raw)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(got) != 4 {
			t.Fatalf("got %d services, want 4", len(got))
		}
		if got[0].Kind != KindProcess || got[0].Port != 9119 {
			t.Errorf("hermes = %+v", got[0])
		}
		if got[1].Kind != KindSystemctl || got[1].Unit != "myllm.service" {
			t.Errorf("my-unit = %+v", got[1])
		}
		if got[3].Kind != KindPort { // kind omitted defaults to port
			t.Errorf("bare kind = %q, want default port", got[3].Kind)
		}
		if got[3].Icon != "cpu" || got[3].Color != "gray" { // defaults applied
			t.Errorf("bare defaults not applied: %+v", got[3])
		}
	})

	t.Run("rejects invalid input", func(t *testing.T) {
		cases := map[string]string{
			"malformed json":      `{"services":[`,
			"missing id":          `{"services":[{"name":"X","port":1}]}`,
			"missing name":        `{"services":[{"id":"x","port":1}]}`,
			"unknown kind":        `{"services":[{"id":"x","name":"X","kind":"bogus"}]}`,
			"systemctl no unit":   `{"services":[{"id":"x","name":"X","kind":"systemctl"}]}`,
			"docker no container": `{"services":[{"id":"x","name":"X","kind":"docker"}]}`,
			"process bad port":    `{"services":[{"id":"x","name":"X","kind":"process","port":0}]}`,
			"port out of range":   `{"services":[{"id":"x","name":"X","kind":"port","port":70000}]}`,
		}
		for name, raw := range cases {
			t.Run(name, func(t *testing.T) {
				if _, err := parseUserConfig([]byte(raw)); err == nil {
					t.Errorf("expected error for %s, got nil", name)
				}
			})
		}
	})

	t.Run("empty services list is valid", func(t *testing.T) {
		got, err := parseUserConfig([]byte(`{"services":[]}`))
		if err != nil || len(got) != 0 {
			t.Errorf("got %v, %v; want empty, nil", got, err)
		}
	})
}

func TestMergeServices(t *testing.T) {
	defaults := []Service{
		{ID: "ollama", Name: "Ollama", Kind: KindSystemctl, Port: 11434},
		{ID: "hermes", Name: "Hermes Agent", Kind: KindProcess, Port: 9119},
	}

	t.Run("override in place, append new, preserve order", func(t *testing.T) {
		user := []Service{
			{ID: "hermes", Name: "My Hermes", Kind: KindProcess, Port: 5000}, // override
			{ID: "vllm", Name: "vLLM", Kind: KindProcess, Port: 8000},        // new
		}
		got := mergeServices(defaults, user)
		if len(got) != 3 {
			t.Fatalf("got %d, want 3", len(got))
		}
		if got[0].ID != "ollama" {
			t.Errorf("order changed: %+v", got)
		}
		if got[1].ID != "hermes" || got[1].Name != "My Hermes" || got[1].Port != 5000 {
			t.Errorf("hermes not overridden in place: %+v", got[1])
		}
		if got[2].ID != "vllm" {
			t.Errorf("new service not appended: %+v", got[2])
		}
	})

	t.Run("does not mutate defaults", func(t *testing.T) {
		user := []Service{{ID: "ollama", Name: "Changed", Kind: KindPort, Port: 1}}
		_ = mergeServices(defaults, user)
		if defaults[0].Name != "Ollama" {
			t.Errorf("defaults were mutated: %+v", defaults[0])
		}
	})

	t.Run("nil user config returns defaults copy", func(t *testing.T) {
		got := mergeServices(defaults, nil)
		if len(got) != 2 {
			t.Errorf("got %d, want 2", len(got))
		}
	})
}
