package cmd

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"

	"github.com/charmbracelet/huh"
	"github.com/flo-mic/simplecd/internal/config"
	"github.com/flo-mic/simplecd/internal/proxmox"
)

// ProxmoxResult holds the outcomes of a successful CT provisioning.
type ProxmoxResult struct {
	ServerURL string // e.g. http://192.168.1.100:8765
	Token     string // randomly generated auth token
}

// RunProxmoxWizard runs the full Proxmox CT provisioning flow.
// Returns the server URL and auth token to pre-fill into the project config.
func RunProxmoxWizard(stdout io.Writer) (*ProxmoxResult, error) {
	// Load or prompt for Proxmox credentials
	pxCfg, err := loadOrPromptProxmoxConfig()
	if err != nil {
		return nil, err
	}

	client := proxmox.NewClient(pxCfg.Host, pxCfg.Port, pxCfg.Token, pxCfg.Insecure)

	// Test connectivity
	fmt.Fprintln(stdout, "Connecting to Proxmox...")
	if err := client.Ping(); err != nil {
		return nil, fmt.Errorf("cannot connect to Proxmox at %s:%d: %w", pxCfg.Host, pxCfg.Port, err)
	}
	fmt.Fprintln(stdout, "Connected.")

	// Fetch storages that support container rootfs
	storages, err := client.ListStorages(pxCfg.Node, "rootdir")
	if err != nil {
		return nil, fmt.Errorf("listing storages: %w", err)
	}
	storageOpts := buildStorageOptions(storages)

	// Suggest next VMID
	suggestedVMID := 100
	if id, err := client.NextVMID(); err == nil {
		suggestedVMID = id
	}

	// --- CT parameter wizard ---
	var (
		vmidStr   = strconv.Itoa(suggestedVMID)
		hostname  string
		storage   = firstOrEmpty(storageOpts)
		template  string
		coresStr  = "1"
		memoryStr = "512"
		swapStr   = "0"
		diskStr   = "8"
		usedhcp   = true
		staticIP  string
		bridge    = "vmbr0"
		rootPass  string
	)

	if err := huh.NewForm(
		huh.NewGroup(
			huh.NewInput().
				Title("Container ID (VMID)").
				Value(&vmidStr).
				Validate(validateInt),
			huh.NewInput().
				Title("Container hostname").
				Placeholder("my-service").
				Value(&hostname).
				Validate(func(s string) error {
					if strings.TrimSpace(s) == "" {
						return fmt.Errorf("hostname cannot be empty")
					}
					return nil
				}),
			storageField(storageOpts, &storage),
		),
		huh.NewGroup(
			huh.NewInput().
				Title("CPU cores").
				Value(&coresStr).
				Validate(validateInt),
			huh.NewInput().
				Title("Memory (MB)").
				Value(&memoryStr).
				Validate(validateInt),
			huh.NewInput().
				Title("Swap (MB, 0 = disabled)").
				Value(&swapStr).
				Validate(validateInt),
			huh.NewInput().
				Title("Disk size (GB)").
				Value(&diskStr).
				Validate(validateInt),
		),
		huh.NewGroup(
			huh.NewInput().
				Title("Network bridge").
				Description("Name of the Proxmox bridge (e.g. vmbr0). Check Node > Network.").
				Value(&bridge).
				Validate(func(s string) error {
					if strings.TrimSpace(s) == "" {
						return fmt.Errorf("bridge cannot be empty")
					}
					return nil
				}),
			huh.NewConfirm().
				Title("Use DHCP for networking?").
				Description("No = enter a static IP address").
				Value(&usedhcp),
		),
	).Run(); err != nil {
		return nil, err
	}

	// Static IP prompt (only if DHCP = no)
	if !usedhcp {
		if err := huh.NewForm(huh.NewGroup(
			huh.NewInput().
				Title("Static IP (CIDR notation)").
				Description("e.g. 192.168.1.100/24,gw=192.168.1.1").
				Value(&staticIP).
				Validate(func(s string) error {
					if !strings.Contains(s, "/") {
						return fmt.Errorf("must be in CIDR format, e.g. 192.168.1.100/24,gw=192.168.1.1")
					}
					return nil
				}),
		)).Run(); err != nil {
			return nil, err
		}
	}

	// Fetch templates from all vztmpl-capable storages
	fmt.Fprintln(stdout, "Fetching available OS templates...")
	templates, err := client.ListTemplates(pxCfg.Node)
	if err != nil || len(templates) == 0 {
		// Fall back to text input if template listing fails
		if err := huh.NewForm(huh.NewGroup(
			huh.NewInput().
				Title("OS Template").
				Description("e.g. local:vztmpl/debian-12-standard_12.7-1_amd64.tar.zst").
				Value(&template),
		)).Run(); err != nil {
			return nil, err
		}
	} else {
		templateOpts := buildTemplateOptions(templates)
		if err := huh.NewForm(huh.NewGroup(
			huh.NewSelect[string]().
				Title("OS Template").
				Options(templateOpts...).
				Value(&template),
		)).Run(); err != nil {
			return nil, err
		}
	}

	// Root password
	if err := huh.NewForm(huh.NewGroup(
		huh.NewInput().
			Title("Root password for the new container").
			Description("Used for SSH bootstrap — can be changed afterwards.").
			EchoMode(huh.EchoModePassword).
			Value(&rootPass).
			Validate(func(s string) error {
				if len(s) < 6 {
					return fmt.Errorf("password must be at least 6 characters")
				}
				return nil
			}),
	)).Run(); err != nil {
		return nil, err
	}

	// Build net0 string (firewall=1 matches Proxmox UI default and avoids bridge permission issues)
	net0 := fmt.Sprintf("name=eth0,bridge=%s,firewall=1,", bridge)
	if usedhcp {
		net0 += "ip=dhcp"
	} else {
		net0 += "ip=" + staticIP
	}

	vmid, _ := strconv.Atoi(vmidStr)
	cores, _ := strconv.Atoi(coresStr)
	memory, _ := strconv.Atoi(memoryStr)
	swap, _ := strconv.Atoi(swapStr)
	disk, _ := strconv.Atoi(diskStr)

	// Generate temporary SSH key for bootstrap (avoids password auth issues)
	tmpKey, pubKey, err := generateTempSSHKey()
	if err != nil {
		return nil, fmt.Errorf("generating SSH key: %w", err)
	}
	defer os.Remove(tmpKey)
	defer os.Remove(tmpKey + ".pub")

	lxcCfg := &proxmox.LXCCreateConfig{
		VMID:          vmid,
		Node:          pxCfg.Node,
		Hostname:      hostname,
		Template:      template,
		Storage:       storage,
		Cores:         cores,
		Memory:        memory,
		Swap:          swap,
		DiskGB:        disk,
		Net0:          net0,
		Password:      rootPass,
		SSHPublicKeys: pubKey,
	}

	// Provision the CT
	fmt.Fprintln(stdout, "")
	ip, err := client.ProvisionAndWait(context.Background(), lxcCfg, func(msg string) {
		fmt.Fprintf(stdout, "  %s\n", msg)
	})
	if err != nil {
		return nil, fmt.Errorf("provisioning container: %w", err)
	}
	fmt.Fprintf(stdout, "  Container IP: %s\n", ip)

	// Generate auth token
	token, err := generateToken()
	if err != nil {
		return nil, fmt.Errorf("generating token: %w", err)
	}

	// SSH bootstrap using the temporary key
	fmt.Fprintln(stdout, "  Installing simplecdd on the container...")
	if err := bootstrapContainer(ip, tmpKey, token, stdout); err != nil {
		return nil, fmt.Errorf("bootstrap failed: %w\n\nYou can retry manually:\n  make install-server CT_HOST=%s", err, ip)
	}

	fmt.Fprintf(stdout, "  Container ready at %s\n", ip)

	return &ProxmoxResult{
		ServerURL: fmt.Sprintf("http://%s:8765", ip),
		Token:     token,
	}, nil
}

