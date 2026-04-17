package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()

	if cfg.API.Endpoint != "http://localhost:1234/v1" {
		t.Errorf("unexpected endpoint: %s", cfg.API.Endpoint)
	}
	if cfg.API.Model != "google/gemma-4-26b-a4b" {
		t.Errorf("unexpected model: %s", cfg.API.Model)
	}
	if cfg.Memory.HotTokenLimit != 4096 {
		t.Errorf("unexpected hot token limit: %d", cfg.Memory.HotTokenLimit)
	}
}

func TestConfigSaveLoad(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ConfigFile)

	cfg := DefaultConfig()
	cfg.API.Model = "test-model"

	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	var loaded Config
	if err := json.Unmarshal(got, &loaded); err != nil {
		t.Fatal(err)
	}

	if loaded.API.Model != "test-model" {
		t.Errorf("expected test-model, got %s", loaded.API.Model)
	}
}

func TestConfigJSON(t *testing.T) {
	cfg := DefaultConfig()
	data, err := json.Marshal(cfg)
	if err != nil {
		t.Fatal(err)
	}

	var loaded Config
	if err := json.Unmarshal(data, &loaded); err != nil {
		t.Fatal(err)
	}

	if loaded.API.Endpoint != cfg.API.Endpoint {
		t.Errorf("endpoint mismatch")
	}
	if loaded.Memory.HotTokenLimit != cfg.Memory.HotTokenLimit {
		t.Errorf("hot token limit mismatch")
	}
}
