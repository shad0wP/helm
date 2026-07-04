package service

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"regexp"
)

// Bundled registry of recognizable local-inference tools, embedded into the
// binary at build time — extending this list to cover a new tool is a
// one-line JSON change, not a code change. Kept in-binary (not fetched at
// runtime) to preserve the no-network-except-updates posture.
//
//go:embed signatures.json
var signaturesJSON []byte

// signature describes one recognizable local-inference tool: a regex pattern
// matched (case-insensitively) against a discovered process's command line,
// plus the display metadata used when a process matches it.
type signature struct {
	Pattern string `json:"pattern"`
	Name    string `json:"name"`
	Icon    string `json:"icon"`
	Color   string `json:"color"`
	// Port is an informational default-port hint (shown nowhere at runtime
	// today) — the port used for a discovered service always comes from the
	// live listening socket ss/lsof actually found, never from this hint.
	Port int `json:"port"`
}

// compiledSignature pairs a signature with its compiled, case-insensitive regex.
type compiledSignature struct {
	signature
	re *regexp.Regexp
}

type signatureRegistry struct {
	Signatures []signature `json:"signatures"`
}

// parseSignatures parses and compiles a signature registry from raw JSON.
// Pure function; unit-tested against mock data.
func parseSignatures(raw []byte) ([]compiledSignature, error) {
	var reg signatureRegistry
	if err := json.Unmarshal(raw, &reg); err != nil {
		return nil, fmt.Errorf("parsing signature registry: %w", err)
	}
	out := make([]compiledSignature, 0, len(reg.Signatures))
	for i, s := range reg.Signatures {
		if s.Pattern == "" {
			return nil, fmt.Errorf("signatures[%d]: pattern is required", i)
		}
		if s.Name == "" {
			return nil, fmt.Errorf("signatures[%d]: name is required", i)
		}
		re, err := regexp.Compile(`(?i)` + s.Pattern)
		if err != nil {
			return nil, fmt.Errorf("signatures[%d] (%s): invalid pattern %q: %w", i, s.Name, s.Pattern, err)
		}
		out = append(out, compiledSignature{signature: s, re: re})
	}
	return out, nil
}

// signatures is the parsed, compiled bundled registry, built once at init
// from the embedded JSON.
var signatures = mustParseSignatures(signaturesJSON)

// mustParseSignatures panics on a malformed bundled registry: that JSON is
// compiled into the binary, so a parse failure here is a build-time defect,
// not a runtime condition any caller could meaningfully recover from.
func mustParseSignatures(raw []byte) []compiledSignature {
	sigs, err := parseSignatures(raw)
	if err != nil {
		panic("helm: bundled signature registry is invalid: " + err.Error())
	}
	return sigs
}

// matchSignature reports whether command matches a known local-inference tool
// signature, returning the matched entry's display metadata. Pure function;
// unit-tested. Checks the bundled registry in file order, first match wins.
func matchSignature(command string) (signature, bool) {
	for _, cs := range signatures {
		if cs.re.MatchString(command) {
			return cs.signature, true
		}
	}
	return signature{}, false
}

// matchesInferenceSignature reports whether a process command looks like a
// local LLM/inference server. Pure function; unit-tested.
func matchesInferenceSignature(command string) bool {
	_, ok := matchSignature(command)
	return ok
}
