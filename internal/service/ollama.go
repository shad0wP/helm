package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
)

// Free-VRAM support: Ollama keeps models resident in GPU memory for
// `keep_alive` after last use. Evicting them (keep_alive: 0) frees VRAM
// instantly without tearing down the daemon — better for the
// "reclaim the GPU before gaming" case than stopping the service, which
// forces a cold reload later.

// defaultOllamaBase is where a stock Ollama listens.
const defaultOllamaBase = "http://127.0.0.1:11434"
const maxOllamaResponseBytes = 1 << 20

type ollamaPSResponse struct {
	Models []struct {
		Name string `json:"name"`
	} `json:"models"`
}

type ollamaUnloadRequest struct {
	Model     string `json:"model"`
	KeepAlive int    `json:"keep_alive"`
}

// ollamaLoadedModels asks the Ollama API which models are resident.
func ollamaLoadedModels(base string) ([]string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), probeTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, base+"/api/ps", nil)
	if err != nil {
		return nil, err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("querying Ollama at %s: %w", base, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("ollama /api/ps returned %s", resp.Status)
	}
	var ps ollamaPSResponse
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxOllamaResponseBytes+1))
	if err != nil {
		return nil, fmt.Errorf("reading /api/ps: %w", err)
	}
	if len(data) > maxOllamaResponseBytes {
		return nil, errors.New("ollama /api/ps response exceeds 1 MiB")
	}
	if err := json.Unmarshal(data, &ps); err != nil {
		return nil, fmt.Errorf("decoding /api/ps: %w", err)
	}
	names := make([]string, 0, len(ps.Models))
	for _, m := range ps.Models {
		names = append(names, m.Name)
	}
	return names, nil
}

// ollamaUnloadModel evicts one model by requesting a generation with
// keep_alive 0 (the documented eviction idiom).
func ollamaUnloadModel(base, model string) error {
	body, err := json.Marshal(ollamaUnloadRequest{Model: model})
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), controlTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, base+"/api/generate", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("unloading %s: %w", model, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unloading %s: ollama returned %s", model, resp.Status)
	}
	return nil
}

// freeVRAM evicts every loaded Ollama model and returns how many were
// unloaded. base is parameterised for tests; callers use defaultOllamaBase.
func freeVRAM(base string) (int, error) {
	models, err := ollamaLoadedModels(base)
	if err != nil {
		return 0, err
	}
	for _, m := range models {
		if err := ollamaUnloadModel(base, m); err != nil {
			return 0, err
		}
	}
	return len(models), nil
}

// FreeVRAM unloads all models from the local Ollama instance, freeing GPU
// memory without stopping the daemon.
func (m *ServiceManager) FreeVRAM() (int, error) {
	return freeVRAM(defaultOllamaBase)
}
