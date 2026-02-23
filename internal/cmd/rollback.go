package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"

	"github.com/flo-mic/simplecd/internal/config"
)

// Rollback sends a rollback request to the server for the current project.
func Rollback(args []string, stdout, stderr io.Writer) error {
	dir := "."
	for _, a := range args {
		if a != "--" {
			dir = a
		}
	}

	projectDir, err := filepath.Abs(dir)
	if err != nil {
		return fmt.Errorf("resolving project dir: %w", err)
	}

	cfg, err := config.LoadClientConfig(projectDir)
	if err != nil {
		return err
	}

	token := os.Getenv("SIMPLECD_TOKEN")
	if token == "" && cfg.Token != "" {
		token = cfg.Token
	}
	if token == "" {
		return fmt.Errorf("no auth token: set SIMPLECD_TOKEN or add 'token:' to .simplecd/config.yaml")
	}

	body, _ := json.Marshal(map[string]string{"name": cfg.Name})
	resp, err := httpPost(cfg.Server+"/rollback", token, "application/json", body)
	if err != nil {
		return fmt.Errorf("rollback request: %w", err)
	}
	defer resp.Body.Close()

	io.Copy(stdout, resp.Body)

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("rollback failed with status %d", resp.StatusCode)
	}
	return nil
}
