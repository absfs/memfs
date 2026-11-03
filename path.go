package memfs

import (
	"os"
	"strings"
	"syscall"

	"github.com/absfs/inode"
)

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
// A trailing slash is ignored, so "file/" resolves to file. The empty path
// does not exist.
func (fs *FileSystem) walk(name string, followLast bool) (*inode.Inode, error) {
	if name == "" {
		return nil, syscall.ENOENT
	}
	cur := fs.root
	rest := fs.absPath(name)
	links := 0
	for {
		rest = strings.TrimLeft(rest, "/")
		if rest == "" {
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
// symbolic link followed, and must be a directory (errNotDir otherwise). The
// final element is returned without trailing slashes and is empty only when
// name refers to the root.
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
		return nil, "", errNotDir
	}
	return parent, base, nil
}
