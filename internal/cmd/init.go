package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/huh"
)

// Init runs the interactive init wizard.
func Init(args []string) error {
	dir := "."
	reinit := false
	for _, a := range args {
		if a == "--reinit" || a == "-r" {
			reinit = true
		} else {
			dir = a
		}
	}

	projectDir, err := filepath.Abs(dir)
	if err != nil {
		return err
	}

	simpleDir := filepath.Join(projectDir, ".simplecd")
	configPath := filepath.Join(simpleDir, "config.yaml")

	if _, err := os.Stat(configPath); err == nil && !reinit {
		fmt.Println("A .simplecd/config.yaml already exists. Run with --reinit to overwrite.")
		return nil
	}

	fmt.Println("Welcome to simplecd init. Let's set up your deployment configuration.")
	fmt.Println()

	// --- Step 0: Proxmox provisioning or existing server? ---
	var createCT bool
	if err := huh.NewForm(huh.NewGroup(
		huh.NewConfirm().
			Title("Create a new LXC container on Proxmox?").
			Description("No = configure for an existing server").
			Value(&createCT),
	)).Run(); err != nil {
		return err
	}

	// Pre-filled values from Proxmox provisioning (if chosen)
	var prefillServerURL, prefillToken string

	if createCT {
		result, err := RunProxmoxWizard(os.Stdout)
		if err != nil {
			return fmt.Errorf("Proxmox provisioning failed: %w", err)
		}
		prefillServerURL = result.ServerURL
		prefillToken = result.Token
		fmt.Println()
		fmt.Println("Container ready. Continuing with project configuration...")
		fmt.Println()
	}

	// --- Step 1: Basic info ---
	var projectName string
	serverURL := prefillServerURL

	fields := []huh.Field{
		huh.NewInput().
			Title("Project name").
			Description("Used to identify deployments on the server.").
			Value(&projectName).
			Validate(func(s string) error {
				if strings.TrimSpace(s) == "" {
					return fmt.Errorf("project name cannot be empty")
				}
				return nil
			}),
	}
	// Only ask for server URL if not pre-filled from Proxmox provisioning
	if prefillServerURL == "" {
		fields = append(fields, huh.NewInput().
			Title("Server URL").
			Description("e.g. https://ct.example.com or http://192.168.1.x:8765").
			Value(&serverURL).
			Validate(func(s string) error {
				if !strings.HasPrefix(s, "http://") && !strings.HasPrefix(s, "https://") {
					return fmt.Errorf("must start with http:// or https://")
				}
				return nil
			}))
	}

	if err := huh.NewForm(huh.NewGroup(fields...)).Run(); err != nil {
		return err
	}

	// --- Step 2: Build step? ---
	var hasBuildStep bool
	var srcDir string

	detected := detectProjectType(projectDir)

	if err := huh.NewForm(
		huh.NewGroup(
			huh.NewConfirm().
				Title("Does your project have a build step?").
				Description("e.g. 'go build', 'npm run build', 'cargo build'").
				Value(&hasBuildStep),
		),
	).Run(); err != nil {
		return err
	}

	if hasBuildStep {
		if err := huh.NewForm(
			huh.NewGroup(
				huh.NewInput().
					Title("Build output directory").
					Description("Relative to project root. e.g. ./dist or ./build").
					Placeholder("./dist").
					Value(&srcDir),
			),
		).Run(); err != nil {
			return err
		}
	}

	if srcDir == "" {
		srcDir = "./"
	}

	// --- Step 2.5: Stack template ---
	templateOptions := []huh.Option[string]{
		huh.NewOption("None", "none"),
		huh.NewOption("Go", "go"),
		huh.NewOption("Rust", "rust"),
		huh.NewOption("Node.js", "nodejs"),
		huh.NewOption("Python", "python"),
		huh.NewOption("Java", "java"),
		huh.NewOption("Laravel (PHP)", "laravel"),
		huh.NewOption("Static site (nginx)", "nginx"),
	}
	templateKey := detectedKeyFor(detected)
	if templateKey == "" {
		templateKey = "none"
	}

	if err := huh.NewForm(huh.NewGroup(
		huh.NewSelect[string]().
			Title("Stack template for inventory.yaml").
			Description("Pre-fills system packages, services, and a mapping guide.").
			Options(templateOptions...).
			Value(&templateKey),
	)).Run(); err != nil {
		return err
	}

	var selectedTemplate *stackTemplate
	var suggestedDest string
	if tmpl, ok := stackTemplates[templateKey]; ok {
		selectedTemplate = &tmpl
		hint := strings.ReplaceAll(tmpl.mappingHint, "<name>", projectName)
		fmt.Println()
		fmt.Println("  ── Mapping guide ──────────────────────────────────────────────")
		for _, line := range strings.Split(hint, "\n") {
			fmt.Println(" " + line)
		}
		fmt.Println("  ───────────────────────────────────────────────────────────────")
		fmt.Println()
		if !hasBuildStep && tmpl.suggestedSrc != "" {
			srcDir = tmpl.suggestedSrc
		}
		suggestedDest = tmpl.suggestedDest + "/" + projectName
	}

	// --- Step 3: Deploy destination ---
	var destDir string
	destPlaceholder := "/usr/local/bin"
	if suggestedDest != "" {
		destPlaceholder = suggestedDest
	}
	if err := huh.NewForm(
		huh.NewGroup(
			huh.NewInput().
				Title("Deploy destination on server").
				Description("Absolute path on the CT. e.g. /usr/local/bin or /var/www/myapp").
				Placeholder(destPlaceholder).
				Value(&destDir).
				Validate(func(s string) error {
					if !strings.HasPrefix(s, "/") {
						return fmt.Errorf("must be an absolute path")
					}
					return nil
				}),
		),
	).Run(); err != nil {
		return err
	}
	if destDir == "" {
		destDir = destPlaceholder
	}

	// Default excludes based on project type
	excludes := defaultExcludes(projectDir)

	// --- Step 4: Systemd? ---
	var hasSystemd bool
	var unitFile, serviceName string
	var enableService, restartService bool

	if err := huh.NewForm(
		huh.NewGroup(
			huh.NewConfirm().
				Title("Does this project use a systemd service?").
				Value(&hasSystemd),
		),
	).Run(); err != nil {
		return err
	}

	if hasSystemd {
		if err := huh.NewForm(
			huh.NewGroup(
				huh.NewInput().
					Title("Service name").
					Description("e.g. my-tool  (the .service suffix is added automatically)").
					Value(&serviceName),
				huh.NewInput().
					Title("Path to systemd unit file").
					Description("Relative to project root. e.g. .simplecd/my-tool.service").
					Placeholder(".simplecd/my-tool.service").
					Value(&unitFile),
				huh.NewConfirm().
					Title("Enable service on boot?").
					Value(&enableService),
				huh.NewConfirm().
					Title("Restart service on deploy?").
					Value(&restartService),
			),
		).Run(); err != nil {
			return err
		}
		_ = serviceName // used in template generation
	}

	// --- Step 5: Hooks ---
	var preAction, postAction string

	preOptions := []huh.Option[string]{
		huh.NewOption("Stop systemd service", "stop_service"),
		huh.NewOption("Kill process by name", "kill_process"),
		huh.NewOption("Custom script", "custom"),
		huh.NewOption("None", "none"),
	}
	postOptions := []huh.Option[string]{
		huh.NewOption("Start systemd service", "start_service"),
		huh.NewOption("Restart systemd service", "restart_service"),
		huh.NewOption("Custom script", "custom"),
		huh.NewOption("None", "none"),
	}

	if err := huh.NewForm(
		huh.NewGroup(
			huh.NewSelect[string]().
				Title("Server action BEFORE deploying files").
				Options(preOptions...).
				Value(&preAction),
			huh.NewSelect[string]().
				Title("Server action AFTER deploying files").
				Options(postOptions...).
				Value(&postAction),
		),
	).Run(); err != nil {
		return err
	}

	// --- Step 6: Local pre-hook? ---
	var hasLocalHook bool
	var localHookPath string

	if err := huh.NewForm(
		huh.NewGroup(
			huh.NewConfirm().
				Title("Do you need a local pre-deploy hook?").
				Description("e.g. run linter, generate docs, build the project").
				Value(&hasLocalHook),
		),
	).Run(); err != nil {
		return err
	}

	if hasLocalHook {
		localHookPath = ".simplecd/local-pre.sh"
	}

	// --- Generate files ---
	if err := os.MkdirAll(simpleDir, 0755); err != nil {
		return fmt.Errorf("creating .simplecd/: %w", err)
	}

	// config.yaml
	cfg := buildConfigYAML(projectName, serverURL, prefillToken, srcDir, destDir, excludes, unitFile, enableService, restartService, preAction, postAction, localHookPath, hasSystemd)
	if err := os.WriteFile(configPath, []byte(cfg), 0644); err != nil {
		return err
	}
	fmt.Println("Created .simplecd/config.yaml")

	// pre-deploy script
	if preAction != "none" && preAction != "" {
		scriptPath := filepath.Join(simpleDir, "stop.sh")
		content := generatePreScript(preAction, serviceName)
		os.WriteFile(scriptPath, []byte(content), 0755)
		fmt.Println("Created .simplecd/stop.sh")
	}

	// post-deploy script
	if postAction != "none" && postAction != "" {
		scriptPath := filepath.Join(simpleDir, "start.sh")
		content := generatePostScript(postAction, serviceName)
		os.WriteFile(scriptPath, []byte(content), 0755)
		fmt.Println("Created .simplecd/start.sh")
	}

	// local pre-hook
	if localHookPath != "" {
		scriptPath := filepath.Join(projectDir, localHookPath)
		content := "#!/bin/sh\n# Local pre-deploy hook\n# Add your build/lint commands here\nset -e\n\n"
		os.WriteFile(scriptPath, []byte(content), 0755)
		fmt.Println("Created .simplecd/local-pre.sh")
	}

	// inventory.yaml from stack template
	if selectedTemplate != nil {
		invPath := filepath.Join(simpleDir, "inventory.yaml")
		if err := os.WriteFile(invPath, []byte(selectedTemplate.inventoryYAML), 0644); err == nil {
			fmt.Println("Created .simplecd/inventory.yaml")
		}
	}

	// Ensure .simplecd/ is excluded from git
	if err := ensureGitignore(projectDir); err != nil {
		fmt.Printf("warning: could not update .gitignore: %v\n", err)
	} else {
		fmt.Println("Updated .gitignore (.simplecd/ excluded)")
	}

	fmt.Println()
	fmt.Println("Done! Next steps:")
	step := 1
	if prefillToken == "" {
		fmt.Printf("  %d. Set your auth token: export SIMPLECD_TOKEN=<your-token>\n", step)
		step++
	} else {
		fmt.Printf("  %d. Token is in .simplecd/config.yaml (move to SIMPLECD_TOKEN env var for better security)\n", step)
		step++
	}
	fmt.Printf("  %d. Run: simplecd deploy\n", step)
	step++
	if selectedTemplate == nil {
		fmt.Printf("  %d. Optionally create .simplecd/inventory.yaml to manage system packages\n", step)
	} else {
		fmt.Printf("  %d. Review .simplecd/inventory.yaml and adjust packages/services as needed\n", step)
	}

	return nil
}

