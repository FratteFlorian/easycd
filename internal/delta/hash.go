package delta

import (
	"crypto/sha256"
	"encoding/hex"
	"io"
	"os"
)

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
