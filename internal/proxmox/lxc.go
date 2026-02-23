package proxmox

import (
	"context"
	"fmt"
	"net/url"
	"strings"
	"time"
)

// ListStorages returns storage backends on the given node that support the given content type.
// Pass content="" to get all storages.
func (c *Client) ListStorages(node, content string) ([]StorageInfo, error) {
	path := fmt.Sprintf("/nodes/%s/storage", node)
	if content != "" {
		path += "?content=" + content
	}
	var result []StorageInfo
	if err := c.get(path, &result); err != nil {
		return nil, err
	}
	return result, nil
}

// ListTemplates returns available LXC OS templates across all vztmpl-capable storages.
func (c *Client) ListTemplates(node string) ([]Template, error) {
	templateStorages, err := c.ListStorages(node, "vztmpl")
	if err != nil {
		return nil, err
	}
	var all []Template
	for _, s := range templateStorages {
		path := fmt.Sprintf("/nodes/%s/storage/%s/content?content=vztmpl", node, s.Storage)
		var items []Template
		if err := c.get(path, &items); err != nil {
			continue // skip inaccessible storages
		}
		all = append(all, items...)
	}
	return all, nil
}

// NextVMID suggests the next available VMID by querying the cluster.
func (c *Client) NextVMID() (int, error) {
	var id int
	if err := c.get("/cluster/nextid", &id); err != nil {
		return 0, err
	}
	return id, nil
}

// CreateLXC creates a new LXC container and returns the task UPID.
func (c *Client) CreateLXC(cfg *LXCCreateConfig) (string, error) {
	params := url.Values{}
	params.Set("vmid", fmt.Sprintf("%d", cfg.VMID))
	params.Set("hostname", cfg.Hostname)
	params.Set("ostemplate", cfg.Template)
	params.Set("storage", cfg.Storage)
	params.Set("cores", fmt.Sprintf("%d", cfg.Cores))
	params.Set("memory", fmt.Sprintf("%d", cfg.Memory))
	params.Set("swap", fmt.Sprintf("%d", cfg.Swap))
	params.Set("rootfs", fmt.Sprintf("%s:%d", cfg.Storage, cfg.DiskGB))
	params.Set("net0", cfg.Net0)
	params.Set("unprivileged", "1")
	if cfg.Password != "" {
		params.Set("password", cfg.Password)
	}
	if cfg.SSHPublicKeys != "" {
		params.Set("ssh-public-keys", cfg.SSHPublicKeys)
	}
	params.Set("features", "nesting=1")

	upid, err := c.post(fmt.Sprintf("/nodes/%s/lxc", cfg.Node), params)
	if err != nil {
		return "", fmt.Errorf("creating LXC: %w", err)
	}
	return upid, nil
}

// StartLXC starts a container and returns the task UPID.
func (c *Client) StartLXC(node string, vmid int) (string, error) {
	upid, err := c.post(fmt.Sprintf("/nodes/%s/lxc/%d/status/start", node, vmid), url.Values{})
	if err != nil {
		return "", fmt.Errorf("starting LXC %d: %w", vmid, err)
	}
	return upid, nil
}

// WaitForIP polls the container's network interfaces until a non-loopback IPv4
// address appears, or until the context deadline is exceeded.
func (c *Client) WaitForIP(ctx context.Context, node string, vmid int) (string, error) {
	path := fmt.Sprintf("/nodes/%s/lxc/%d/interfaces", node, vmid)

	for {
		select {
		case <-ctx.Done():
			return "", fmt.Errorf("timed out waiting for container IP address")
		case <-time.After(2 * time.Second):
		}

		var ifaces []Interface
		if err := c.get(path, &ifaces); err != nil {
			// Container may not be fully up yet — retry
			continue
		}

		for _, iface := range ifaces {
			if iface.Name == "lo" || iface.Inet == "" {
				continue
			}
			ip := strings.SplitN(iface.Inet, "/", 2)[0]
			if ip != "" && ip != "127.0.0.1" {
				return ip, nil
			}
		}
	}
}

// ProvisionAndWait creates an LXC container, waits for it to be created,
// starts it, and waits until it has an IP address.
// Returns the container's IPv4 address.
func (c *Client) ProvisionAndWait(ctx context.Context, cfg *LXCCreateConfig, progress func(string)) (string, error) {
	progress("Creating LXC container...")
	upid, err := c.CreateLXC(cfg)
	if err != nil {
		return "", err
	}

	progress("Waiting for container to be created...")
	if err := c.WaitForTask(ctx, cfg.Node, upid, 2*time.Second); err != nil {
		return "", fmt.Errorf("container creation failed: %w", err)
	}

	progress("Starting container...")
	startUPID, err := c.StartLXC(cfg.Node, cfg.VMID)
	if err != nil {
		return "", err
	}
	if err := c.WaitForTask(ctx, cfg.Node, startUPID, 1*time.Second); err != nil {
		return "", fmt.Errorf("container start failed: %w", err)
	}

	progress("Waiting for container to get an IP address...")
	ipCtx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()

	ip, err := c.WaitForIP(ipCtx, cfg.Node, cfg.VMID)
	if err != nil {
		return "", err
	}

	return ip, nil
}
