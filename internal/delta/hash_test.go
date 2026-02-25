package delta

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestHashFile(t *testing.T) {
	f, err := os.CreateTemp(t.TempDir(), "hash-*")
	if err != nil {
		t.Fatal(err)
	}
	f.WriteString("hello eacd")
	f.Close()

	hash, err := HashFile(f.Name())
	if err != nil {
		t.Fatalf("HashFile failed: %v", err)
	}
	if !strings.HasPrefix(hash, "sha256:") {
		t.Errorf("expected sha256: prefix, got %q", hash)
	}
	// Same file hashed twice must be identical.
	hash2, _ := HashFile(f.Name())
	if hash != hash2 {
		t.Errorf("hashes differ for same file: %q vs %q", hash, hash2)
	}
}

func TestHashFile_Deterministic(t *testing.T) {
	dir := t.TempDir()
	a := filepath.Join(dir, "a.txt")
	b := filepath.Join(dir, "b.txt")
	os.WriteFile(a, []byte("same content"), 0644)
	os.WriteFile(b, []byte("same content"), 0644)

	ha, _ := HashFile(a)
	hb, _ := HashFile(b)
	if ha != hb {
		t.Errorf("files with identical content should have the same hash")
	}
}

func TestHashFile_DifferentContent(t *testing.T) {
	dir := t.TempDir()
	a := filepath.Join(dir, "a.txt")
	b := filepath.Join(dir, "b.txt")
	os.WriteFile(a, []byte("content A"), 0644)
	os.WriteFile(b, []byte("content B"), 0644)

	ha, _ := HashFile(a)
	hb, _ := HashFile(b)
	if ha == hb {
		t.Errorf("files with different content should have different hashes")
	}
}

func TestHashFile_Missing(t *testing.T) {
	_, err := HashFile("/nonexistent/path/file.txt")
	if err == nil {
		t.Error("expected error for missing file, got nil")
	}
}

func TestHashExistingFiles(t *testing.T) {
	dir := t.TempDir()
	existing := filepath.Join(dir, "exists.txt")
	os.WriteFile(existing, []byte("data"), 0644)
	missing := filepath.Join(dir, "missing.txt")

	result := HashExistingFiles([]string{existing, missing})

	if _, ok := result[existing]; !ok {
		t.Errorf("expected hash for existing file %q", existing)
	}
	if !strings.HasPrefix(result[existing], "sha256:") {
		t.Errorf("expected sha256: prefix, got %q", result[existing])
	}
	if _, ok := result[missing]; ok {
		t.Errorf("missing file should not appear in result")
	}
}

func TestHashExistingFiles_Empty(t *testing.T) {
	result := HashExistingFiles(nil)
	if len(result) != 0 {
		t.Errorf("expected empty map, got %v", result)
	}
}
