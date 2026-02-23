package config

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// ServerConfig is loaded from /etc/eacd/server.yaml on the CT.
type ServerConfig struct {
	Listen string `yaml:"listen"`   // e.g. ":8765"
	Token  string `yaml:"token"`
	LogDir string `yaml:"log_dir"`
}

// LoadServerConfig reads and parses the server config file.
func LoadServerConfig(path string) (*ServerConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("cannot read %s: %w", path, err)
	}

	var cfg ServerConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("cannot parse %s: %w", path, err)
	}

	if cfg.Token == "" {
		return nil, fmt.Errorf("%s: 'token' is required", path)
	}
	if cfg.Listen == "" {
		cfg.Listen = ":8765"
	}
	if cfg.LogDir == "" {
		cfg.LogDir = "/var/log/eacd"
	}

	return &cfg, nil
}
