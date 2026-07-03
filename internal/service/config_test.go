package service

import "testing"

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
