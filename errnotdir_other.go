//go:build !windows

package memfs

import "syscall"

// errNotDir is the error reported when a path runs through something that is
// not a directory, such as "/file.txt/x". On Unix the os package reports
// syscall.ENOTDIR, which errors.Is does not treat as fs.ErrNotExist.
var errNotDir error = syscall.ENOTDIR
