//go:build windows

package memfs

import "syscall"

// errNotDir is the error reported when a path runs through something that is
// not a directory, such as "/file.txt/x". On Windows the os package reports
// ERROR_PATH_NOT_FOUND for such a path, which errors.Is and os.IsNotExist
// treat as fs.ErrNotExist, so memfs returns the same errno.
var errNotDir error = syscall.ERROR_PATH_NOT_FOUND
