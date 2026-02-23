package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/flo-mic/simplecd/internal/api"
	"github.com/flo-mic/simplecd/internal/archive"
	"github.com/flo-mic/simplecd/internal/auth"
	"github.com/flo-mic/simplecd/internal/config"
	"github.com/flo-mic/simplecd/internal/delta"
	"github.com/flo-mic/simplecd/internal/deploy"
	"github.com/flo-mic/simplecd/internal/inventory"
)

var deployMu sync.Mutex

// rateLimiter is a simple sliding-window per-IP rate limiter.
type rateLimiter struct {
	mu       sync.Mutex
	requests map[string][]time.Time
	limit    int
	window   time.Duration
}

func newRateLimiter(limit int, window time.Duration) *rateLimiter {
	return &rateLimiter{requests: make(map[string][]time.Time), limit: limit, window: window}
}

func (rl *rateLimiter) allow(ip string) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	now := time.Now()
	cutoff := now.Add(-rl.window)
	var valid []time.Time
	for _, t := range rl.requests[ip] {
		if t.After(cutoff) {
			valid = append(valid, t)
		}
	}
	if len(valid) >= rl.limit {
		rl.requests[ip] = valid
		return false
	}
	rl.requests[ip] = append(valid, now)
	return true
}

func (rl *rateLimiter) middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ip, _, _ := net.SplitHostPort(r.RemoteAddr)
		if !rl.allow(ip) {
			http.Error(w, "rate limit exceeded", http.StatusTooManyRequests)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func main() {
	cfgPath := flag.String("config", "/etc/simplecd/server.yaml", "Path to server config")
	flag.Parse()

	cfg, err := config.LoadServerConfig(*cfgPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	if err := os.MkdirAll(cfg.LogDir, 0755); err != nil {
		fmt.Fprintf(os.Stderr, "error creating log dir: %v\n", err)
		os.Exit(1)
	}

	logFile, err := os.OpenFile(filepath.Join(cfg.LogDir, "simplecdd.log"), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error opening log file: %v\n", err)
		os.Exit(1)
	}
	defer logFile.Close()

	logger := slog.New(slog.NewTextHandler(io.MultiWriter(os.Stdout, logFile), nil))
	slog.SetDefault(logger)

	checkRL := newRateLimiter(60, time.Minute)  // 60 checks/min per IP
	deployRL := newRateLimiter(10, time.Minute) // 10 deploys/min per IP

	mux := http.NewServeMux()
	mux.Handle("/check", checkRL.middleware(auth.Middleware(cfg.Token, http.HandlerFunc(handleCheck))))
	mux.Handle("/deploy", deployRL.middleware(auth.Middleware(cfg.Token, http.HandlerFunc(handleDeploy))))
	mux.Handle("/rollback", deployRL.middleware(auth.Middleware(cfg.Token, http.HandlerFunc(handleRollback))))
	mux.Handle("/health", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		fmt.Fprintln(w, "ok")
	}))

	slog.Info("simplecdd starting", "listen", cfg.Listen)
	if err := http.ListenAndServe(cfg.Listen, mux); err != nil {
		slog.Error("server error", "err", err)
		os.Exit(1)
	}
}

