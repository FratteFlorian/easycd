package cmd

import (
	"bufio"
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/flo-mic/eacd/internal/api"
	"github.com/flo-mic/eacd/internal/archive"
	"github.com/flo-mic/eacd/internal/config"
	"github.com/flo-mic/eacd/internal/delta"
)

// Deploy runs the deploy subcommand.
func Deploy(args []string, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("deploy", flag.ContinueOnError)
	fs.SetOutput(stderr)
	dir := fs.String("dir", ".", "Project directory (default: current directory)")
	if err := fs.Parse(args); err != nil {
		return err
	}

	projectDir, err := filepath.Abs(*dir)
	if err != nil {
		return fmt.Errorf("resolving project dir: %w", err)
	}

	cfg, err := config.LoadClientConfig(projectDir)
	if err != nil {
		return err
	}

	// Resolve token: env var takes precedence over config file
	token := os.Getenv("EACD_TOKEN")
	if token == "" && cfg.Token != "" {
		fmt.Fprintln(stderr, "warning: token is hardcoded in .eacd/config.yaml — consider using EACD_TOKEN env var instead")
		token = cfg.Token
	}
	if token == "" {
		return fmt.Errorf("no auth token: set EACD_TOKEN or add 'token:' to .eacd/config.yaml")
	}

	// Run local pre-hook
	if cfg.Hooks.LocalPre != "" {
		hookPath := filepath.Join(projectDir, cfg.Hooks.LocalPre)
		fmt.Fprintf(stdout, "[eacd] Running local pre-hook: %s\n", hookPath)
		if err := runLocalScript(hookPath, stdout, stderr); err != nil {
			return fmt.Errorf("local pre-hook failed: %w", err)
		}
	}

	// Collect files from all mappings and compute hashes
	type localFile struct {
		srcPath     string
		dest        string
		mode        string
		archiveName string
	}

	var allFiles []localFile
	for mi, m := range cfg.Deploy.Mappings {
		srcDir := filepath.Join(projectDir, m.Src)
		if err := filepath.Walk(srcDir, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			rel, _ := filepath.Rel(srcDir, path)
			if info.IsDir() {
				if rel != "." && archive.ShouldExclude(rel, true, m.Exclude) {
					return filepath.SkipDir
				}
				return nil
			}
			if archive.ShouldExclude(rel, false, m.Exclude) {
				return nil
			}
			allFiles = append(allFiles, localFile{
				srcPath:     path,
				dest:        filepath.Join(m.Dest, rel),
				mode:        m.Mode,
				archiveName: fmt.Sprintf("files/%d/%s", mi, rel),
			})
			return nil
		}); err != nil {
			return fmt.Errorf("walking %s: %w", srcDir, err)
		}
	}

	// Compute hashes
	checkFiles := make([]api.FileHashEntry, len(allFiles))
	hashes := make(map[string]string, len(allFiles))
	for i, f := range allFiles {
		h, err := delta.HashFile(f.srcPath)
		if err != nil {
			return fmt.Errorf("hashing %s: %w", f.srcPath, err)
		}
		hashes[f.dest] = h
		checkFiles[i] = api.FileHashEntry{Dest: f.dest, Hash: h}
	}

	// POST /check
	checkBody, _ := json.Marshal(api.CheckRequest{Name: cfg.Name, Files: checkFiles})
	checkResp, err := httpPost(cfg.Server+"/check", token, "application/json", checkBody)
	if err != nil {
		return fmt.Errorf("check request: %w", err)
	}
	defer checkResp.Body.Close()
	if checkResp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(checkResp.Body)
		return fmt.Errorf("check failed (%d): %s", checkResp.StatusCode, body)
	}

	var checkResult api.CheckResponse
	if err := json.NewDecoder(checkResp.Body).Decode(&checkResult); err != nil {
		return fmt.Errorf("parsing check response: %w", err)
	}

	needed := make(map[string]bool, len(checkResult.Upload))
	for _, d := range checkResult.Upload {
		needed[d] = true
	}
	fmt.Fprintf(stdout, "[eacd] Files to upload: %d / %d\n", len(needed), len(allFiles))

	// Build manifest + archive
	manifest := api.Manifest{Name: cfg.Name}
	var archiveBuf bytes.Buffer
	tw, gw := archive.NewWriter(&archiveBuf)

	for _, f := range allFiles {
		entry := api.FileEntry{Dest: f.dest, Mode: f.mode, Hash: hashes[f.dest]}
		if needed[f.dest] {
			entry.ArchivePath = f.archiveName
			if err := archive.AddFile(tw, f.srcPath, f.archiveName, 0644); err != nil {
				return fmt.Errorf("adding %s: %w", f.srcPath, err)
			}
		}
		manifest.Files = append(manifest.Files, entry)
	}

	// Server-side hook scripts (always upload if configured)
	if cfg.Hooks.ServerPre != "" || cfg.Hooks.ServerPost != "" {
		manifest.Hooks = &api.HooksEntry{}
	}
	if cfg.Hooks.ServerPre != "" {
		name := "scripts/pre-deploy.sh"
		if err := archive.AddFile(tw, filepath.Join(projectDir, cfg.Hooks.ServerPre), name, 0755); err != nil {
			return fmt.Errorf("adding pre script: %w", err)
		}
		manifest.Hooks.ServerPre = name
	}
	if cfg.Hooks.ServerPost != "" {
		name := "scripts/post-deploy.sh"
		if err := archive.AddFile(tw, filepath.Join(projectDir, cfg.Hooks.ServerPost), name, 0755); err != nil {
			return fmt.Errorf("adding post script: %w", err)
		}
		manifest.Hooks.ServerPost = name
	}

	// Systemd unit
	if cfg.Deploy.Systemd != nil {
		unitPath := filepath.Join(projectDir, cfg.Deploy.Systemd.Unit)
		unitName := filepath.Base(unitPath)
		archiveName := "files/systemd/" + unitName
		if err := archive.AddFile(tw, unitPath, archiveName, 0644); err != nil {
			return fmt.Errorf("adding unit file: %w", err)
		}
		manifest.Systemd = &api.SystemdEntry{
			UnitArchivePath: archiveName,
			UnitDest:        "/etc/systemd/system/" + unitName,
			Enable:          cfg.Deploy.Systemd.Enable,
			Restart:         cfg.Deploy.Systemd.Restart,
		}
	}

	// Inventory
	if inv, err := loadInventory(filepath.Join(projectDir, ".eacd", "inventory.yaml")); err == nil && inv != nil {
		manifest.Inventory = inv
	}

	tw.Close()
	gw.Close()

	// POST /deploy
	manifestJSON, _ := json.Marshal(manifest)
	body, contentType, err := buildMultipart(manifestJSON, archiveBuf.Bytes())
	if err != nil {
		return fmt.Errorf("building request body: %w", err)
	}

	fmt.Fprintf(stdout, "[eacd] Deploying %s → %s\n", cfg.Name, cfg.Server)
	deployResp, err := httpPost(cfg.Server+"/deploy", token, contentType, body)
	if err != nil {
		return fmt.Errorf("deploy request: %w", err)
	}
	defer deployResp.Body.Close()

	if deployResp.StatusCode != http.StatusOK {
		errBody, _ := io.ReadAll(deployResp.Body)
		return fmt.Errorf("deployment failed (%d): %s", deployResp.StatusCode, bytes.TrimSpace(errBody))
	}
	return streamAndCheck(deployResp.Body, stdout, "deployment failed (see output above)")
}

