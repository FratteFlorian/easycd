package inventory

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"github.com/flo-mic/eacd/internal/api"
)

// reconcileService ensures a systemd service is in the desired state,
// including its environment drop-in.
func reconcileService(svc api.InventoryService, log io.Writer) error {
	// Handle env drop-in first; if it changed we must restart regardless of current state.
	envChanged, err := reconcileServiceEnv(svc, log)
	if err != nil {
		return fmt.Errorf("reconciling env for %s: %w", svc.Name, err)
	}

	isEnabled, err := serviceIsEnabled(svc.Name)
	if err != nil {
		// Service might not exist yet if a package was just installed — non-fatal.
		fmt.Fprintf(log, "[eacd] WARNING: cannot check service %s: %v\n", svc.Name, err)
		return nil
	}

	if svc.Enabled && !isEnabled {
		fmt.Fprintf(log, "[eacd] Enabling service: %s\n", svc.Name)
		if err := runCmd(log, "systemctl", "enable", svc.Name); err != nil {
			return err
		}
	} else if !svc.Enabled && isEnabled {
		fmt.Fprintf(log, "[eacd] Disabling service: %s\n", svc.Name)
		if err := runCmd(log, "systemctl", "disable", svc.Name); err != nil {
			return err
		}
	}

	switch svc.State {
	case "started":
		isRunning, _ := serviceIsActive(svc.Name)
		if !isRunning {
			fmt.Fprintf(log, "[eacd] Starting service: %s\n", svc.Name)
			return runCmd(log, "systemctl", "start", svc.Name)
		}
		if envChanged {
			fmt.Fprintf(log, "[eacd] Restarting service (env changed): %s\n", svc.Name)
			return runCmd(log, "systemctl", "restart", svc.Name)
		}
	case "stopped":
		isRunning, _ := serviceIsActive(svc.Name)
		if isRunning {
			fmt.Fprintf(log, "[eacd] Stopping service: %s\n", svc.Name)
			return runCmd(log, "systemctl", "stop", svc.Name)
		}
	}

	return nil
}

// reconcileServiceEnv writes or removes the systemd drop-in for env vars.
// Returns true if the drop-in was created, updated, or deleted.
func reconcileServiceEnv(svc api.InventoryService, log io.Writer) (bool, error) {
	dropinDir := fmt.Sprintf("/etc/systemd/system/%s.service.d", svc.Name)
	dropinFile := filepath.Join(dropinDir, "eacd-env.conf")

	if len(svc.Env) == 0 {
		if _, err := os.Stat(dropinFile); err == nil {
			fmt.Fprintf(log, "[eacd] Removing env drop-in for service: %s\n", svc.Name)
			if err := os.Remove(dropinFile); err != nil {
				return false, fmt.Errorf("removing drop-in: %w", err)
			}
			if err := runCmd(log, "systemctl", "daemon-reload"); err != nil {
				return false, err
			}
			return true, nil
		}
		return false, nil
	}

	// Build drop-in content with sorted keys for stable comparison.
	keys := make([]string, 0, len(svc.Env))
	for k := range svc.Env {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var b strings.Builder
	b.WriteString("[Service]\n")
	for _, k := range keys {
		v := strings.ReplaceAll(svc.Env[k], `"`, `\"`)
		fmt.Fprintf(&b, "Environment=\"%s=%s\"\n", k, v)
	}
	content := b.String()

	// Skip write if content unchanged.
	if existing, err := os.ReadFile(dropinFile); err == nil && string(existing) == content {
		return false, nil
	}

	fmt.Fprintf(log, "[eacd] Writing env drop-in for service: %s\n", svc.Name)
	if err := os.MkdirAll(dropinDir, 0755); err != nil {
		return false, fmt.Errorf("creating drop-in dir: %w", err)
	}
	if err := os.WriteFile(dropinFile, []byte(content), 0644); err != nil {
		return false, fmt.Errorf("writing drop-in: %w", err)
	}
	if err := runCmd(log, "systemctl", "daemon-reload"); err != nil {
		return false, err
	}
	return true, nil
}

func serviceIsEnabled(name string) (bool, error) {
	err := exec.Command("systemctl", "is-enabled", "--quiet", name).Run()
	if err == nil {
		return true, nil
	}
	if _, ok := err.(*exec.ExitError); ok {
		return false, nil
	}
	return false, err
}

func serviceIsActive(name string) (bool, error) {
	err := exec.Command("systemctl", "is-active", "--quiet", name).Run()
	if err == nil {
		return true, nil
	}
	return false, nil
}
