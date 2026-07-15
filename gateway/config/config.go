package config

import (
	"os"
	"path/filepath"
)

// Config holds runtime-only settings. Model configs are stored in the DB.
type Config struct {
	RuntimeURL string `json:"runtime_url"`
}

func DefaultConfig() *Config {
	return &Config{
		RuntimeURL: "http://127.0.0.1:8081",
	}
}

func ConfigDir() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".beleader")
}

func DBPath() string {
	if p := os.Getenv("DB_PATH"); p != "" {
		return p
	}
	return filepath.Join(ConfigDir(), "beleader.db")
}

func (c *Config) ActiveModel() *ModelProfile {
	return nil
}

func (c *Config) ResolveModel(sessionModelID string) *ModelProfile {
	return nil
}

// ModelProfile is used for JSON serialization in the settings API.
// The DB-persisted model is defined in the db package.
type ModelProfile struct {
	ID           string `json:"id"`
	BaseURL      string `json:"base_url"`
	APIKey       string `json:"api_key"`
	Model        string `json:"model"`
	Vision       bool   `json:"vision"`
	ContextLimit    int    `json:"context_limit"`
	ReasoningEffort string `json:"reasoning_effort"`
}