// loadOrPromptProxmoxConfig loads the global Proxmox config or runs a TUI to create it.
func loadOrPromptProxmoxConfig() (*config.ProxmoxConfig, error) {
	existing, err := config.LoadProxmoxConfig()
	if err != nil {
		return nil, err
	}
	if existing != nil && existing.Host != "" && existing.Token != "" {
		var useExisting bool
		if err := huh.NewForm(huh.NewGroup(
			huh.NewConfirm().
				Title(fmt.Sprintf("Use existing Proxmox config? (%s:%d)", existing.Host, existing.Port)).
				Value(&useExisting),
		)).Run(); err != nil {
			return nil, err
		}
		if useExisting {
			return existing, nil
		}
	}

	// Prompt for credentials
	cfg := &config.ProxmoxConfig{Port: 8006, Node: "pve", Insecure: true}
	tokenStr := ""
	portStr := strconv.Itoa(cfg.Port)

	if err := huh.NewForm(
		huh.NewGroup(
			huh.NewInput().
				Title("Proxmox host (IP or hostname)").
				Placeholder("192.168.1.x").
				Value(&cfg.Host).
				Validate(func(s string) error {
					if strings.TrimSpace(s) == "" {
						return fmt.Errorf("host cannot be empty")
					}
					return nil
				}),
			huh.NewInput().
				Title("Proxmox port").
				Value(&portStr).
				Validate(validateInt),
			huh.NewInput().
				Title("Proxmox node name").
				Value(&cfg.Node),
			huh.NewInput().
				Title("API Token").
				Description("Format: user@realm!tokenid=secret  (or set PROXMOX_TOKEN env var)").
				EchoMode(huh.EchoModePassword).
				Value(&tokenStr),
			huh.NewConfirm().
				Title("Skip TLS certificate verification?").
				Description("Recommended for homelab setups with self-signed certs.").
				Value(&cfg.Insecure),
		),
	).Run(); err != nil {
		return nil, err
	}

	cfg.Token = tokenStr
	cfg.Port, _ = strconv.Atoi(portStr)

	var save bool
	if err := huh.NewForm(huh.NewGroup(
		huh.NewConfirm().
			Title("Save Proxmox config to ~/.config/simplecd/proxmox.yaml?").
			Value(&save),
	)).Run(); err != nil {
		return nil, err
	}
	if save {
		if err := config.SaveProxmoxConfig(cfg); err != nil {
			fmt.Fprintf(os.Stderr, "warning: could not save proxmox config: %v\n", err)
		}
	}

	return cfg, nil
}

