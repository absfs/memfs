//go:build !windows

package memfs

import "syscall"

// Errors for the two cases where os on Windows does not report a path that
// runs through a non-directory as ERROR_PATH_NOT_FOUND. On Unix both are
// plain ENOTDIR. See errors_windows.go.
var (
	// errChdirNotDir is returned by Chdir when the target is not a directory.
	errChdirNotDir error = syscall.ENOTDIR

	// errRenameParentNotDir is returned by Rename when the parent of the new
	// path exists but is not a directory.
	errRenameParentNotDir error = syscall.ENOTDIR
)
