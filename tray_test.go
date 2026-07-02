package main

import (
	"bytes"
	"testing"

	"helm/internal/icon"
	"helm/internal/service"
)

func TestIconForState(t *testing.T) {
	tests := []struct {
		state string
		want  []byte
	}{
		{"all", icon.IconGreen},
		{"some", icon.IconAmber},
		{"none", icon.IconGray},
		{"", icon.IconGray},           // empty -> grey (default branch)
		{"unexpected", icon.IconGray}, // any unknown value -> grey
	}
	for _, tt := range tests {
		t.Run(tt.state, func(t *testing.T) {
			if got := iconForState(tt.state); !bytes.Equal(got, tt.want) {
				t.Errorf("iconForState(%q) returned the wrong icon", tt.state)
			}
		})
	}
}

func TestMenuModel(t *testing.T) {
	t.Run("empty and nil snapshots produce no rows", func(t *testing.T) {
		if got := menuModel(nil); len(got) != 0 {
			t.Errorf("menuModel(nil) = %d entries, want 0", len(got))
		}
		if got := menuModel([]service.Service{}); len(got) != 0 {
			t.Errorf("menuModel(empty) = %d entries, want 0", len(got))
		}
	})

	t.Run("controllable services are enabled, checked tracks Running", func(t *testing.T) {
		got := menuModel([]service.Service{
			{ID: "ollama", Name: "Ollama", Kind: service.KindSystemctl, Running: true},
			{ID: "open-webui", Name: "Open WebUI", Kind: service.KindDocker, Running: false},
		})
		if len(got) != 2 {
			t.Fatalf("got %d entries, want 2", len(got))
		}
		if !got[0].Enabled || !got[0].Checked || got[0].Label != "Ollama" || got[0].ID != "ollama" {
			t.Errorf("systemctl entry wrong: %+v", got[0])
		}
		if !got[1].Enabled || got[1].Checked || got[1].Label != "Open WebUI" {
			t.Errorf("docker entry wrong: %+v", got[1])
		}
	})

	t.Run("port probes are disabled and labelled read-only", func(t *testing.T) {
		got := menuModel([]service.Service{
			{ID: "hermes", Name: "Hermes Agent", Kind: service.KindPort, Running: true},
		})
		if len(got) != 1 {
			t.Fatalf("got %d entries, want 1", len(got))
		}
		if got[0].Enabled {
			t.Error("port-kind entry must be disabled")
		}
		if !got[0].Checked {
			t.Error("running port service must still show checked")
		}
		if got[0].Label != "Hermes Agent (read-only)" {
			t.Errorf("label = %q, want read-only suffix", got[0].Label)
		}
	})

	t.Run("order is preserved", func(t *testing.T) {
		got := menuModel([]service.Service{
			{ID: "a", Name: "A", Kind: service.KindDocker},
			{ID: "b", Name: "B", Kind: service.KindPort},
			{ID: "c", Name: "C", Kind: service.KindSystemctl},
		})
		want := []string{"a", "b", "c"}
		for i, id := range want {
			if got[i].ID != id {
				t.Errorf("entry[%d].ID = %q, want %q", i, got[i].ID, id)
			}
		}
	})
}
