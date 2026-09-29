//go:build windows

package update

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"time"
)

// replaceFile moves src over dst. Windows won't overwrite a running .exe, but
// it will rename one, so dst (possibly this very process, or the tray) is
// first moved aside to dst.old and the new file takes its place. If that
// fails, the old file is put back.
func replaceFile(src, dst string) error {
	if _, err := os.Stat(dst); err == nil {
		old := dst + ".old"
		if err := os.Remove(old); err != nil && !os.IsNotExist(err) {
			// A previous update's .old is still running (e.g. the tray).
			old = dst + "." + strconv.FormatInt(time.Now().UnixNano(), 10) + ".old"
		}
		if err := os.Rename(dst, old); err != nil {
			return fmt.Errorf("move the current %s aside: %w", filepath.Base(dst), err)
		}
		if err := os.Rename(src, dst); err != nil {
			if rbErr := os.Rename(old, dst); rbErr != nil {
				return fmt.Errorf("put new %s in place: %w (and restoring the old one failed: %v)", filepath.Base(dst), err, rbErr)
			}
			return fmt.Errorf("put new %s in place: %w (the old one was restored)", filepath.Base(dst), err)
		}
		return nil
	}
	return os.Rename(src, dst)
}

// cleanupOld deletes *.old files left by previous updates. Ones still running
// (a tray that hasn't been restarted) can't be deleted yet; they're simply
// retried next time.
func cleanupOld(dir string) {
	matches, _ := filepath.Glob(filepath.Join(dir, "portfind*.old"))
	for _, m := range matches {
		os.Remove(m)
	}
}
