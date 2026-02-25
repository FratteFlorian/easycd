package inventory

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/flo-mic/eacd/internal/api"
)

// patchDropinBase redirects drop-in writes to a temp dir for tests.
func patchDropinBase(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	orig := dropinBaseDir
	dropinBaseDir = dir
	t.Cleanup(func() { dropinBaseDir = orig })
	return dir
}

// noopDaemonReload replaces systemctl daemon-reload with a no-op for tests.
func patchDaemonReload(t *testing.T) {
	t.Helper()
	orig := daemonReload
	daemonReload = func(log io.Writer) error { return nil }
	t.Cleanup(func() { daemonReload = orig })
}

// --- buildDropinContent ---

func TestBuildDropinContent_SortedKeys(t *testing.T) {
	env := map[string]string{
		"PORT":         "8080",
		"DATABASE_URL": "postgres://localhost/app",
		"APP_ENV":      "production",
	}
	content := buildDropinContent(env)

	lines := strings.Split(strings.TrimSpace(content), "\n")
	if lines[0] != "[Service]" {
		t.Errorf("first line should be [Service], got %q", lines[0])
	}
	// Keys must appear in alphabetical order.
	if !strings.Contains(lines[1], "APP_ENV") {
		t.Errorf("APP_ENV should be first key, got %q", lines[1])
	}
	if !strings.Contains(lines[2], "DATABASE_URL") {
		t.Errorf("DATABASE_URL should be second key, got %q", lines[2])
	}
	if !strings.Contains(lines[3], "PORT") {
		t.Errorf("PORT should be third key, got %q", lines[3])
	}
}

func TestBuildDropinContent_Format(t *testing.T) {
	content := buildDropinContent(map[string]string{"KEY": "value"})
	want := "[Service]\nEnvironment=\"KEY=value\"\n"
	if content != want {
		t.Errorf("got %q, want %q", content, want)
	}
}

func TestBuildDropinContent_EscapesQuotes(t *testing.T) {
	content := buildDropinContent(map[string]string{"KEY": `val"ue`})
	if !strings.Contains(content, `val\"ue`) {
		t.Errorf("double quotes should be escaped, got %q", content)
	}
}

func TestBuildDropinContent_Stable(t *testing.T) {
	env := map[string]string{"Z": "last", "A": "first", "M": "middle"}
	c1 := buildDropinContent(env)
	c2 := buildDropinContent(env)
	if c1 != c2 {
		t.Error("buildDropinContent should be deterministic")
	}
}

// --- reconcileServiceEnv ---

func TestReconcileServiceEnv_WritesDropin(t *testing.T) {
	base := patchDropinBase(t)
	patchDaemonReload(t)

	svc := api.InventoryService{
		Name: "my-api",
		Env:  map[string]string{"PORT": "8080"},
	}

	changed, err := reconcileServiceEnv(svc, io.Discard)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !changed {
		t.Error("expected changed=true on first write")
	}

	dropinFile := filepath.Join(base, "my-api.service.d", "eacd-env.conf")
	data, err := os.ReadFile(dropinFile)
	if err != nil {
		t.Fatalf("drop-in file not created: %v", err)
	}
	if !strings.Contains(string(data), `Environment="PORT=8080"`) {
		t.Errorf("unexpected drop-in content: %q", string(data))
	}
}

func TestReconcileServiceEnv_Idempotent(t *testing.T) {
	patchDropinBase(t)
	patchDaemonReload(t)

	svc := api.InventoryService{
		Name: "my-api",
		Env:  map[string]string{"PORT": "8080"},
	}

	reconcileServiceEnv(svc, io.Discard) // first write

	changed, err := reconcileServiceEnv(svc, io.Discard) // second write — same content
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if changed {
		t.Error("expected changed=false when content is unchanged")
	}
}

func TestReconcileServiceEnv_UpdatesOnChange(t *testing.T) {
	base := patchDropinBase(t)
	patchDaemonReload(t)

	svc := api.InventoryService{Name: "my-api", Env: map[string]string{"PORT": "8080"}}
	reconcileServiceEnv(svc, io.Discard)

	svc.Env["PORT"] = "9090"
	changed, err := reconcileServiceEnv(svc, io.Discard)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !changed {
		t.Error("expected changed=true after env update")
	}

	data, _ := os.ReadFile(filepath.Join(base, "my-api.service.d", "eacd-env.conf"))
	if !strings.Contains(string(data), `"PORT=9090"`) {
		t.Errorf("drop-in should contain updated port, got: %q", string(data))
	}
}

func TestReconcileServiceEnv_RemovesDropin(t *testing.T) {
	base := patchDropinBase(t)
	patchDaemonReload(t)

	svc := api.InventoryService{Name: "my-api", Env: map[string]string{"PORT": "8080"}}
	reconcileServiceEnv(svc, io.Discard) // create drop-in

	// Now remove env entirely.
	svc.Env = nil
	changed, err := reconcileServiceEnv(svc, io.Discard)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !changed {
		t.Error("expected changed=true when drop-in is removed")
	}

	dropinFile := filepath.Join(base, "my-api.service.d", "eacd-env.conf")
	if _, err := os.Stat(dropinFile); err == nil {
		t.Error("drop-in file should have been deleted")
	}
}

func TestReconcileServiceEnv_NoopWhenEmptyAndNoDropin(t *testing.T) {
	patchDropinBase(t)
	patchDaemonReload(t)

	svc := api.InventoryService{Name: "my-api", Env: nil}
	changed, err := reconcileServiceEnv(svc, io.Discard)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if changed {
		t.Error("expected changed=false when env is empty and no drop-in exists")
	}
}
