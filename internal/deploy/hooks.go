package deploy

import (
	"fmt"
	"io"
	"os/exec"
)

// RunHook executes a shell command via /bin/sh -c.
// Output is written to log. Returns an error if the command exits non-zero.
func RunHook(cmd string, log io.Writer) error {
	fmt.Fprintf(log, "[eacd] Running hook: %s\n", cmd)
	c := exec.Command("/bin/sh", "-c", cmd)
	c.Stdout = log
	c.Stderr = log
	c.Dir = "/"
	if err := c.Run(); err != nil {
		return fmt.Errorf("hook %q failed: %w", cmd, err)
	}
	return nil
}

// RunLocalHook executes a hook script locally (client-side).
// scriptPath is the path to the script file.
func RunLocalHook(scriptPath string, log io.Writer) error {
	return RunHook(scriptPath, log)
}
