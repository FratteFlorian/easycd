package cmd

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"

	"github.com/flo-mic/eacd/internal/config"
)

// Rollback sends a rollback request to the server for the current project.
func Rollback(args []string, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("rollback", flag.ContinueOnError)
	fs.SetOutput(stderr)
	dir := fs.String("dir", ".", "Project directory (default: current directory)")
	if err := fs.Parse(args); err != nil {
		return err
	}

	projectDir, err := filepath.Abs(*dir)
	if err != nil {
		return fmt.Errorf("resolving project dir: %w", err)
	}

	cfg, err := config.LoadClientConfig(projectDir)
	if err != nil {
		return err
	}

	token := os.Getenv("EACD_TOKEN")
	if token == "" && cfg.Token != "" {
		token = cfg.Token
	}
	if token == "" {
		return fmt.Errorf("no auth token: set EACD_TOKEN or add 'token:' to .eacd/config.yaml")
	}

	body, _ := json.Marshal(map[string]string{"name": cfg.Name})
	resp, err := httpPost(cfg.Server+"/rollback", token, "application/json", body)
	if err != nil {
		return fmt.Errorf("rollback request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		errBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("rollback failed (%d): %s", resp.StatusCode, bytes.TrimSpace(errBody))
	}
	return streamAndCheck(resp.Body, stdout, "rollback failed (see output above)")
}
