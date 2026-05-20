package streamer

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// ValidatePath checks that the given path is safe to use:
//   1. Rejects paths containing ".." components before any resolution.
//   2. Cleans the path via filepath.Clean.
//   3. Resolves all symlinks via filepath.EvalSymlinks.
//   4. Verifies the resolved path is within one of the allowed root directories.
//   5. On Windows, uses case-insensitive comparison for prefix checks.
//
// Returns the resolved absolute path on success, or an error describing why
// the path was rejected.
func ValidatePath(path string, roots []string) (string, error) {
	// 1. Reject ".." components before resolution — this prevents an attacker
	// from using a symlink that points above the root, then using ".." to
	// traverse further.
	if strings.Contains(path, "..") {
		return "", fmt.Errorf("path traversal: path contains '..' component")
	}

	// 2. Clean the path to remove redundant separators, . components, etc.
	clean := filepath.Clean(path)

	// 3. Resolve symlinks to get the real, absolute path.
	resolved, err := filepath.EvalSymlinks(clean)
	if err != nil {
		return "", fmt.Errorf("path traversal: cannot resolve path: %w", err)
	}

	// Ensure the resolved path is absolute (EvalSymlinks should guarantee
	// this, but be defensive).
	resolved = filepath.Clean(resolved)

	// 4. Verify the resolved path is within one of the allowed roots.
	for _, root := range roots {
		root = filepath.Clean(root)

		if runtime.GOOS == "windows" {
			// 5. Case-insensitive comparison on Windows.
			if strings.EqualFold(resolved, root) ||
				strings.HasPrefix(strings.ToLower(resolved), strings.ToLower(root)+string(os.PathSeparator)) {
				return resolved, nil
			}
		} else {
			if resolved == root ||
				strings.HasPrefix(resolved, root+string(os.PathSeparator)) {
				return resolved, nil
			}
		}
	}

	return "", fmt.Errorf("path traversal: resolved path %s is not within allowed roots", resolved)
}
