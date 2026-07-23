package config

import (
	"os"
	"path/filepath"
)

// Config holds runtime-only settings. Model configs are stored in the DB.
type Config struct {
	RegToken          string `json:"-"`
	DataDir           string `json:"-"`
	RestrictWorkspace bool   `json:"-"`
}

func DefaultConfig() *Config {
	dir := os.Getenv("DATA_DIR")
	if dir == "" {
		home, _ := os.UserHomeDir()
		dir = filepath.Join(home, ".beleader", "runtime")
	}
	return &Config{
		RegToken:          os.Getenv("GATEWAY_TOKEN"),
		DataDir:           dir,
		RestrictWorkspace: os.Getenv("RESTRICT_WORKSPACE") == "true",
	}
}

// ModelProfile is used for JSON serialization in the settings API.
// The DB-persisted model is defined in the db package.
type ModelProfile struct {
	ID              string `json:"id"`
	BaseURL         string `json:"base_url"`
	APIKey          string `json:"api_key"`
	Model           string `json:"model"`
	Vision          bool   `json:"vision"`
	ContextLimit    int    `json:"context_limit"`
	ReasoningEffort string `json:"reasoning_effort"`
}
