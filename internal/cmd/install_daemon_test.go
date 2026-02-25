package cmd

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestUpdateClientConfig_ReplacesServerAndToken(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, ".eacd", "config.yaml")
	os.MkdirAll(filepath.Dir(cfgPath), 0755)
	os.WriteFile(cfgPath, []byte(`name: my-app
server: http://old-host:8765
token: old-token
deploy:
  mappings:
    - src: ./dist
      dest: /usr/local/bin
`), 0644)

	wd, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(wd)

	if err := updateClientConfig("http://new-host:8765", "new-token", io.Discard); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	data, _ := os.ReadFile(cfgPath)
	content := string(data)

	if !strings.Contains(content, "server: http://new-host:8765") {
		t.Errorf("server not updated, got:\n%s", content)
	}
	if !strings.Contains(content, "token: new-token") {
		t.Errorf("token not updated, got:\n%s", content)
	}
	// Other fields must be preserved.
	if !strings.Contains(content, "name: my-app") {
		t.Errorf("name field should be preserved, got:\n%s", content)
	}
}

func TestUpdateClientConfig_AppendsIfMissing(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, ".eacd", "config.yaml")
	os.MkdirAll(filepath.Dir(cfgPath), 0755)
	os.WriteFile(cfgPath, []byte(`name: my-app
deploy:
  mappings:
    - src: ./dist
      dest: /usr/local/bin
`), 0644)

	wd, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(wd)

	if err := updateClientConfig("http://192.168.1.50:8765", "abc123", io.Discard); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	data, _ := os.ReadFile(cfgPath)
	content := string(data)

	if !strings.Contains(content, "server: http://192.168.1.50:8765") {
		t.Errorf("server should be appended, got:\n%s", content)
	}
	if !strings.Contains(content, "token: abc123") {
		t.Errorf("token should be appended, got:\n%s", content)
	}
}

func TestUpdateClientConfig_FileNotFound(t *testing.T) {
	wd, _ := os.Getwd()
	os.Chdir(t.TempDir())
	defer os.Chdir(wd)

	err := updateClientConfig("http://host:8765", "token", io.Discard)
	if err == nil {
		t.Error("expected error when config file does not exist")
	}
}

func TestFindSSHKey_ReturnsEmptyWhenNoneExist(t *testing.T) {
	// Override HOME to a temp dir that has no .ssh/ directory.
	orig := os.Getenv("HOME")
	os.Setenv("HOME", t.TempDir())
	defer os.Setenv("HOME", orig)

	key := findSSHKey()
	if key != "" {
		t.Errorf("expected empty string, got %q", key)
	}
}

func TestFindSSHKey_FindsEd25519(t *testing.T) {
	home := t.TempDir()
	sshDir := filepath.Join(home, ".ssh")
	os.MkdirAll(sshDir, 0700)
	os.WriteFile(filepath.Join(sshDir, "id_ed25519"), []byte("fake key"), 0600)

	orig := os.Getenv("HOME")
	os.Setenv("HOME", home)
	defer os.Setenv("HOME", orig)

	key := findSSHKey()
	if !strings.HasSuffix(key, "id_ed25519") {
		t.Errorf("expected id_ed25519, got %q", key)
	}
}

func TestFindSSHKey_FallsBackToRSA(t *testing.T) {
	home := t.TempDir()
	sshDir := filepath.Join(home, ".ssh")
	os.MkdirAll(sshDir, 0700)
	os.WriteFile(filepath.Join(sshDir, "id_rsa"), []byte("fake key"), 0600)

	orig := os.Getenv("HOME")
	os.Setenv("HOME", home)
	defer os.Setenv("HOME", orig)

	key := findSSHKey()
	if !strings.HasSuffix(key, "id_rsa") {
		t.Errorf("expected id_rsa fallback, got %q", key)
	}
}

func TestFindSSHKey_PrefersEd25519OverRSA(t *testing.T) {
	home := t.TempDir()
	sshDir := filepath.Join(home, ".ssh")
	os.MkdirAll(sshDir, 0700)
	os.WriteFile(filepath.Join(sshDir, "id_ed25519"), []byte("ed25519 key"), 0600)
	os.WriteFile(filepath.Join(sshDir, "id_rsa"), []byte("rsa key"), 0600)

	orig := os.Getenv("HOME")
	os.Setenv("HOME", home)
	defer os.Setenv("HOME", orig)

	key := findSSHKey()
	if !strings.HasSuffix(key, "id_ed25519") {
		t.Errorf("should prefer id_ed25519 over id_rsa, got %q", key)
	}
}
