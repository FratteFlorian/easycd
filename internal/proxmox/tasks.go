package proxmox

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// WaitForTask polls a Proxmox task until it completes or the context is cancelled.
// Returns an error if the task exits with a non-OK status.
func (c *Client) WaitForTask(ctx context.Context, node, upid string, poll time.Duration) error {
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(poll):
		}

		status, err := c.taskStatus(node, upid)
		if err != nil {
			return err
		}

		if status.Status == "stopped" {
			if status.ExitStatus != "OK" {
				return fmt.Errorf("task %s failed: %s", upid, status.ExitStatus)
			}
			return nil
		}
	}
}

func (c *Client) taskStatus(node, upid string) (*TaskStatus, error) {
	// UPID contains colons which must be URL-encoded in the path
	escapedUPID := urlEncodeUPID(upid)
	path := fmt.Sprintf("/nodes/%s/tasks/%s/status", node, escapedUPID)

	var status TaskStatus
	if err := c.get(path, &status); err != nil {
		return nil, err
	}
	return &status, nil
}

// urlEncodeUPID encodes colons in the UPID for use in URL paths.
func urlEncodeUPID(upid string) string {
	// Proxmox API expects the UPID percent-encoded in path segments
	var b strings.Builder
	for _, c := range upid {
		if c == ':' {
			b.WriteString("%3A")
		} else {
			b.WriteRune(c)
		}
	}
	return b.String()
}
