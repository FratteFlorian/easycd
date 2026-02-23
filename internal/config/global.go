package config

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// ProxmoxConfig holds connection details for a Proxmox VE server.
// Stored in ~/.config/simplecd/proxmox.yaml and shared across all projects.
type ProxmoxConfig struct {
	Host     string `yaml:"host"`
	Port     int    `yaml:"port"`
	Node     string `yaml:"node"`
	Token    string `yaml:"token"`   // PVEAPIToken=user@realm!id=secret
	Insecure bool   `yaml:"insecure"` // skip TLS verification
}

func globalConfigDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".config", "simplecd"), nil
}

func proxmoxConfigPath() (string, error) {
	dir, err := globalConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "proxmox.yaml"), nil
}

// LoadProxmoxConfig reads ~/.config/simplecd/proxmox.yaml.
// Returns nil, nil if the file does not exist.
func LoadProxmoxConfig() (*ProxmoxConfig, error) {
	path, err := proxmoxConfigPath()
	if err != nil {
		return nil, err
	}

	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("reading proxmox config: %w", err)
	}

	var cfg ProxmoxConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parsing proxmox config: %w", err)
	}

	// Env var overrides file token
	if t := os.Getenv("PROXMOX_TOKEN"); t != "" {
		cfg.Token = t
	}

	applyProxmoxDefaults(&cfg)
	return &cfg, nil
}

// SaveProxmoxConfig writes the config to ~/.config/simplecd/proxmox.yaml.
func SaveProxmoxConfig(cfg *ProxmoxConfig) error {
	dir, err := globalConfigDir()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	path := filepath.Join(dir, "proxmox.yaml")

	data, err := yaml.Marshal(cfg)
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0600)
}

func applyProxmoxDefaults(cfg *ProxmoxConfig) {
	if cfg.Port == 0 {
		cfg.Port = 8006
	}
	if cfg.Node == "" {
		cfg.Node = "pve"
	}
}
