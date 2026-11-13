package state

import (
	"fmt"
	"os"
	"time"
)

// A corrupt state file is moved to <state file>.corrupt-<UTC time>, for
// example .recipe_state.corrupt-20261113T120000Z. When that name is taken a
// numeric suffix (.1, .2, ...) is added, so an earlier backup is never
// overwritten.
const (
	corruptBackupInfix  = ".corrupt-"
	corruptBackupLayout = "20060102T150405Z"
)

// CorruptBackupPattern describes where MoveCorruptAside moves the state file
// at path, for messages that name it before the move happens.
func CorruptBackupPattern(path string) string {
	return path + corruptBackupInfix + "<UTC time>"
}

// MoveCorruptAside renames the state file to a backup next to it (see
// CorruptBackupPattern) and returns the backup path. The caller holds the run
// lock, so no other ralph run races for the same backup name.
func MoveCorruptAside(now time.Time) (string, error) {
	p, err := GetStatePath()
	if err != nil {
		return "", err
	}
	backup, err := freeBackupPath(p + corruptBackupInfix + now.UTC().Format(corruptBackupLayout))
	if err != nil {
		return "", err
	}
	if err := os.Rename(p, backup); err != nil {
		return "", fmt.Errorf("move recipe state aside: %w", err)
	}
	return backup, nil
}

// freeBackupPath returns base, or base with the first numeric suffix that
// names no existing file.
func freeBackupPath(base string) (string, error) {
	candidate := base
	for i := 1; ; i++ {
		_, err := os.Lstat(candidate)
		if os.IsNotExist(err) {
			return candidate, nil
		}
		if err != nil {
			return "", fmt.Errorf("check backup path: %w", err)
		}
		candidate = fmt.Sprintf("%s.%d", base, i)
	}
}
