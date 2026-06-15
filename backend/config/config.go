package config

import (
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

type Config struct {
	LLM        LLMConfig       `yaml:"llm" json:"llm"`
	HC         HCConfig        `yaml:"hc" json:"hc"`
	Thresholds ThresholdConfig `yaml:"thresholds" json:"thresholds"`
	Browser      BrowserConfig   `yaml:"browser" json:"browser"`
	SpeakEnabled bool            `yaml:"speak_enabled" json:"speak_enabled"`
	WorkDir      string          `yaml:"work_dir" json:"work_dir"`

	path string `yaml:"-" json:"-"`
}

type BrowserConfig struct {
	Headless bool `yaml:"headless" json:"headless"`
}

type ModelProfile struct {
	ID           string `yaml:"id" json:"id"`
	BaseURL      string `yaml:"base_url" json:"base_url"`
	APIKey       string `yaml:"api_key" json:"api_key"`
	Model        string `yaml:"model" json:"model"`
	Vision       bool   `yaml:"vision" json:"vision"`
	ContextLimit int    `yaml:"context_limit" json:"context_limit"`
}

type LLMConfig struct {
	Models []ModelProfile `yaml:"models" json:"models"`
	Active string         `yaml:"active" json:"active"`
}

type HCConfig struct {
	Max int `yaml:"max" json:"max"`
}

type ThresholdConfig struct {
	MaxContextPct int `yaml:"max_context_pct" json:"max_context_pct"`
}

func DefaultConfig() *Config {
	home, _ := os.UserHomeDir()
	return &Config{
		LLM: LLMConfig{
			Models: []ModelProfile{},
			Active: "",
		},
		HC: HCConfig{
			Max: 5,
		},
		Thresholds: ThresholdConfig{
			MaxContextPct: 60,
		},
		Browser: BrowserConfig{
			Headless: true,
		},
		SpeakEnabled: true,
		WorkDir: filepath.Join(home, ".beleader", "projects"),
	}
}

func (c *Config) ActiveModel() *ModelProfile {
	for i := range c.LLM.Models {
		if c.LLM.Models[i].ID == c.LLM.Active {
			return &c.LLM.Models[i]
		}
	}
	if len(c.LLM.Models) > 0 {
		return &c.LLM.Models[0]
	}
	return nil
}

func (c *Config) ModelByID(id string) *ModelProfile {
	for i := range c.LLM.Models {
		if c.LLM.Models[i].ID == id {
			return &c.LLM.Models[i]
		}
	}
	return c.ActiveModel()
}

func (c *Config) ResolveModel(sessionModelID string) *ModelProfile {
	if sessionModelID != "" {
		if m := c.ModelByID(sessionModelID); m != nil {
			return m
		}
	}
	return c.ActiveModel()
}

func Load(path string) (*Config, error) {
	cfg := DefaultConfig()
	cfg.path = path

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			if saveErr := cfg.Save(); saveErr != nil {
				return cfg, nil
			}
			return cfg, nil
		}
		return nil, err
	}
	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, err
	}
	for i := range cfg.LLM.Models {
		if cfg.LLM.Models[i].ContextLimit == 0 {
			cfg.LLM.Models[i].ContextLimit = 128000
		}
	}
	return cfg, nil
}

func (c *Config) Save() error {
	if c.path == "" {
		return nil
	}
	dir := filepath.Dir(c.path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	data, err := yaml.Marshal(c)
	if err != nil {
		return err
	}
	return os.WriteFile(c.path, data, 0644)
}

func (c *Config) Path() string {
	return c.path
}

func (c *Config) SetPath(p string) {
	c.path = p
}

func (c *Config) ConfigDir() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".beleader")
}

func (c *Config) DBPath() string {
	return filepath.Join(c.ConfigDir(), "beleader.db")
}

func (c *Config) ProjectDir(refID string) string {
	return filepath.Join(c.WorkDir, refID)
}

func (c *Config) StatePath(refID string) string {
	return filepath.Join(c.ProjectDir(refID), "state.md")
}

func (c *Config) StatusPath(refID string) string {
	return filepath.Join(c.ProjectDir(refID), "STATUS.md")
}

func (c *Config) PlanPath(refID string) string {
	return filepath.Join(c.ProjectDir(refID), "plan.md")
}

func (c *Config) PlansDir(refID string) string {
	return filepath.Join(c.ProjectDir(refID), "plans")
}

func (c *Config) ScreenshotDir(refID string) string {
	return filepath.Join(c.ProjectDir(refID), "screenshots")
}

func (c *Config) BrowserProfileDir() string {
	return filepath.Join(c.ConfigDir(), "browser-profile")
}
