package inventory

import (
	"fmt"
	"io"
	"os/exec"
)

// reconcileService ensures a systemd service is in the desired state.
func reconcileService(name string, enabled bool, state string, log io.Writer) error {
	isEnabled, err := serviceIsEnabled(name)
	if err != nil {
		// Service might not exist yet if a package was just installed — non-fatal
		fmt.Fprintf(log, "[eacd] WARNING: cannot check service %s: %v\n", name, err)
		return nil
	}

	if enabled && !isEnabled {
		fmt.Fprintf(log, "[eacd] Enabling service: %s\n", name)
		if err := runCmd(log, "systemctl", "enable", name); err != nil {
			return err
		}
	} else if !enabled && isEnabled {
		fmt.Fprintf(log, "[eacd] Disabling service: %s\n", name)
		if err := runCmd(log, "systemctl", "disable", name); err != nil {
			return err
		}
	}

	switch state {
	case "started":
		isRunning, _ := serviceIsActive(name)
		if !isRunning {
			fmt.Fprintf(log, "[eacd] Starting service: %s\n", name)
			return runCmd(log, "systemctl", "start", name)
		}
	case "stopped":
		isRunning, _ := serviceIsActive(name)
		if isRunning {
			fmt.Fprintf(log, "[eacd] Stopping service: %s\n", name)
			return runCmd(log, "systemctl", "stop", name)
		}
	}

	return nil
}

func serviceIsEnabled(name string) (bool, error) {
	err := exec.Command("systemctl", "is-enabled", "--quiet", name).Run()
	if err == nil {
		return true, nil
	}
	if exitErr, ok := err.(*exec.ExitError); ok {
		_ = exitErr
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
