package config

import (
	"os"
)

// Config holds runtime-only settings. Model configs are stored in the DB.
type Config struct {
	RegToken string `json:"-"`
}

func DefaultConfig() *Config {
	return &Config{
		RegToken: os.Getenv("GATEWAY_TOKEN"),
	}
}
