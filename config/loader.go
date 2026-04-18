package config

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

func LoadConfig(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config %s: %w", path, err)
	}
	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse config %s: %w", path, err)
	}
	return &cfg, nil
}

func LoadModel(path string) (*ModelConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read model %s: %w", path, err)
	}
	var m ModelConfig
	if err := yaml.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("parse model %s: %w", path, err)
	}
	if m.Model == "" {
		return nil, fmt.Errorf("model.model is required in %s", path)
	}
	if m.Official.URL == "" {
		return nil, fmt.Errorf("model.official.url is required in %s", path)
	}
	if m.Official.Key == "" {
		return nil, fmt.Errorf("model.official.key is required in %s", path)
	}
	return &m, nil
}
