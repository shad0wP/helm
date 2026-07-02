package service

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestFreeVRAM(t *testing.T) {
	t.Run("unloads every loaded model", func(t *testing.T) {
		var generated []string
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch r.URL.Path {
			case "/api/ps":
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{"models":[{"name":"llama3:8b"},{"name":"qwen2:7b"}]}`))
			case "/api/generate":
				// Record that an eviction generate was issued.
				generated = append(generated, r.Method)
				w.WriteHeader(http.StatusOK)
			default:
				w.WriteHeader(http.StatusNotFound)
			}
		}))
		defer srv.Close()

		n, err := freeVRAM(srv.URL)
		if err != nil {
			t.Fatalf("freeVRAM err: %v", err)
		}
		if n != 2 {
			t.Errorf("unloaded %d, want 2", n)
		}
		if len(generated) != 2 {
			t.Errorf("issued %d generate calls, want 2", len(generated))
		}
	})

	t.Run("no models loaded returns zero", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte(`{"models":[]}`))
		}))
		defer srv.Close()
		n, err := freeVRAM(srv.URL)
		if err != nil || n != 0 {
			t.Errorf("got n=%d err=%v, want 0, nil", n, err)
		}
	})

	t.Run("daemon down surfaces an error", func(t *testing.T) {
		// Nothing listening on this base.
		if _, err := freeVRAM("http://127.0.0.1:1"); err == nil {
			t.Error("expected an error when Ollama is unreachable")
		}
	})

	t.Run("non-200 on ps is an error", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
		}))
		defer srv.Close()
		if _, err := freeVRAM(srv.URL); err == nil {
			t.Error("expected an error on HTTP 500")
		}
	})
}
