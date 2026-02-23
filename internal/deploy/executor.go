package deploy

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
)

// PlaceFile copies a file from src to dest with the given octal mode string (e.g. "0755").
// It creates parent directories as needed.
func PlaceFile(src, dest, modeStr string, log io.Writer) error {
	mode, err := parseMode(modeStr, 0644)
	if err != nil {
		return fmt.Errorf("invalid mode %q: %w", modeStr, err)
	}

	if err := os.MkdirAll(filepath.Dir(dest), 0755); err != nil {
		return fmt.Errorf("mkdir %s: %w", filepath.Dir(dest), err)
	}

	in, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("open src %s: %w", src, err)
	}
	defer in.Close()

	out, err := os.OpenFile(dest, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, mode)
	if err != nil {
		return fmt.Errorf("open dest %s: %w", dest, err)
	}
	defer out.Close()

	if _, err := io.Copy(out, in); err != nil {
		return fmt.Errorf("copy to %s: %w", dest, err)
	}

	fmt.Fprintf(log, "[eacd] Placed %s (mode %s)\n", dest, modeStr)
	return nil
}

func parseMode(s string, fallback os.FileMode) (os.FileMode, error) {
	if s == "" {
		return fallback, nil
	}
	v, err := strconv.ParseUint(s, 8, 32)
	if err != nil {
		return 0, err
	}
	return os.FileMode(v), nil
}
