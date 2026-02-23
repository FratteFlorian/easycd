package archive

import (
	"archive/tar"
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// AddFile adds a single file to a tar writer under the given archive path.
func AddFile(tw *tar.Writer, srcPath, archivePath string, mode int64) error {
	f, err := os.Open(srcPath)
	if err != nil {
		return err
	}
	defer f.Close()

	info, err := f.Stat()
	if err != nil {
		return err
	}

	hdr := &tar.Header{
		Name:    archivePath,
		Mode:    mode,
		Size:    info.Size(),
		ModTime: info.ModTime(),
		Typeflag: tar.TypeReg,
	}
	if err := tw.WriteHeader(hdr); err != nil {
		return err
	}
	_, err = io.Copy(tw, f)
	return err
}

// AddDir recursively adds all files in srcDir to the tar writer,
// placing them under archivePrefix. Files matching any exclude pattern are skipped.
// fileMode and dirMode are octal strings like "0644".
func AddDir(tw *tar.Writer, srcDir, archivePrefix string, excludes []string, fileMode, dirMode int64) error {
	return filepath.Walk(srcDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		rel, err := filepath.Rel(srcDir, path)
		if err != nil {
			return err
		}
		if rel == "." {
			return nil
		}

		// Check exclude patterns
		if ShouldExclude(rel, info.IsDir(), excludes) {
			if info.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}

		archivePath := filepath.Join(archivePrefix, rel)

		if info.IsDir() {
			hdr := &tar.Header{
				Name:     archivePath + "/",
				Mode:     dirMode,
				Typeflag: tar.TypeDir,
				ModTime:  info.ModTime(),
			}
			return tw.WriteHeader(hdr)
		}

		return AddFile(tw, path, archivePath, fileMode)
	})
}

// ShouldExclude returns true if the relative path matches any exclude pattern.
// Patterns ending with "/" match directories only.
func ShouldExclude(rel string, isDir bool, excludes []string) bool {
	for _, pattern := range excludes {
		// Directory-only pattern (e.g. "vendor/")
		if strings.HasSuffix(pattern, "/") {
			dirPattern := strings.TrimSuffix(pattern, "/")
			if isDir && (rel == dirPattern || strings.HasPrefix(rel, dirPattern+string(filepath.Separator))) {
				return true
			}
			if strings.HasPrefix(rel, dirPattern+string(filepath.Separator)) {
				return true
			}
			continue
		}

		// Glob pattern match against filename
		matched, _ := filepath.Match(pattern, filepath.Base(rel))
		if matched {
			return true
		}

		// Exact match or prefix match
		if rel == pattern || strings.HasPrefix(rel, pattern+string(filepath.Separator)) {
			return true
		}
	}
	return false
}

// NewWriter returns a gzip+tar writer wrapping w.
// The caller must close both the returned *tar.Writer and *gzip.Writer.
func NewWriter(w io.Writer) (*tar.Writer, *gzip.Writer) {
	gw := gzip.NewWriter(w)
	tw := tar.NewWriter(gw)
	return tw, gw
}

// Extract unpacks a tar.gz from r into destDir.
// Only entries whose name starts with allowedPrefix are extracted.
// This prevents path traversal: archive entry names are never used as destination paths directly.
func Extract(r io.Reader, destDir, allowedPrefix string) error {
	gr, err := gzip.NewReader(r)
	if err != nil {
		return fmt.Errorf("gzip: %w", err)
	}
	defer gr.Close()

	tr := tar.NewReader(gr)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("tar: %w", err)
		}

		// Safety: only extract entries under the allowed prefix
		if !strings.HasPrefix(hdr.Name, allowedPrefix) {
			continue
		}

		target := filepath.Join(destDir, filepath.Clean("/"+hdr.Name))

		// Safety check: ensure the resolved path stays within destDir
		if !strings.HasPrefix(target, filepath.Clean(destDir)+string(filepath.Separator)) &&
			target != filepath.Clean(destDir) {
			continue
		}

		switch hdr.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, os.FileMode(hdr.Mode)); err != nil {
				return err
			}
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
				return err
			}
			f, err := os.OpenFile(target, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, os.FileMode(hdr.Mode))
			if err != nil {
				return err
			}
			if _, err := io.Copy(f, tr); err != nil {
				f.Close()
				return err
			}
			f.Close()
		}
	}
	return nil
}
