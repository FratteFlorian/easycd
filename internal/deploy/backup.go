package deploy

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

const rollbackDir = "/var/lib/eacd"

func rollbackBase(project string) string {
	return filepath.Join(rollbackDir, project, "rollback")
}

// BackupFiles saves the current on-disk versions of destPaths so they can be
// restored by RestoreBackup. newFiles are files that did not exist before this
// deploy and should be deleted on rollback.
func BackupFiles(project string, destPaths []string) error {
	base := rollbackBase(project)
	filesDir := filepath.Join(base, "files")

	// Clean previous backup
	os.RemoveAll(base)
	if err := os.MkdirAll(filesDir, 0755); err != nil {
		return err
	}

	var newFiles []string
	for _, dest := range destPaths {
		if _, err := os.Stat(dest); os.IsNotExist(err) {
			newFiles = append(newFiles, dest)
			continue
		}
		// Backup: store under filesDir using the absolute path as sub-path
		// e.g. /var/www/html/index.html → <filesDir>/var/www/html/index.html
		rel := strings.TrimPrefix(dest, "/")
		backupPath := filepath.Join(filesDir, rel)
		if err := os.MkdirAll(filepath.Dir(backupPath), 0755); err != nil {
			return fmt.Errorf("backup mkdir: %w", err)
		}
		if err := copyFile(dest, backupPath); err != nil {
			return fmt.Errorf("backup %s: %w", dest, err)
		}
	}

	// Persist list of new files (to delete on rollback)
	data, _ := json.Marshal(newFiles)
	return os.WriteFile(filepath.Join(base, "new-files.json"), data, 0644)
}

// RestoreBackup undoes the last deployment: restores backed-up files and
// deletes any files that were new in that deployment.
func RestoreBackup(project string, log io.Writer) error {
	base := rollbackBase(project)
	filesDir := filepath.Join(base, "files")

	if _, err := os.Stat(base); os.IsNotExist(err) {
		return fmt.Errorf("no rollback snapshot available for project %q", project)
	}

	// Restore backed-up files
	err := filepath.Walk(filesDir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return err
		}
		rel, _ := filepath.Rel(filesDir, path)
		dest := "/" + rel
		fmt.Fprintf(log, "[eacd] rollback: restoring %s\n", dest)
		if mkErr := os.MkdirAll(filepath.Dir(dest), 0755); mkErr != nil {
			return mkErr
		}
		return copyFile(path, dest)
	})
	if err != nil {
		return fmt.Errorf("restoring files: %w", err)
	}

	// Delete files that were new in the rolled-back deploy
	raw, _ := os.ReadFile(filepath.Join(base, "new-files.json"))
	var newFiles []string
	if len(raw) > 0 {
		json.Unmarshal(raw, &newFiles)
	}
	for _, f := range newFiles {
		fmt.Fprintf(log, "[eacd] rollback: removing new file %s\n", f)
		os.Remove(f)
	}

	os.RemoveAll(base)
	return nil
}

// RollbackAvailable returns true if a rollback snapshot exists for the project.
func RollbackAvailable(project string) bool {
	_, err := os.Stat(rollbackBase(project))
	return err == nil
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	info, err := in.Stat()
	if err != nil {
		return err
	}

	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, info.Mode())
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, in)
	return err
}
