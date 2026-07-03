package service

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

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
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return nil, fmt.Errorf("parsing services.json: %w", err)
	}
	out := make([]Service, 0, len(cfg.Services))
	for i, us := range cfg.Services {
		if us.ID == "" || us.Name == "" {
			return nil, fmt.Errorf("services[%d]: id and name are required", i)
		}
		kind := ServiceKind(us.Kind)
		switch kind {
		case "":
			kind = KindPort
		case KindSystemctl, KindDocker, KindPort, KindProcess:
		default:
			return nil, fmt.Errorf("services[%d] (%s): unknown kind %q", i, us.ID, us.Kind)
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