// handleCheck compares the client's file hashes against what's on disk
// and returns which files need to be uploaded.
func handleCheck(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req api.CheckRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "bad request: "+err.Error(), http.StatusBadRequest)
		return
	}

	dests := make([]string, len(req.Files))
	clientHashes := make(map[string]string, len(req.Files))
	for i, f := range req.Files {
		dests[i] = f.Dest
		clientHashes[f.Dest] = f.Hash
	}

	serverHashes := delta.HashExistingFiles(dests)

	var upload []string
	for dest, clientHash := range clientHashes {
		if serverHashes[dest] != clientHash {
			upload = append(upload, dest)
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(api.CheckResponse{Upload: upload})
}

// handleDeploy processes a deployment request.
func handleDeploy(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Serialize deployments — one at a time
	if !deployMu.TryLock() {
		http.Error(w, "deployment in progress, try again later", http.StatusConflict)
		return
	}
	defer deployMu.Unlock()

	// Set up streaming response
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(http.StatusOK)
	log := &flushWriter{w: w}

	mr, err := r.MultipartReader()
	if err != nil {
		fmt.Fprintf(log, "[simplecd] ERROR: reading multipart: %v\n", err)
		return
	}

	// Part 1: manifest
	manifestPart, err := mr.NextPart()
	if err != nil || manifestPart.FormName() != "manifest" {
		fmt.Fprintf(log, "[simplecd] ERROR: expected 'manifest' part\n")
		return
	}
	var manifest api.Manifest
	if err := json.NewDecoder(manifestPart).Decode(&manifest); err != nil {
		fmt.Fprintf(log, "[simplecd] ERROR: parsing manifest: %v\n", err)
		return
	}

	// Part 2: archive
	archivePart, err := mr.NextPart()
	if err != nil || archivePart.FormName() != "archive" {
		fmt.Fprintf(log, "[simplecd] ERROR: expected 'archive' part\n")
		return
	}

	// Extract archive to temp dir
	tmpDir, err := os.MkdirTemp("", "simplecd-")
	if err != nil {
		fmt.Fprintf(log, "[simplecd] ERROR: creating temp dir: %v\n", err)
		return
	}
	defer os.RemoveAll(tmpDir)

	if err := archive.Extract(archivePart, tmpDir, ""); err != nil {
		fmt.Fprintf(log, "[simplecd] ERROR: extracting archive: %v\n", err)
		return
	}

	fmt.Fprintf(log, "[simplecd] Starting deployment of %s\n", manifest.Name)

	// Inventory reconciliation (before file placement)
	if manifest.Inventory != nil {
		fmt.Fprintf(log, "[simplecd] Reconciling inventory...\n")
		if err := inventory.Reconcile(manifest.Name, manifest.Inventory, log); err != nil {
			fmt.Fprintf(log, "[simplecd] ERROR: inventory reconciliation: %v\n", err)
			return
		}
	}

	// Backup existing files for rollback
	var destPaths []string
	for _, f := range manifest.Files {
		destPaths = append(destPaths, f.Dest)
	}
	if err := deploy.BackupFiles(manifest.Name, destPaths); err != nil {
		fmt.Fprintf(log, "[simplecd] WARNING: backup failed (rollback unavailable): %v\n", err)
	}

	// Server pre-hook
	if manifest.Hooks != nil && manifest.Hooks.ServerPre != "" {
		scriptPath := filepath.Join(tmpDir, manifest.Hooks.ServerPre)
		if err := os.Chmod(scriptPath, 0755); err == nil {
			if err := deploy.RunHook(scriptPath, log); err != nil {
				fmt.Fprintf(log, "[simplecd] ERROR: pre-hook: %v\n", err)
				return
			}
		}
	}

	// Place files
	for _, f := range manifest.Files {
		if f.ArchivePath == "" {
			fmt.Fprintf(log, "[simplecd] Skipping %s (unchanged)\n", f.Dest)
			continue
		}
		src := filepath.Join(tmpDir, f.ArchivePath)
		if err := deploy.PlaceFile(src, f.Dest, f.Mode, log); err != nil {
			fmt.Fprintf(log, "[simplecd] ERROR: placing %s: %v\n", f.Dest, err)
			return
		}
	}

	// Systemd unit
	if manifest.Systemd != nil && manifest.Systemd.UnitArchivePath != "" {
		src := filepath.Join(tmpDir, manifest.Systemd.UnitArchivePath)
		if err := deploy.InstallUnit(src, manifest.Systemd.UnitDest, manifest.Systemd.Enable, manifest.Systemd.Restart, log); err != nil {
			fmt.Fprintf(log, "[simplecd] ERROR: systemd: %v\n", err)
			return
		}
	}

	// Server post-hook (failure is non-fatal)
	if manifest.Hooks != nil && manifest.Hooks.ServerPost != "" {
		scriptPath := filepath.Join(tmpDir, manifest.Hooks.ServerPost)
		if err := os.Chmod(scriptPath, 0755); err == nil {
			if err := deploy.RunHook(scriptPath, log); err != nil {
				fmt.Fprintf(log, "[simplecd] WARNING: post-hook failed: %v\n", err)
			}
		}
	}

	slog.Info("deployment complete", "project", manifest.Name)
	fmt.Fprintf(log, "[simplecd] Deployment complete\n")
}

// handleRollback restores the previous deployment snapshot for a project.
func handleRollback(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Name == "" {
		http.Error(w, "bad request: missing project name", http.StatusBadRequest)
		return
	}

	if !deployMu.TryLock() {
		http.Error(w, "deployment in progress, try again later", http.StatusConflict)
		return
	}
	defer deployMu.Unlock()

	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(http.StatusOK)
	log := &flushWriter{w: w}

	if !deploy.RollbackAvailable(req.Name) {
		fmt.Fprintf(log, "[simplecd] ERROR: no rollback snapshot available for %q\n", req.Name)
		return
	}

	fmt.Fprintf(log, "[simplecd] Rolling back %s...\n", req.Name)
	if err := deploy.RestoreBackup(req.Name, log); err != nil {
		fmt.Fprintf(log, "[simplecd] ERROR: rollback failed: %v\n", err)
		return
	}

	slog.Info("rollback complete", "project", req.Name)
	fmt.Fprintf(log, "[simplecd] Rollback complete\n")
}

// flushWriter wraps a ResponseWriter and flushes after each write for streaming.
type flushWriter struct {
	w http.ResponseWriter
}

func (fw *flushWriter) Write(p []byte) (int, error) {
	n, err := fw.w.Write(p)
	if f, ok := fw.w.(http.Flusher); ok {
		f.Flush()
	}
	return n, err
}
