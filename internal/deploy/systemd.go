package deploy

import (
	"fmt"
	"io"
	"os/exec"
	"path/filepath"
)

// InstallUnit copies a unit file to dest and optionally enables and restarts it.
func InstallUnit(srcPath, unitDest string, enable, restart bool, log io.Writer) error {
	if err := PlaceFile(srcPath, unitDest, "0644", log); err != nil {
		return err
	}

	if err := runSystemctl(log, "daemon-reload"); err != nil {
		return err
	}

	unitName := filepath.Base(unitDest)

	if enable {
		if err := runSystemctl(log, "enable", unitName); err != nil {
			return err
		}
	}
	if restart {
		if err := runSystemctl(log, "restart", unitName); err != nil {
			return err
		}
	}
	return nil
}

func runSystemctl(log io.Writer, args ...string) error {
	fmt.Fprintf(log, "[simplecd] systemctl %v\n", args)
	cmd := exec.Command("systemctl", args...)
	cmd.Stdout = log
	cmd.Stderr = log
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("systemctl %v: %w", args, err)
	}
	return nil
}