func buildConfigYAML(name, server, token, src, dest string, excludes []string, unitFile string, enableService, restartService bool, preAction, postAction, localHook string, hasSystemd bool) string {
	var sb strings.Builder

	sb.WriteString("name: " + name + "\n")
	sb.WriteString("server: " + server + "\n")
	if token != "" {
		// Token was auto-generated by Proxmox provisioning — write it with a clear comment
		sb.WriteString("# Auto-generated token from Proxmox provisioning.\n")
		sb.WriteString("# Move to SIMPLECD_TOKEN env var for better security.\n")
		sb.WriteString("token: " + token + "\n")
	} else {
		sb.WriteString("# token: your-token  # Use SIMPLECD_TOKEN env var instead\n")
	}
	sb.WriteString("\n")

	sb.WriteString("deploy:\n")
	sb.WriteString("  mappings:\n")
	sb.WriteString("    - src: " + src + "\n")
	sb.WriteString("      dest: " + dest + "\n")
	sb.WriteString("      mode: \"0644\"\n")
	sb.WriteString("      dir_mode: \"0755\"\n")
	if len(excludes) > 0 {
		sb.WriteString("      exclude:\n")
		for _, e := range excludes {
			sb.WriteString("        - \"" + e + "\"\n")
		}
	}

	if hasSystemd && unitFile != "" {
		sb.WriteString("\n  systemd:\n")
		sb.WriteString("    unit: " + unitFile + "\n")
		sb.WriteString(fmt.Sprintf("    enable: %v\n", enableService))
		sb.WriteString(fmt.Sprintf("    restart: %v\n", restartService))
	}

	sb.WriteString("\nhooks:\n")
	if localHook != "" {
		sb.WriteString("  local_pre: " + localHook + "\n")
	}
	if preAction != "none" && preAction != "" {
		sb.WriteString("  server_pre: .simplecd/stop.sh\n")
	}
	if postAction != "none" && postAction != "" {
		sb.WriteString("  server_post: .simplecd/start.sh\n")
	}
	if localHook == "" && (preAction == "none" || preAction == "") && (postAction == "none" || postAction == "") {
		sb.WriteString("  # local_pre: .simplecd/local-pre.sh\n")
		sb.WriteString("  # server_pre: .simplecd/stop.sh\n")
		sb.WriteString("  # server_post: .simplecd/start.sh\n")
	}

	return sb.String()
}