// bootstrapContainer copies simplecdd to the CT and sets up the service using key-based SSH.
func bootstrapContainer(ip, keyPath, token string, stdout io.Writer) error {
	binaryPath := findSimplecddBinary()
	if binaryPath == "" {
		return fmt.Errorf("dist/simplecdd not found — run 'make build-server' first")
	}

	serviceFile := findServiceFile()

	sshArgs := []string{
		"-i", keyPath,
		"-o", "StrictHostKeyChecking=no",
		"-o", "UserKnownHostsFile=/dev/null",
		"-o", "LogLevel=ERROR",
		"-o", "PasswordAuthentication=no",
	}

	target := fmt.Sprintf("root@%s", ip)

	// Wait a moment for SSH to come up and authorized_keys to be written
	fmt.Fprintln(stdout, "  Waiting for SSH to become available...")
	if err := waitForSSH(ip, keyPath, 60); err != nil {
		return fmt.Errorf("SSH not available: %w", err)
	}

	// Copy simplecdd binary
	fmt.Fprintln(stdout, "  Copying simplecdd binary...")
	if err := scpFile(binaryPath, target+":/usr/local/bin/simplecdd", sshArgs); err != nil {
		return fmt.Errorf("scp simplecdd: %w", err)
	}

	// Copy service file if available
	if serviceFile != "" {
		fmt.Fprintln(stdout, "  Copying systemd unit...")
		if err := scpFile(serviceFile, target+":/etc/systemd/system/simplecdd.service", sshArgs); err != nil {
			return fmt.Errorf("scp service file: %w", err)
		}
	}

	serverYAML := fmt.Sprintf("listen: :8765\ntoken: %s\nlog_dir: /var/log/simplecd\n", token)
	setupScript := fmt.Sprintf(`set -e
chmod +x /usr/local/bin/simplecdd
mkdir -p /etc/simplecd /var/log/simplecd /var/lib/simplecd/.global
cat > /etc/simplecd/server.yaml << 'YAMLEOF'
%sYAMLEOF
systemctl daemon-reload
systemctl enable --now simplecdd
echo "simplecdd installed and running"
`, serverYAML)

	if serviceFile == "" {
		inlineUnit := `[Unit]
Description=simplecd deployment daemon
After=network.target

[Service]
Type=simple
ExecStart=/usr/local/bin/simplecdd --config /etc/simplecd/server.yaml
Restart=on-failure
RestartSec=5

[Install]
WantedBy=multi-user.target
`
		setupScript = fmt.Sprintf(`set -e
chmod +x /usr/local/bin/simplecdd
mkdir -p /etc/simplecd /var/log/simplecd /var/lib/simplecd/.global
cat > /etc/systemd/system/simplecdd.service << 'SVCEOF'
%sSVCEOF
cat > /etc/simplecd/server.yaml << 'YAMLEOF'
%sYAMLEOF
systemctl daemon-reload
systemctl enable --now simplecdd
echo "simplecdd installed and running"
`, inlineUnit, serverYAML)
	}

	fmt.Fprintln(stdout, "  Running setup script...")
	if err := sshRun(target, setupScript, sshArgs, stdout); err != nil {
		return fmt.Errorf("ssh setup: %w", err)
	}

	return nil
}

