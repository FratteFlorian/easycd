package archive

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestShouldExclude(t *testing.T) {
	cases := []struct {
		rel     string
		isDir   bool
		pattern string
		want    bool
	}{
		{"vendor", true, "vendor/", true},
		{"vendor/autoload.php", false, "vendor/", true},
		{"app/vendor", false, "vendor/", false},
		{"node_modules", true, "node_modules/", true},
		{".env", false, ".env", true},
		{"app/.env", false, ".env", true}, // basename match: .env matches at any depth
		{"debug.log", false, "*.log", true},
		{"logs/app.log", false, "*.log", true},
		{"README.md", false, "*.log", false},
	}

	for _, c := range cases {
		got := ShouldExclude(c.rel, c.isDir, []string{c.pattern})
		if got != c.want {
			t.Errorf("shouldExclude(%q, isDir=%v, %q) = %v, want %v", c.rel, c.isDir, c.pattern, got, c.want)
		}
	}
}

func TestAddDirAndExtract(t *testing.T) {
	// Create a temp source directory with some files
	srcDir := t.TempDir()
	os.WriteFile(filepath.Join(srcDir, "hello.txt"), []byte("hello"), 0644)
	os.WriteFile(filepath.Join(srcDir, "secret.log"), []byte("log"), 0644)
	os.MkdirAll(filepath.Join(srcDir, "vendor"), 0755)
	os.WriteFile(filepath.Join(srcDir, "vendor", "lib.php"), []byte("<?php"), 0644)

	// Build archive excluding vendor/ and *.log
	var buf bytes.Buffer
	tw, gw := NewWriter(&buf)
	if err := AddDir(tw, srcDir, "files", []string{"vendor/", "*.log"}, 0644, 0755); err != nil {
		t.Fatal(err)
	}
	tw.Close()
	gw.Close()

	// Extract
	destDir := t.TempDir()
	if err := Extract(bytes.NewReader(buf.Bytes()), destDir, "files"); err != nil {
		t.Fatal(err)
	}

	// hello.txt should be present
	if _, err := os.Stat(filepath.Join(destDir, "files", "hello.txt")); err != nil {
		t.Errorf("hello.txt should exist: %v", err)
	}

	// secret.log should be excluded
	if _, err := os.Stat(filepath.Join(destDir, "files", "secret.log")); err == nil {
		t.Error("secret.log should have been excluded")
	}

	// vendor/ should be excluded
	if _, err := os.Stat(filepath.Join(destDir, "files", "vendor")); err == nil {
		t.Error("vendor/ should have been excluded")
	}
}