func httpPost(url, token, contentType string, body []byte) (*http.Response, error) {
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", contentType)
	return http.DefaultClient.Do(req)
}

func buildMultipart(manifestJSON, archiveData []byte) ([]byte, string, error) {
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)

	mh := make(textproto.MIMEHeader)
	mh.Set("Content-Disposition", `form-data; name="manifest"`)
	mh.Set("Content-Type", "application/json")
	pw, _ := mw.CreatePart(mh)
	pw.Write(manifestJSON)

	ah := make(textproto.MIMEHeader)
	ah.Set("Content-Disposition", `form-data; name="archive"`)
	ah.Set("Content-Type", "application/octet-stream")
	aw, _ := mw.CreatePart(ah)
	aw.Write(archiveData)

	mw.Close()
	return buf.Bytes(), mw.FormDataContentType(), nil
}

func runLocalScript(scriptPath string, stdout, stderr io.Writer) error {
	os.Chmod(scriptPath, 0755)
	c := exec.Command("/bin/sh", "-c", scriptPath)
	c.Stdout = stdout
	c.Stderr = stderr
	return c.Run()
}

// streamAndCheck streams r to out line-by-line and verifies the final
// "[eacd] STATUS:OK" sentinel written by the server. The sentinel line
// itself is not forwarded to out. Returns errMsg as error if the sentinel is absent.
func streamAndCheck(r io.Reader, out io.Writer, errMsg string) error {
	scanner := bufio.NewScanner(r)
	var prev string
	for scanner.Scan() {
		if prev != "" {
			fmt.Fprintln(out, prev)
		}
		prev = scanner.Text()
	}
	if prev == "[eacd] STATUS:OK" {
		return nil
	}
	// Sentinel absent: print the last buffered line (real content) and report failure.
	if prev != "" {
		fmt.Fprintln(out, prev)
	}
	return fmt.Errorf("%s", errMsg)
}
