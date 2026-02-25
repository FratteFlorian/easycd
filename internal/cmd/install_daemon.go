package cmd

import (
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// InstallDaemon installs eacdd on any Linux host via SSH.
func InstallDaemon(args []string, stdout io.Writer) error {
	fs := flag.NewFlagSet("install-daemon", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	host := fs.String("host", "", "Target host IP or hostname (required)")
	user := fs.String("user", "root", "SSH user")
	keyPath := fs.String("key", "", "Path to SSH private key (default: ~/.ssh/id_ed25519 or id_rsa)")
	if err := fs.Parse(args); err != nil {
		return err
	}

	if *host == "" {
		return fmt.Errorf("--host is required\nUsage: eacd install-daemon --host <ip> [--user <user>] [--key <path>]")
	}

	// Resolve SSH key.
	resolvedKey := *keyPath
	if resolvedKey == "" {
		resolvedKey = findSSHKey()
		if resolvedKey == "" {
			return fmt.Errorf("no SSH key found; use --key to specify one (tried ~/.ssh/id_ed25519 and ~/.ssh/id_rsa)")
		}
		fmt.Fprintf(stdout, "[eacd] Using SSH key: %s\n", resolvedKey)
	}

	token, err := generateToken()
	if err != nil {
		return fmt.Errorf("generating token: %w", err)
	}

	fmt.Fprintf(stdout, "[eacd] Connecting to %s@%s...\n", *user, *host)
	if err := waitForSSH(*host, resolvedKey, 30); err != nil {
		return fmt.Errorf("SSH not available on %s: %w", *host, err)
	}

	if err := bootstrapHost(*host, *user, resolvedKey, token, stdout); err != nil {
		return err
	}

	// Try to update .eacd/config.yaml in the current directory.
	serverURL := fmt.Sprintf("http://%s:8765", *host)
	if err := updateClientConfig(serverURL, token, stdout); err != nil {
		fmt.Fprintf(stdout, "[eacd] Could not update .eacd/config.yaml: %v\n", err)
		fmt.Fprintf(stdout, "[eacd] Add manually:\n")
		fmt.Fprintf(stdout, "  server: %s\n", serverURL)
		fmt.Fprintf(stdout, "  token: %s\n", token)
	}

	fmt.Fprintf(stdout, "\n[eacd] Done! eacdd is running on %s\n", *host)
	fmt.Fprintf(stdout, "[eacd] Tip: export EACD_TOKEN=%s\n", token)
	return nil
}

// bootstrapHost is a user-parameterized variant of bootstrapContainer that works
// on any Linux host (not just Proxmox root@<ip>).
func bootstrapHost(ip, user, keyPath, token string, stdout io.Writer) error {
	binaryPath := findEacddBinary()
	if binaryPath == "" {
		return fmt.Errorf("eacdd binary not found — run 'make build-server' or install eacd first")
	}

	serviceFile := findServiceFile()

	sshArgs := []string{
		"-i", keyPath,
		"-o", "StrictHostKeyChecking=no",
		"-o", "UserKnownHostsFile=/dev/null",
		"-o", "LogLevel=ERROR",
		"-o", "PasswordAuthentication=no",
	}
	target := fmt.Sprintf("%s@%s", user, ip)

	fmt.Fprintf(stdout, "[eacd] Copying eacdd binary...\n")
	if err := scpFile(binaryPath, target+":/usr/local/bin/eacdd", sshArgs); err != nil {
		return fmt.Errorf("scp eacdd: %w", err)
	}

	if serviceFile != "" {
		fmt.Fprintf(stdout, "[eacd] Copying systemd unit...\n")
		if err := scpFile(serviceFile, target+":/etc/systemd/system/eacdd.service", sshArgs); err != nil {
			return fmt.Errorf("scp service file: %w", err)
		}
	}

	serverYAML := fmt.Sprintf("listen: :8765\ntoken: %s\nlog_dir: /var/log/eacd\n", token)
	setupScript := fmt.Sprintf(`set -e
chmod +x /usr/local/bin/eacdd
mkdir -p /etc/eacd /var/log/eacd /var/lib/eacd/.global
cat > /etc/eacd/server.yaml << 'YAMLEOF'
%sYAMLEOF
systemctl daemon-reload
systemctl enable --now eacdd
echo "eacdd installed and running"
`, serverYAML)

	if serviceFile == "" {
		inlineUnit := `[Unit]
Description=eacd deployment daemon
After=network.target

[Service]
Type=simple
ExecStart=/usr/local/bin/eacdd --config /etc/eacd/server.yaml
Restart=on-failure
RestartSec=5

[Install]
WantedBy=multi-user.target
`
		setupScript = fmt.Sprintf(`set -e
chmod +x /usr/local/bin/eacdd
mkdir -p /etc/eacd /var/log/eacd /var/lib/eacd/.global
cat > /etc/systemd/system/eacdd.service << 'SVCEOF'
%sSVCEOF
cat > /etc/eacd/server.yaml << 'YAMLEOF'
%sYAMLEOF
systemctl daemon-reload
systemctl enable --now eacdd
echo "eacdd installed and running"
`, inlineUnit, serverYAML)
	}

	fmt.Fprintf(stdout, "[eacd] Running setup script...\n")
	if err := sshRun(target, setupScript, sshArgs, stdout); err != nil {
		return fmt.Errorf("setup failed: %w", err)
	}

	return nil
}

// findSSHKey returns the first default SSH private key found in ~/.ssh/.
func findSSHKey() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	for _, name := range []string{"id_ed25519", "id_rsa"} {
		p := filepath.Join(home, ".ssh", name)
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	return ""
}

// updateClientConfig sets the server and token fields in .eacd/config.yaml,
// preserving all other content and comments via line-by-line replacement.
func updateClientConfig(serverURL, token string, stdout io.Writer) error {
	const cfgPath = ".eacd/config.yaml"
	data, err := os.ReadFile(cfgPath)
	if err != nil {
		return err
	}

	lines := strings.Split(string(data), "\n")
	serverSet, tokenSet := false, false
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "server:") {
			lines[i] = fmt.Sprintf("server: %s", serverURL)
			serverSet = true
		} else if strings.HasPrefix(trimmed, "token:") {
			lines[i] = fmt.Sprintf("token: %s", token)
			tokenSet = true
		}
	}
	if !serverSet {
		lines = append(lines, fmt.Sprintf("server: %s", serverURL))
	}
	if !tokenSet {
		lines = append(lines, fmt.Sprintf("token: %s", token))
	}

	if err := os.WriteFile(cfgPath, []byte(strings.Join(lines, "\n")), 0644); err != nil {
		return err
	}
	fmt.Fprintf(stdout, "[eacd] Updated .eacd/config.yaml (server + token)\n")
	return nil
}