func generatePreScript(action, serviceName string) string {
	switch action {
	case "stop_service":
		svc := serviceName
		if svc == "" {
			svc = "my-service"
		}
		return fmt.Sprintf("#!/bin/sh\nset -e\nsystemctl stop %s || true\n", svc+".service")
	case "kill_process":
		return "#!/bin/sh\n# Kill process by name\n# pkill -f my-process || true\n"
	default:
		return "#!/bin/sh\nset -e\n# Custom pre-deploy script\n"
	}
}

func generatePostScript(action, serviceName string) string {
	switch action {
	case "start_service":
		svc := serviceName
		if svc == "" {
			svc = "my-service"
		}
		return fmt.Sprintf("#!/bin/sh\nset -e\nsystemctl start %s\n", svc+".service")
	case "restart_service":
		svc := serviceName
		if svc == "" {
			svc = "my-service"
		}
		return fmt.Sprintf("#!/bin/sh\nset -e\nsystemctl restart %s\n", svc+".service")
	default:
		return "#!/bin/sh\nset -e\n# Custom post-deploy script\n"
	}
}

func detectProjectType(dir string) string {
	checks := map[string]string{
		"composer.json": "PHP/Laravel",
		"package.json":  "Node.js",
		"go.mod":        "Go",
		"Cargo.toml":    "Rust",
		"requirements.txt": "Python",
		"Gemfile":       "Ruby",
	}
	for file, ptype := range checks {
		if _, err := os.Stat(filepath.Join(dir, file)); err == nil {
			return ptype
		}
	}
	return ""
}

