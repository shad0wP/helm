package service

import "testing"

func TestMustParseSignaturesPanicsOnInvalidRegistry(t *testing.T) {
	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("mustParseSignatures did not panic on an invalid registry")
		}
		msg, ok := r.(string)
		if !ok || msg == "" {
			t.Errorf("panic value = %v, want a non-empty string message", r)
		}
	}()
	mustParseSignatures([]byte(`{"signatures":[{"pattern":"(unclosed","name":"X"}]}`))
}

func TestParseSignatures(t *testing.T) {
	t.Run("valid mock registry parses and compiles", func(t *testing.T) {
		raw := []byte(`{
			"signatures": [
				{"pattern": "\\bmocktool\\b", "name": "MockTool", "icon": "cpu", "color": "blue", "port": 1234},
				{"pattern": "\\bother\\b", "name": "Other", "icon": "robot", "color": "gray"}
			]
		}`)
		got, err := parseSignatures(raw)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(got) != 2 {
			t.Fatalf("got %d signatures, want 2", len(got))
		}
		if got[0].Name != "MockTool" || got[0].Icon != "cpu" || got[0].Color != "blue" || got[0].Port != 1234 {
			t.Errorf("signature[0] = %+v", got[0].signature)
		}
		if got[1].Port != 0 {
			t.Errorf("signature[1] missing port should default to 0, got %d", got[1].Port)
		}
		if !got[0].re.MatchString("running mocktool --serve") {
			t.Error("compiled pattern did not match an expected command line")
		}
		if got[0].re.MatchString("mocktoolish") {
			t.Error("word-boundary pattern matched a substring it shouldn't have")
		}
	})

	t.Run("case-insensitive matching", func(t *testing.T) {
		got, err := parseSignatures([]byte(`{"signatures":[{"pattern":"\\bMOCKTOOL\\b","name":"X"}]}`))
		if err != nil {
			t.Fatal(err)
		}
		if !got[0].re.MatchString("running mocktool now") {
			t.Error("expected case-insensitive match")
		}
	})

	t.Run("malformed JSON is an error", func(t *testing.T) {
		if _, err := parseSignatures([]byte(`{"signatures":[`)); err == nil {
			t.Error("expected an error for malformed JSON")
		}
	})

	t.Run("missing pattern is an error", func(t *testing.T) {
		if _, err := parseSignatures([]byte(`{"signatures":[{"name":"X"}]}`)); err == nil {
			t.Error("expected an error for a missing pattern")
		}
	})

	t.Run("missing name is an error", func(t *testing.T) {
		if _, err := parseSignatures([]byte(`{"signatures":[{"pattern":"x"}]}`)); err == nil {
			t.Error("expected an error for a missing name")
		}
	})

	t.Run("invalid regex pattern is an error", func(t *testing.T) {
		if _, err := parseSignatures([]byte(`{"signatures":[{"pattern":"(unclosed","name":"X"}]}`)); err == nil {
			t.Error("expected an error for an invalid regex pattern")
		}
	})

	t.Run("empty registry is valid", func(t *testing.T) {
		got, err := parseSignatures([]byte(`{"signatures":[]}`))
		if err != nil || len(got) != 0 {
			t.Errorf("got %v, %v; want empty, nil", got, err)
		}
	})
}

func TestMatchSignature(t *testing.T) {
	mock, err := parseSignatures([]byte(`{
		"signatures": [
			{"pattern": "\\bfirsttool\\b", "name": "First", "icon": "cpu", "color": "green"},
			{"pattern": "\\bsecondtool\\b", "name": "Second", "icon": "robot", "color": "blue"}
		]
	}`))
	if err != nil {
		t.Fatal(err)
	}

	t.Run("returns the matched signature's metadata", func(t *testing.T) {
		var got signature
		var ok bool
		for _, cs := range mock {
			if cs.re.MatchString("running secondtool --port 9000") {
				got, ok = cs.signature, true
				break
			}
		}
		if !ok || got.Name != "Second" || got.Icon != "robot" || got.Color != "blue" {
			t.Errorf("got %+v, ok=%v", got, ok)
		}
	})

	t.Run("no match returns ok=false", func(t *testing.T) {
		for _, cs := range mock {
			if cs.re.MatchString("nginx -g daemon off") {
				t.Fatalf("unexpected match against %q", cs.Name)
			}
		}
	})
}

// TestBundledSignatureRegistry is a regression guard against the REAL,
// embedded signatures.json (not mock data): it must parse successfully at
// package init (already implied by every other test running at all, since
// `signatures` is a package-level var), contain at least the tools the
// project explicitly commits to recognizing, and every entry must carry
// complete display metadata.
func TestBundledSignatureRegistry(t *testing.T) {
	if len(signatures) == 0 {
		t.Fatal("bundled signature registry is empty")
	}
	for _, cs := range signatures {
		if cs.Name == "" || cs.Icon == "" || cs.Color == "" {
			t.Errorf("signature %+v is missing required display metadata", cs.signature)
		}
	}

	wantRecognized := map[string]string{
		"ollama":                "Ollama",
		"vllm":                  "vLLM",
		"llama-server":          "llama.cpp",
		"koboldcpp":             "KoboldCpp",
		"lm-studio":             "LM Studio",
		"text-generation-webui": "text-generation-webui",
	}
	for command, wantName := range wantRecognized {
		sig, ok := matchSignature(command)
		if !ok {
			t.Errorf("bundled registry does not recognize %q", command)
			continue
		}
		if sig.Name != wantName {
			t.Errorf("matchSignature(%q).Name = %q, want %q", command, sig.Name, wantName)
		}
	}

	// Full command lines must also match (the wizard/docs describe matching
	// against whatever ss/lsof reports, which may include arguments).
	for _, command := range []string{
		"llama-server", "sglang", "tabbyAPI",
		"python3 -m vllm.entrypoints.openai.api_server",
		"python -m http.server serve", "text-generation-launcher",
	} {
		if !matchesInferenceSignature(command) {
			t.Errorf("matchesInferenceSignature(%q) = false, want true", command)
		}
	}

	// Common daemons that also hold listening sockets must NOT be recognized —
	// a false positive here surfaces a bogus "AI service" in the tray. A bare
	// "python3" (what ss actually reports for a python-based server) must not
	// match either: the python catch-all requires visible arguments.
	for _, command := range []string{"", "sshd", "postgres", "nginx", "chrome", "code", "python3"} {
		if matchesInferenceSignature(command) {
			t.Errorf("matchesInferenceSignature(%q) = true, want false", command)
		}
	}
}
