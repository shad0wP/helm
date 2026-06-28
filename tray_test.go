package main

import (
	"bytes"
	"testing"
)

func TestIconForState(t *testing.T) {
	tests := []struct {
		state string
		want  []byte
	}{
		{"all", IconGreen},
		{"some", IconAmber},
		{"none", IconGray},
		{"", IconGray},           // empty -> grey (default branch)
		{"unexpected", IconGray}, // any unknown value -> grey
	}
	for _, tt := range tests {
		t.Run(tt.state, func(t *testing.T) {
			if got := iconForState(tt.state); !bytes.Equal(got, tt.want) {
				t.Errorf("iconForState(%q) returned the wrong icon", tt.state)
			}
		})
	}
}
