package commands

import (
	"fmt"
	"os"

	"github.com/mad01/ralph/internal/config"
	"github.com/mad01/ralph/internal/report"
)

// validateDotfileTarget checks whether a dotfile's target is in the expected
// state based on its action type (symlink, copy, symlink_dir).
// Returns (status, message, error) suitable for report.Phase.Add* calls.
func validateDotfileTarget(
	df config.Dotfile,
	absoluteTarget string,
	repoPath string,
) (report.Status, string, error) {
	targetInfo, statErr := os.Lstat(absoluteTarget)
	if os.IsNotExist(statErr) {
		return report.StatusWarn, "not linked (target does not exist)", nil
	}
	if statErr != nil {
		return report.StatusFail, fmt.Sprintf("error checking target: %v", statErr), statErr
	}

	switch df.Action {
	case "copy":
		return validateCopyTarget(df, absoluteTarget, repoPath, targetInfo)
	case "symlink_dir":
		return validateDirSymlinkTarget(df, absoluteTarget, repoPath, targetInfo)
	default:
		return validateSymlinkTarget(df, absoluteTarget, repoPath, targetInfo)
	}
}

func validateCopyTarget(
	df config.Dotfile,
	absoluteTarget string,
	repoPath string,
	targetInfo os.FileInfo,
) (report.Status, string, error) {
	if targetInfo.Mode()&os.ModeSymlink != 0 {
		return report.StatusWarn, "expected regular file (action=copy) but found symlink", nil
	}
	if !targetInfo.Mode().IsRegular() {
		return report.StatusWarn, "expected regular file (action=copy) but found non-regular file", nil
	}

	// Size comparison for drift detection (skip for templates — source is ephemeral)
	if !df.IsTemplate && repoPath != "" {
		sourcePath := config.JoinSourcePath(repoPath, df.Source)
		sourceInfo, err := os.Stat(sourcePath)
		if err == nil && sourceInfo.Size() != targetInfo.Size() {
			return report.StatusWarn, fmt.Sprintf(
				"size mismatch (source: %d bytes, target: %d bytes)",
				sourceInfo.Size(),
				targetInfo.Size(),
			), nil
		}
	}

	return report.StatusOK, "", nil
}

func validateDirSymlinkTarget(
	df config.Dotfile,
	absoluteTarget string,
	repoPath string,
	targetInfo os.FileInfo,
) (report.Status, string, error) {
	if targetInfo.Mode()&os.ModeSymlink == 0 {
		return report.StatusWarn, "expected directory symlink (action=symlink_dir) but target is not a symlink", nil
	}

	linkDest, err := os.Readlink(absoluteTarget)
	if err != nil {
		return report.StatusFail, fmt.Sprintf("error reading symlink destination: %v", err), err
	}

	expectedSource := config.JoinSourcePath(repoPath, df.Source)
	if linkDest != expectedSource {
		return report.StatusWarn, fmt.Sprintf(
			"directory symlink points to '%s', expected '%s'",
			linkDest,
			expectedSource,
		), nil
	}

	return report.StatusOK, "", nil
}

func validateSymlinkTarget(
	df config.Dotfile,
	absoluteTarget string,
	repoPath string,
	targetInfo os.FileInfo,
) (report.Status, string, error) {
	if targetInfo.Mode()&os.ModeSymlink == 0 {
		return report.StatusWarn, "exists but is not a symlink", nil
	}

	linkDest, readlinkErr := os.Readlink(absoluteTarget)
	if readlinkErr != nil {
		return report.StatusFail, fmt.Sprintf(
			"error reading symlink destination: %v",
			readlinkErr,
		), readlinkErr
	}

	var actualSourcePath string
	if df.IsTemplate {
		actualSourcePath = linkDest
	} else {
		expectedSource := config.JoinSourcePath(repoPath, df.Source)
		actualSourcePath = expectedSource
		if linkDest != actualSourcePath {
			actualSourcePath = linkDest
		}
	}

	if _, err := os.Stat(actualSourcePath); os.IsNotExist(err) {
		return report.StatusFail, fmt.Sprintf(
			"broken symlink (source '%s' does not exist)",
			actualSourcePath,
		), err
	} else if err != nil {
		return report.StatusFail, fmt.Sprintf("error stating source '%s': %v", actualSourcePath, err), err
	}

	return report.StatusOK, "", nil
}
