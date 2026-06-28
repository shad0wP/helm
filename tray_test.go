package main

import (
	"bytes"
	"testing"

	"helm/internal/icon"
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
