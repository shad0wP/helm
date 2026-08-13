package service

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

var serviceIDPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{0,63}$`)

// User-declared services. Format choice: JSON via encoding/json — TOML would
// require a third-party module, and the project's rule is zero Go deps beyond
// Wails, so the config file is ~/.config/helm/services.json.
//
// Schema (all fields but id/name optional; kind defaults to "port"):
//
//	{
//	  "services": [
//	    {
//	      "id": "hermes",
//	      "name": "Hermes Agent",
//	      "kind": "process",        // "systemctl" | "docker" | "port" | "process"
//	      "unit": "",               // systemctl unit (kind=systemctl)
//	      "container": "",          // docker container name (kind=docker)
//	      "port": 9119,             // detection port (port/process kinds)
//	      "icon": "robot",          // Tabler icon name
//	      "color": "purple"
//	    }
//	  ]
//	}
//
// Entries whose id matches a built-in override that built-in; new ids are
// appended. This is how the user tells Helm exactly what "Hermes Agent" is
// and how to stop it.

type userConfig struct {
	Services []userService `json:"services"`
}

type userService struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Kind      string `json:"kind"`
	Unit      string `json:"unit"`
	Container string `json:"container"`
	Port      int    `json:"port"`
	Icon      string `json:"icon"`
	Color     string `json:"color"`
}

// userConfigPath returns ~/.config/helm/services.json, honouring
// XDG_CONFIG_HOME when set.
func userConfigPath() string {
	if xdg := os.Getenv("XDG_CONFIG_HOME"); xdg != "" {
		return filepath.Join(xdg, "helm", "services.json")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".config", "helm", "services.json")
}

// parseUserConfig validates and converts raw config bytes into services.
// Pure function; unit-tested.
func parseUserConfig(raw []byte) ([]Service, error) {
	var cfg userConfig
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&cfg); err != nil {
		return nil, fmt.Errorf("parsing services.json: %w", err)
	}
	if err := dec.Decode(&struct{}{}); err != io.EOF {
		if err == nil {
			return nil, fmt.Errorf("parsing services.json: multiple JSON values")
		}
		return nil, fmt.Errorf("parsing services.json: trailing content: %w", err)
	}
	out := make([]Service, 0, len(cfg.Services))
	seen := make(map[string]struct{}, len(cfg.Services))
	for i, us := range cfg.Services {
		if us.ID == "" || us.Name == "" {
			return nil, fmt.Errorf("services[%d]: id and name are required", i)
		}
		if !serviceIDPattern.MatchString(us.ID) {
			return nil, fmt.Errorf("services[%d]: id %q must contain only lowercase letters, digits, '_' or '-'", i, us.ID)
		}
		if _, exists := seen[us.ID]; exists {
			return nil, fmt.Errorf("services[%d]: duplicate id %q", i, us.ID)
		}
		seen[us.ID] = struct{}{}
		if strings.HasPrefix(us.Unit, "-") || strings.HasPrefix(us.Container, "-") {
			return nil, fmt.Errorf("services[%d] (%s): command target cannot begin with '-'", i, us.ID)
		}
		kind := ServiceKind(us.Kind)
		switch kind {
		case "":
			kind = KindPort
		case KindSystemctl, KindDocker, KindPort, KindProcess:
		default:
			return nil, fmt.Errorf("services[%d] (%s): unknown kind %q", i, us.ID, us.Kind)
		}
		// Port bounds hold for every kind (0 = "none declared"); port/process
		// kinds additionally require one to be present, checked below. Without
		// this, a negative or absurd port on a systemctl/docker entry flows
		// into display strings and TCP dial attempts.
		if us.Port < 0 || us.Port > 65535 {
			return nil, fmt.Errorf("services[%d] (%s): port %d is out of range (0-65535)", i, us.ID, us.Port)
		}
		switch kind {
		case KindSystemctl:
			if us.Unit == "" {
				return nil, fmt.Errorf("services[%d] (%s): kind systemctl requires unit", i, us.ID)
			}
		case KindDocker:
			if us.Container == "" {
				return nil, fmt.Errorf("services[%d] (%s): kind docker requires container", i, us.ID)
			}
		case KindPort, KindProcess:
			if us.Port <= 0 || us.Port > 65535 {
				return nil, fmt.Errorf("services[%d] (%s): kind %s requires a valid port", i, us.ID, kind)
			}
		}
		icon := us.Icon
		if icon == "" {
			icon = "cpu"
		}
		color := us.Color
		if color == "" {
			color = "gray"
		}
		out = append(out, Service{
			ID:        us.ID,
			Name:      us.Name,
			Kind:      kind,
			Unit:      us.Unit,
			Container: us.Container,
			Port:      us.Port,
			Icon:      icon,
			Color:     color,
		})
	}
	return out, nil
}

// loadUserServices reads the user config; a missing file is not an error
// (returns nil), a malformed file is logged by the caller.
func loadUserServices() ([]Service, error) {
	path := userConfigPath()
	if path == "" {
		return nil, nil
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	return parseUserConfig(raw)
}

// mergeServices overlays user-declared services onto the built-in defaults:
// matching ids replace the built-in entry in place (order preserved); new ids
// are appended in config order. Pure function; unit-tested.
func mergeServices(defaults, user []Service) []Service {
	out := make([]Service, len(defaults))
	copy(out, defaults)
	index := map[string]int{}
	for i, s := range out {
		index[s.ID] = i
	}
	for _, us := range user {
		if i, ok := index[us.ID]; ok {
			out[i] = us
		} else {
			index[us.ID] = len(out)
			out = append(out, us)
		}
	}
	return out
}
