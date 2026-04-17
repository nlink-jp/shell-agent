package config

import (
	"encoding/json"
	"os"
	"path/filepath"
)

const (
	AppName    = "shell-agent"
	ConfigFile = "config.json"
)

// Config holds all application settings.
type Config struct {
	API      APIConfig      `json:"api"`
	Memory   MemoryConfig   `json:"memory"`
	Tools    ToolsConfig    `json:"tools"`
	Guardian GuardianConfig `json:"guardian"`
	Window   WindowConfig   `json:"window"`
}

// WindowConfig holds window position and size.
type WindowConfig struct {
	X      int `json:"x"`
	Y      int `json:"y"`
	Width  int `json:"width"`
	Height int `json:"height"`
}

// APIConfig holds LLM API connection settings.
type APIConfig struct {
	Endpoint string `json:"endpoint"`
	Model    string `json:"model"`
	APIKey   string `json:"api_key,omitempty"`
}

// MemoryConfig holds memory tier settings.
type MemoryConfig struct {
	HotTokenLimit     int `json:"hot_token_limit"`
	WarmRetentionMins int `json:"warm_retention_mins"`
	ColdRetentionMins int `json:"cold_retention_mins"`
}

// ToolsConfig holds tool script settings.
type ToolsConfig struct {
	ScriptDir string `json:"script_dir"`
}

// GuardianConfig holds mcp-guardian settings.
type GuardianConfig struct {
	BinaryPath string `json:"binary_path"`
	ConfigPath string `json:"config_path"`
}

// DefaultConfig returns sensible defaults.
func DefaultConfig() *Config {
	return &Config{
		API: APIConfig{
			Endpoint: "http://localhost:1234/v1",
			Model:    "google/gemma-4-26b-a4b",
		},
		Memory: MemoryConfig{
			HotTokenLimit:     4096,
			WarmRetentionMins: 60,
			ColdRetentionMins: 1440,
		},
		Tools: ToolsConfig{
			ScriptDir: filepath.Join(ConfigDir(), "tools"),
		},
		Guardian: GuardianConfig{
			BinaryPath: "mcp-guardian",
		},
	}
}

// ConfigDir returns the application support directory.
func ConfigDir() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, "Library", "Application Support", AppName)
}

// Load reads config from disk, creating defaults if missing.
func Load() (*Config, error) {
	dir := ConfigDir()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}

	path := filepath.Join(dir, ConfigFile)
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			cfg := DefaultConfig()
			return cfg, cfg.Save()
		}
		return nil, err
	}

	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}

// Save writes the config to disk.
func (c *Config) Save() error {
	dir := ConfigDir()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}

	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, ConfigFile), data, 0o644)
}
