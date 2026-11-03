//go:build !windows

package memfs

import (
	"runtime"
	"syscall"
)

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

// linuxSlash selects the Linux rules for a trailing slash on a name that is
// not a directory, where they differ from those of macOS and the BSDs. See
// errSlashExists and its neighbours in path.go. It is a variable so that the
// tests can exercise both sets of rules on any platform.
var linuxSlash = runtime.GOOS == "linux" || runtime.GOOS == "android"
