//go:build windows

package memfs

import "syscall"

// On Windows, syscall.ENOTDIR is ERROR_PATH_NOT_FOUND, which is what os
// reports for most paths that run through a non-directory, and which
// errors.Is treats as fs.ErrNotExist. Two operations differ, and memfs
// returns what os returns for them.
var (
	// errChdirNotDir is returned by Chdir when the target is not a
	// directory: os.Chdir reports ERROR_DIRECTORY ("The directory name is
	// invalid"), which is not fs.ErrNotExist.
	errChdirNotDir error = syscall.Errno(267) // ERROR_DIRECTORY

	// errRenameParentNotDir is returned by Rename when the parent of the new
	// path exists but is not a directory: MoveFileEx, and so os.Rename,
	// reports ERROR_INVALID_PARAMETER, which is not fs.ErrNotExist. A
	// non-directory further up the new path still yields
	// ERROR_PATH_NOT_FOUND.
	errRenameParentNotDir error = syscall.Errno(87) // ERROR_INVALID_PARAMETER
)
