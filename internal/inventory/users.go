package inventory

import (
	"fmt"
	"io"
	"os/exec"
	"os/user"
	"strings"

	"github.com/flo-mic/simplecd/internal/api"
)

// ensureUser creates a system user if it doesn't already exist.
// Users are never automatically deleted.
func ensureUser(u api.InventoryUser, log io.Writer) error {
	if _, err := user.Lookup(u.Name); err == nil {
		fmt.Fprintf(log, "[simplecd] User %s already exists, skipping\n", u.Name)
		return nil
	}

	fmt.Fprintf(log, "[simplecd] Creating user: %s\n", u.Name)

	args := []string{"--system"}

	if u.Home != "" {
		args = append(args, "--home-dir", u.Home, "--create-home")
	} else {
		args = append(args, "--no-create-home")
	}

	if u.Shell != "" {
		args = append(args, "--shell", u.Shell)
	} else {
		args = append(args, "--shell", "/usr/sbin/nologin")
	}

	if len(u.Groups) > 0 {
		args = append(args, "--groups", strings.Join(u.Groups, ","))
	}

	args = append(args, u.Name)

	return runCmd(log, "useradd", args...)
}

// userExists checks if a system user exists (exported for testing).
func userExists(name string) bool {
	_, err := exec.Command("id", name).Output()
	return err == nil
}