// waitForSSH polls until SSH accepts the key or the timeout (seconds) is reached.
func waitForSSH(ip, keyPath string, timeoutSec int) error {
	args := []string{
		"-i", keyPath,
		"-o", "StrictHostKeyChecking=no",
		"-o", "UserKnownHostsFile=/dev/null",
		"-o", "LogLevel=ERROR",
		"-o", "PasswordAuthentication=no",
		"-o", fmt.Sprintf("ConnectTimeout=%d", 3),
		fmt.Sprintf("root@%s", ip),
		"true",
	}
	deadline := timeoutSec / 3
	for i := 0; i < deadline; i++ {
		cmd := exec.Command("ssh", args...)
		if err := cmd.Run(); err == nil {
			return nil
		}
		// brief pause already covered by ConnectTimeout
	}
	return fmt.Errorf("timed out after %ds", timeoutSec)
}

func scpFile(src, dst string, sshArgs []string) error {
	args := append(sshArgs, src, dst)
	cmd := exec.Command("scp", args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func sshRun(target, script string, sshArgs []string, stdout io.Writer) error {
	args := append(sshArgs, target, script)
	cmd := exec.Command("ssh", args...)
	cmd.Stdout = stdout
	cmd.Stderr = stdout
	return cmd.Run()
}

// generateTempSSHKey creates a temporary ed25519 key pair and returns (privateKeyPath, publicKeyContent, error).
func generateTempSSHKey() (string, string, error) {
	keyPath := filepath.Join(os.TempDir(), fmt.Sprintf("simplecd_bootstrap_%d", os.Getpid()))
	cmd := exec.Command("ssh-keygen", "-t", "ed25519", "-f", keyPath, "-N", "", "-q")
	if out, err := cmd.CombinedOutput(); err != nil {
		return "", "", fmt.Errorf("ssh-keygen: %w: %s", err, out)
	}
	pubKey, err := os.ReadFile(keyPath + ".pub")
	if err != nil {
		return "", "", fmt.Errorf("reading public key: %w", err)
	}
	return keyPath, strings.TrimSpace(string(pubKey)), nil
}

func generateToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

func findSimplecddBinary() string {
	// Try common locations relative to executable
	exe, _ := os.Executable()
	candidates := []string{
		"dist/simplecdd",
		filepath.Join(filepath.Dir(exe), "dist/simplecdd"),
		filepath.Join(filepath.Dir(exe), "simplecdd"),
	}
	if runtime.GOOS == "windows" {
		for i, c := range candidates {
			candidates[i] = c + ".exe"
		}
	}
	for _, c := range candidates {
		if _, err := os.Stat(c); err == nil {
			return c
		}
	}
	return ""
}

func findServiceFile() string {
	candidates := []string{
		"install/simplecdd.service",
		"simplecdd.service",
	}
	for _, c := range candidates {
		if _, err := os.Stat(c); err == nil {
			return c
		}
	}
	return ""
}

func buildStorageOptions(storages []proxmox.StorageInfo) []huh.Option[string] {
	var opts []huh.Option[string]
	for _, s := range storages {
		label := s.Storage
		if s.Type != "" {
			label = fmt.Sprintf("%s (%s)", s.Storage, s.Type)
		}
		opts = append(opts, huh.NewOption(label, s.Storage))
	}
	return opts
}

// firstOrEmpty returns the value (not the display label) of the first option.
func firstOrEmpty(opts []huh.Option[string]) string {
	if len(opts) > 0 {
		return opts[0].Value
	}
	return ""
}

func storageField(opts []huh.Option[string], value *string) huh.Field {
	if len(opts) > 0 {
		return huh.NewSelect[string]().
			Title("Storage backend").
			Options(opts...).
			Value(value)
	}
	return huh.NewInput().
		Title("Storage backend").
		Description("Could not fetch storages from API. Enter manually (e.g. local-lvm, local).").
		Value(value).
		Validate(func(s string) error {
			if strings.TrimSpace(s) == "" {
				return fmt.Errorf("storage cannot be empty")
			}
			return nil
		})
}

func buildTemplateOptions(templates []proxmox.Template) []huh.Option[string] {
	var opts []huh.Option[string]
	for _, t := range templates {
		opts = append(opts, huh.NewOption(t.VolID, t.VolID))
	}
	return opts
}

func validateInt(s string) error {
	if _, err := strconv.Atoi(s); err != nil {
		return fmt.Errorf("must be a number")
	}
	return nil
}
