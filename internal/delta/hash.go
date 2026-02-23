package delta

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/flo-mic/simplecd/internal/archive"
	"github.com/flo-mic/simplecd/internal/config"
)

// FileHash holds a destination path and its SHA256 hash.
type FileHash struct {
	Dest string
	Hash string
}

// HashMapping computes SHA256 hashes for all files in a Mapping.
// It returns a slice of FileHash entries mapping each file's remote dest to its hash.
func HashMapping(m config.Mapping, projectDir string) ([]FileHash, error) {
	srcDir := filepath.Join(projectDir, m.Src)
	destDir := m.Dest

	var results []FileHash
	err := filepath.Walk(srcDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}

		rel, err := filepath.Rel(srcDir, path)
		if err != nil {
			return err
		}

		if archive.ShouldExclude(rel, false, m.Exclude) {
			return nil
		}

		hash, err := hashFile(path)
		if err != nil {
			return fmt.Errorf("hashing %s: %w", path, err)
		}

		dest := filepath.Join(destDir, rel)
		results = append(results, FileHash{Dest: dest, Hash: "sha256:" + hash})
		return nil
	})
	return results, err
}

// HashFile computes the SHA256 hash of a single file and returns "sha256:<hex>".
func HashFile(path string) (string, error) {
	h, err := hashFile(path)
	if err != nil {
		return "", err
	}
	return "sha256:" + h, nil
}

func hashFile(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// HashExistingFiles computes SHA256 hashes for files already on disk (server side).
// Returns a map of dest path → hash for files that exist.
func HashExistingFiles(dests []string) map[string]string {
	result := make(map[string]string, len(dests))
	for _, dest := range dests {
		h, err := hashFile(dest)
		if err != nil {
			// File doesn't exist or can't be read — treat as missing
			continue
		}
		result[dest] = "sha256:" + h
	}
	return result
}
