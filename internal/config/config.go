package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/anomalyco/sek/internal/llm"
)

type Config struct {
	ProjectDir string   `json:"project_dir,omitempty"`
	DataDir    string   `json:"data_dir,omitempty"`
	LLM        llm.Config `json:"llm,omitempty"`

	Store struct {
		Path string `json:"path,omitempty"`
	} `json:"store,omitempty"`

	MCP struct {
		HTTPAddr string `json:"http_addr,omitempty"`
	} `json:"mcp,omitempty"`
}

func DefaultPath(projectDir string) string {
	return filepath.Join(projectDir, ".sek", "config.json")
}

func defaultDataDir() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".sek")
}

func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config: %w", err)
	}
	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}
	return &cfg, nil
}

func (c *Config) Save(path string) error {
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return fmt.Errorf("create config dir: %w", err)
	}
	return os.WriteFile(path, data, 0644)
}

func (c *Config) StorePath() string {
	if c.Store.Path != "" {
		return c.Store.Path
	}
	return filepath.Join(c.ProjectDir, ".sek", "store.db")
}

func (c *Config) DataDirPath() string {
	if c.DataDir != "" {
		return c.DataDir
	}
	return defaultDataDir()
}
