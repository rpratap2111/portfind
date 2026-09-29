//go:build !windows

package update

import "os"

// replaceFile atomically renames src over dst. A running program keeps using
// its old file (the inode stays alive until it exits), so this is safe even
// for the portfind doing the update.
func replaceFile(src, dst string) error {
	return os.Rename(src, dst)
}

// cleanupOld is a no-op: only Windows leaves .old files behind.
func cleanupOld(string) {}
