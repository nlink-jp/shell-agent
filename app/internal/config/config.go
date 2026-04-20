package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
)

const (
	AppName    = "shell-agent"
	ConfigFile = "config.json"
)

// Config holds all application settings.
type Config struct {
	API       APIConfig        `json:"api"`
	Memory    MemoryConfig     `json:"memory"`
	Tools     ToolsConfig      `json:"tools"`
	Guardians []GuardianConfig `json:"guardians"`
	Window      WindowConfig     `json:"window"`
	Theme       string           `json:"theme"`
	StartupMode string           `json:"startup_mode"` // "new" or "last"
	LastSession string           `json:"last_session"`
	Location    LocationConfig   `json:"location"`
}

// LocationConfig holds location settings.
type LocationConfig struct {
	Enabled  bool   `json:"enabled"`
	Locality string `json:"locality"` // cached locality name
	AdminArea string `json:"admin_area"`
	Country  string `json:"country"`
	Lat      float64 `json:"lat"`
	Lon      float64 `json:"lon"`
}

// WindowConfig holds window position and size.
type WindowConfig struct {
	X              int  `json:"x"`
	Y              int  `json:"y"`
	Width          int  `json:"width"`
	Height         int  `json:"height"`
	SidebarWidth   int  `json:"sidebar_width"`
	SidebarCollapsed bool `json:"sidebar_collapsed"`
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
	MaxToolRounds     int `json:"max_tool_rounds"`
}

// ToolsConfig holds tool script settings.
type ToolsConfig struct {
	ScriptDir     string   `json:"script_dir"`
	DisabledTools []string `json:"disabled_tools"`
}

// GuardianConfig holds mcp-guardian settings for one MCP server.
type GuardianConfig struct {
	Name        string `json:"name"`
	BinaryPath  string `json:"binary_path"`
	ProfilePath string `json:"profile_path"`
}

// DefaultConfig returns sensible defaults.
func DefaultConfig() *Config {
	return &Config{
		API: APIConfig{
			Endpoint: "http://localhost:1234/v1",
			Model:    "google/gemma-4-26b-a4b",
		},
		Memory: MemoryConfig{
			HotTokenLimit:     65536,
			WarmRetentionMins: 60,
			ColdRetentionMins: 1440,
		},
		Tools: ToolsConfig{
			ScriptDir: filepath.Join(ConfigDir(), "tools"),
		},
		Guardians:   []GuardianConfig{},
		StartupMode: "last",
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

	// Apply minimum defaults for critical values
	if cfg.Memory.HotTokenLimit < 8192 {
		cfg.Memory.HotTokenLimit = 65536
	}

	return &cfg, nil
}

// ExpandPath resolves ~ and environment variables in a path.
func ExpandPath(path string) string {
	if path == "" {
		return path
	}
	// Expand environment variables ($HOME, ${VAR}, etc.)
	path = os.ExpandEnv(path)
	// Expand ~ to home directory
	if strings.HasPrefix(path, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			path = filepath.Join(home, path[2:])
		}
	} else if path == "~" {
		if home, err := os.UserHomeDir(); err == nil {
			path = home
		}
	}
	return path
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
