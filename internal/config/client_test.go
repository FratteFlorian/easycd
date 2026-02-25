package config

import (
	"os"
	"path/filepath"
	"testing"
)

func writeConfig(t *testing.T, dir, content string) {
	t.Helper()
	eacdDir := filepath.Join(dir, ".eacd")
	if err := os.MkdirAll(eacdDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(eacdDir, "config.yaml"), []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
}

func TestLoadClientConfig_Valid(t *testing.T) {
	dir := t.TempDir()
	writeConfig(t, dir, `
name: my-api
server: http://192.168.1.50:8765
deploy:
  mappings:
    - src: ./dist
      dest: /usr/local/bin
`)
	cfg, err := LoadClientConfig(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Name != "my-api" {
		t.Errorf("Name = %q, want %q", cfg.Name, "my-api")
	}
	if cfg.Server != "http://192.168.1.50:8765" {
		t.Errorf("Server = %q", cfg.Server)
	}
	if len(cfg.Deploy.Mappings) != 1 {
		t.Errorf("expected 1 mapping, got %d", len(cfg.Deploy.Mappings))
	}
}

func TestLoadClientConfig_Defaults(t *testing.T) {
	dir := t.TempDir()
	writeConfig(t, dir, `
name: app
server: http://host:8765
deploy:
  mappings:
    - src: ./dist
      dest: /usr/local/bin
`)
	cfg, err := LoadClientConfig(dir)
	if err != nil {
		t.Fatal(err)
	}
	m := cfg.Deploy.Mappings[0]
	if m.Mode != "0644" {
		t.Errorf("default Mode = %q, want 0644", m.Mode)
	}
	if m.DirMode != "0755" {
		t.Errorf("default DirMode = %q, want 0755", m.DirMode)
	}
}

func TestLoadClientConfig_MissingName(t *testing.T) {
	dir := t.TempDir()
	writeConfig(t, dir, `
server: http://host:8765
deploy:
  mappings:
    - src: ./dist
      dest: /usr/local/bin
`)
	_, err := LoadClientConfig(dir)
	if err == nil {
		t.Error("expected error for missing name")
	}
}

func TestLoadClientConfig_MissingServer(t *testing.T) {
	dir := t.TempDir()
	writeConfig(t, dir, `
name: app
deploy:
  mappings:
    - src: ./dist
      dest: /usr/local/bin
`)
	_, err := LoadClientConfig(dir)
	if err == nil {
		t.Error("expected error for missing server")
	}
}

func TestLoadClientConfig_MissingMappings(t *testing.T) {
	dir := t.TempDir()
	writeConfig(t, dir, `
name: app
server: http://host:8765
`)
	_, err := LoadClientConfig(dir)
	if err == nil {
		t.Error("expected error for missing mappings")
	}
}

func TestLoadClientConfig_FileNotFound(t *testing.T) {
	_, err := LoadClientConfig(t.TempDir())
	if err == nil {
		t.Error("expected error when config file does not exist")
	}
}

func TestLoadClientConfig_TokenOptional(t *testing.T) {
	dir := t.TempDir()
	writeConfig(t, dir, `
name: app
server: http://host:8765
deploy:
  mappings:
    - src: ./dist
      dest: /usr/local/bin
`)
	cfg, err := LoadClientConfig(dir)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Token != "" {
		t.Errorf("expected empty token, got %q", cfg.Token)
	}
}

func TestLoadClientConfig_HooksAndSystemd(t *testing.T) {
	dir := t.TempDir()
	writeConfig(t, dir, `
name: app
server: http://host:8765
deploy:
  mappings:
    - src: ./dist
      dest: /usr/local/bin
  systemd:
    unit: .eacd/app.service
    enable: true
    restart: true
hooks:
  local_pre: .eacd/build.sh
  server_pre: .eacd/stop.sh
  server_post: .eacd/start.sh
`)
	cfg, err := LoadClientConfig(dir)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Deploy.Systemd == nil {
		t.Fatal("expected Systemd to be set")
	}
	if !cfg.Deploy.Systemd.Enable {
		t.Error("expected Systemd.Enable = true")
	}
	if cfg.Hooks.LocalPre != ".eacd/build.sh" {
		t.Errorf("LocalPre = %q", cfg.Hooks.LocalPre)
	}
	if cfg.Hooks.ServerPost != ".eacd/start.sh" {
		t.Errorf("ServerPost = %q", cfg.Hooks.ServerPost)
	}
}

func TestLoadClientConfig_ModeNotOverridden(t *testing.T) {
	dir := t.TempDir()
	writeConfig(t, dir, `
name: app
server: http://host:8765
deploy:
  mappings:
    - src: ./dist
      dest: /usr/local/bin
      mode: "0755"
      dir_mode: "0700"
`)
	cfg, err := LoadClientConfig(dir)
	if err != nil {
		t.Fatal(err)
	}
	m := cfg.Deploy.Mappings[0]
	if m.Mode != "0755" {
		t.Errorf("Mode should not be overridden, got %q", m.Mode)
	}
	if m.DirMode != "0700" {
		t.Errorf("DirMode should not be overridden, got %q", m.DirMode)
	}
}
