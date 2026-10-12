package journal

import (
	"fmt"
	"os"
	"path/filepath"
)

const (
	dailyDirLayout      = "2006-01-02"
	filenameTimeLayout  = "20060102T150405Z"
	tempFilePermissions = 0o644
	dirPermissions      = 0o755
)

// Save renders entry to its raw Markdown form and writes it atomically to
// disk under root/userID/YYYY-MM-DD/ (JE-003), using the UTC instant of
// entry.CreatedAt for both the daily folder and the filename. It returns
// the entry's path relative to root.
//
// The write never depends on any model or external service being
// available (JE-012): rendering and persistence are pure, local operations.
func Save(root, userID string, entry Entry, maxEntryBytes int) (relPath string, err error) {
	fm := entry.ToFrontMatter()
	content, err := Render(fm, entry.Title, entry.Description)
	if err != nil {
		return "", fmt.Errorf("rendering entry: %w", err)
	}
	if len(content) > maxEntryBytes {
		return "", validationErr("entry", "rendered entry size %d bytes exceeds the %d byte limit", len(content), maxEntryBytes)
	}

	createdUTC := entry.CreatedAt.UTC()
	dateDir := createdUTC.Format(dailyDirLayout)
	filename := createdUTC.Format(filenameTimeLayout) + "_" + entry.ID + ".md"

	dailyDir := filepath.Join(root, userID, dateDir)
	finalPath := filepath.Join(dailyDir, filename)

	if err := os.MkdirAll(dailyDir, dirPermissions); err != nil {
		return "", fmt.Errorf("creating daily directory: %w", err)
	}

	tmpFile, err := os.CreateTemp(dailyDir, ".tmp-"+entry.ID+"-*")
	if err != nil {
		return "", fmt.Errorf("creating temp file: %w", err)
	}
	tmpPath := tmpFile.Name()
	// Best-effort cleanup: once the rename below succeeds, tmpPath no
	// longer exists and Remove is a silent no-op.
	defer os.Remove(tmpPath)

	if err := writeAndSync(tmpFile, content); err != nil {
		return "", fmt.Errorf("writing temp file: %w", err)
	}
	if err := os.Chmod(tmpPath, tempFilePermissions); err != nil {
		return "", fmt.Errorf("setting temp file permissions: %w", err)
	}

	if err := os.Rename(tmpPath, finalPath); err != nil {
		return "", fmt.Errorf("finalizing entry file: %w", err)
	}

	return filepath.Join(userID, dateDir, filename), nil
}

func writeAndSync(f *os.File, content string) error {
	if _, err := f.WriteString(content); err != nil {
		f.Close()
		return err
	}
	if err := f.Sync(); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}
