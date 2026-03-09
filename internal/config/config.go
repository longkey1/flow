package config

import (
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Dir string `yaml:"dir"`
}

func Load(baseDir string) (*Config, error) {
	cfg := &Config{Dir: ".flow"}

	data, err := os.ReadFile(filepath.Join(baseDir, ".flow.yaml"))
	if err != nil {
		if os.IsNotExist(err) {
			return cfg, nil
		}
		return nil, err
	}

	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, err
	}

	if cfg.Dir == "" {
		cfg.Dir = ".flow"
	}

	return cfg, nil
}

func (c *Config) WorkflowsDir(baseDir string) string {
	return filepath.Join(baseDir, c.Dir, "workflows")
}