// ensureGitignore adds ".simplecd/" to the project's .gitignore if not already present.
func ensureGitignore(projectDir string) error {
	const entry = ".simplecd/"
	gitignorePath := filepath.Join(projectDir, ".gitignore")

	var existing string
	if data, err := os.ReadFile(gitignorePath); err == nil {
		existing = string(data)
		for _, line := range strings.Split(existing, "\n") {
			if strings.TrimSpace(line) == entry {
				return nil // already present
			}
		}
	}

	var content string
	if existing != "" && !strings.HasSuffix(existing, "\n") {
		content = existing + "\n" + entry + "\n"
	} else {
		content = existing + entry + "\n"
	}
	return os.WriteFile(gitignorePath, []byte(content), 0644)
}

func defaultExcludes(dir string) []string {
	ptype := detectProjectType(dir)
	switch ptype {
	case "PHP/Laravel":
		return []string{"vendor/", "node_modules/", ".env", ".git/", "storage/logs/", "*.log"}
	case "Node.js":
		return []string{"node_modules/", ".env", ".git/", "*.log"}
	case "Go":
		return []string{".git/", "*.log"}
	case "Rust":
		return []string{"target/", ".git/", "*.log"}
	case "Python":
		return []string{"__pycache__/", ".venv/", "venv/", ".git/", "*.pyc", "*.log"}
	case "Ruby":
		return []string{".bundle/", ".git/", "log/", "tmp/"}
	default:
		return []string{".git/", "*.log"}
	}
}
