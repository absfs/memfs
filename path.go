package memfs

import (
	"os"
	"strings"
	"syscall"

	"github.com/absfs/inode"
)

// errNotDir is the error for a path that runs through something that is not a
// directory, such as "/file.txt/x". It is what the os package reports for
// such a path: ENOTDIR on Unix, and on Windows syscall.ENOTDIR is
// ERROR_PATH_NOT_FOUND, which errors.Is and os.IsNotExist treat as
// fs.ErrNotExist, exactly as for os.
var errNotDir error = syscall.ENOTDIR

// maxSymlinks bounds the number of symbolic links expanded while resolving a
// single path, matching the Linux limit. Exceeding it yields ELOOP.
const maxSymlinks = 40

// absPath returns name unchanged if it is absolute, otherwise it is joined to
// the current working directory. The result is deliberately not cleaned:
// "file/.." must reach the walker so that it can fail with ENOTDIR, as it does
// on Unix, rather than being lexically reduced to the parent directory.
func (fs *FileSystem) absPath(name string) string {
	if strings.HasPrefix(name, "/") {
		return name
	}
	return fs.cwd + "/" + name
}

// walk resolves name to its inode one component at a time.
//
// Every component that has a component after it must resolve to a directory.
// If it resolves to anything else (a regular file, or a symbolic link whose
// target is one), walk fails with errNotDir, the error the os package reports
// for such a path on the current platform. A missing component yields ENOENT.
// Symbolic links in intermediate components are always followed; the final
// component is followed only if followLast is set. "." and ".." are resolved
// against the directory entries, so ".." after a symbolic link leads to the
// parent of the link's target, as on Unix.
//
// A trailing slash names a directory, as on Unix: the final component is
// then followed even if followLast is false, and must resolve to a
// directory, so "file/" and "link-to-file/" fail with errNotDir. The same
// applies to a symbolic link whose target ends in a slash. The empty path
// does not exist.
func (fs *FileSystem) walk(name string, followLast bool) (*inode.Inode, error) {
	if name == "" {
		return nil, syscall.ENOENT
	}
	cur := fs.root
	rest := fs.absPath(name)
	mustDir := strings.HasSuffix(rest, "/")
	followLast = followLast || mustDir
	links := 0
	for {
		rest = strings.TrimLeft(rest, "/")
		if rest == "" {
			if mustDir && !cur.IsDir() {
				return nil, errNotDir
			}
			return cur, nil
		}
		if !cur.IsDir() {
			return nil, errNotDir
		}
		var elem string
		elem, rest, _ = strings.Cut(rest, "/")
		if elem == "." {
			continue
		}
		entry := cur.Lookup(elem)
		if entry == nil {
			return nil, syscall.ENOENT
		}
		next := entry.Inode
		last := strings.TrimLeft(rest, "/") == ""
		if next.Mode&os.ModeSymlink == 0 || (last && !followLast) {
			cur = next
			continue
		}

		links++
		if links > maxSymlinks {
			return nil, syscall.ELOOP
		}
		target, ok := fs.symlinks.Load(next.Ino)
		if !ok || target.(string) == "" {
			return nil, syscall.ENOENT
		}
		if last && strings.HasSuffix(target.(string), "/") {
			mustDir = true
		}
		// A relative target is resolved from the directory holding the
		// link, which is cur; an absolute one restarts at the root.
		rest = target.(string) + "/" + rest
		if strings.HasPrefix(rest, "/") {
			cur = fs.root
		}
	}
}

// walkParent resolves the directory that would hold name and returns it with
// the final path element. The directory part is resolved like walk with every
// symbolic link followed, and must be a directory. If it resolves to something
// else, walkParent returns that inode with errNotDir, so that callers can tell
// this case from a non-directory further up the path (where the inode is nil).
// The final element is returned without trailing slashes and is empty only
// when name refers to the root. A trailing slash is not checked here: callers
// that create, remove or rename the final element apply the rules for it
// themselves (see hasTrailingSlash).
func (fs *FileSystem) walkParent(name string) (*inode.Inode, string, error) {
	if name == "" {
		return nil, "", syscall.ENOENT
	}
	abs := strings.TrimRight(fs.absPath(name), "/")
	if abs == "" {
		return fs.root, "", nil
	}
	i := strings.LastIndex(abs, "/")
	dir, base := abs[:i+1], abs[i+1:]
	parent, err := fs.walk(dir, true)
	if err != nil {
		return nil, "", err
	}
	if !parent.IsDir() {
		return parent, "", errNotDir
	}
	return parent, base, nil
}

// hasTrailingSlash reports whether name ends in a slash after a final
// element, as in "dir/", and so names a directory. The root is not such a
// name.
func hasTrailingSlash(name string) bool {
	return strings.HasSuffix(name, "/") && strings.TrimRight(name, "/") != ""
}

// The errors below are for a trailing slash on a final element that is not,
// or would not be, a directory. Linux and the BSD-derived kernels (macOS,
// FreeBSD) disagree on them, and memfs follows whichever the os package
// follows on the running platform, chosen by linuxSlash.

// errSlashExists is the error from Mkdir or Symlink for "name/" when name
// exists but does not resolve to a directory: EEXIST on Linux, ENOTDIR
// elsewhere.
func errSlashExists() error {
	if linuxSlash {
		return syscall.EEXIST
	}
	return errNotDir
}

// errSlashCreate is the error from OpenFile with O_CREATE for "name/" when
// name does not exist: Linux reports EISDIR for any O_CREATE open of a name
// with a trailing slash, the BSDs ENOENT.
func errSlashCreate() error {
	if linuxSlash {
		return syscall.EISDIR
	}
	return syscall.ENOENT
}

// errSlashRename is the error from Rename when the old path is not a
// directory and the new path is "name/" for a missing name: ENOTDIR on
// Linux, ENOENT elsewhere.
func errSlashRename() error {
	if linuxSlash {
		return errNotDir
	}
	return syscall.ENOENT
}
