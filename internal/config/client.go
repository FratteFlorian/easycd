package config

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// ClientConfig is loaded from .simplecd/config.yaml in the project root.
type ClientConfig struct {
	Name   string       `yaml:"name"`
	Server string       `yaml:"server"`
	Token  string       `yaml:"token"`
	Deploy DeployConfig `yaml:"deploy"`
	Hooks  ClientHooks  `yaml:"hooks"`
}

// DeployConfig describes what to deploy and where.
type DeployConfig struct {
	Mappings []Mapping    `yaml:"mappings"`
	Systemd  *SystemdSpec `yaml:"systemd"`
}

// Mapping maps a local source folder to a remote destination folder.
type Mapping struct {
	Src     string   `yaml:"src"`
	Dest    string   `yaml:"dest"`
	Mode    string   `yaml:"mode"`     // file mode, e.g. "0644"
	DirMode string   `yaml:"dir_mode"` // directory mode, e.g. "0755"
	Exclude []string `yaml:"exclude"`  // glob/prefix patterns to skip
}

// SystemdSpec describes an optional systemd unit to deploy.
type SystemdSpec struct {
	Unit    string `yaml:"unit"`
	Enable  bool   `yaml:"enable"`
	Restart bool   `yaml:"restart"`
}

// ClientHooks holds paths to hook scripts (relative to project root).
type ClientHooks struct {
	LocalPre   string `yaml:"local_pre"`
	ServerPre  string `yaml:"server_pre"`
	ServerPost string `yaml:"server_post"`
}

// LoadClientConfig reads and parses .simplecd/config.yaml from the given directory.
func LoadClientConfig(projectDir string) (*ClientConfig, error) {
	path := projectDir + "/.simplecd/config.yaml"
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("cannot read %s: %w", path, err)
	}

	var cfg ClientConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("cannot parse %s: %w", path, err)
	}

	if cfg.Name == "" {
		return nil, fmt.Errorf("%s: 'name' is required", path)
	}
	if cfg.Server == "" {
		return nil, fmt.Errorf("%s: 'server' is required", path)
	}
	if len(cfg.Deploy.Mappings) == 0 {
		return nil, fmt.Errorf("%s: at least one deploy.mapping is required", path)
	}

	// Apply defaults
	for i := range cfg.Deploy.Mappings {
		if cfg.Deploy.Mappings[i].Mode == "" {
			cfg.Deploy.Mappings[i].Mode = "0644"
		}
		if cfg.Deploy.Mappings[i].DirMode == "" {
			cfg.Deploy.Mappings[i].DirMode = "0755"
		}
	}

	return &cfg, nil
}
