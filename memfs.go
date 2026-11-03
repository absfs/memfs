// Package memfs provides an in-memory file system implementation
// that conforms to the absfs.FileSystem interface.
//
// This package implements a complete virtual file system stored entirely
// in memory, supporting standard file operations, directories, symbolic
// links, permissions, and file metadata. It is particularly useful for
// testing, temporary storage, and scenarios where a full filesystem
// is needed without disk I/O.
package memfs

import (
	"errors"
	"io"
	"io/fs"
	"os"
	"path"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/absfs/absfs"
	"github.com/absfs/inode"
)

// FileSystem represents an in-memory file system.
//
// It maintains a hierarchical structure of inodes representing files
// and directories, along with their associated data and metadata.
// The file system supports standard POSIX-like operations including
// file creation, deletion, permissions, symbolic links, and directory
// traversal.
//
// Thread Safety: FileSystem is not safe for concurrent use. File data lives
// in a synchronized ByteStore, but operations that change the directory tree
// (Rename, Remove, Mkdir and so on) take several steps without a lock over
// them. Wrap a FileSystem with github.com/absfs/lockfs to share it between
// goroutines.
type FileSystem struct {
	Umask   os.FileMode
	Tempdir string

	root *inode.Inode
	cwd  string
	dir  *inode.Inode
	ino  *inode.Ino

	store    *MemByteStore
	symlinks sync.Map // uint64 -> string
}

// NewFS creates and initializes a new in-memory file system.
//
// The file system is created with a root directory ("/") and default
// settings including a umask of 0755 and a temp directory at "/tmp".
// Returns a pointer to the initialized FileSystem and nil error.
func NewFS() (*FileSystem, error) {
	fs := new(FileSystem)
	fs.ino = new(inode.Ino)
	fs.Tempdir = "/tmp"

	fs.Umask = 0755
	fs.root = fs.ino.NewDir(fs.Umask)
	fs.cwd = "/"
	fs.dir = fs.root
	fs.store = NewMemByteStore()
	// fs.symlinks is sync.Map - zero value is ready to use
	return fs, nil
}

// Rename moves or renames a file or directory from oldpath to newpath.
//
// Both relative and absolute paths are supported. Relative paths are
// resolved relative to the current working directory. The root directory
// cannot be renamed. Returns an error if oldpath does not exist, newpath
// already exists, a directory would be moved beneath itself, or either
// path runs through something that is not a directory, with the error the
// os package reports on the running platform: ENOTDIR on Unix; on Windows
// ERROR_INVALID_PARAMETER when newpath's parent is not a directory and
// ERROR_PATH_NOT_FOUND otherwise.
func (fs *FileSystem) Rename(oldpath, newpath string) error {
	linkErr := &os.LinkError{
		Op:  "rename",
		Old: oldpath,
		New: newpath,
	}
	if oldpath == "/" {
		linkErr.Err = syscall.EINVAL
		return linkErr
	}

	srcParent, srcName, err := fs.walkParent(oldpath)
	if err != nil {
		linkErr.Err = err
		return linkErr
	}
	if isDotName(srcName) {
		linkErr.Err = syscall.EINVAL
		return linkErr
	}
	src := srcParent.Lookup(srcName)
	if src == nil {
		linkErr.Err = syscall.ENOENT
		return linkErr
	}
	node := src.Inode
	if err := fs.slashEntryErr(oldpath, node); err != nil {
		linkErr.Err = err
		return linkErr
	}

	dstParent, dstName, err := fs.walkParent(newpath)
	if err != nil {
		if dstParent != nil {
			// The new path's parent exists but is not a directory.
			err = errRenameParentNotDir
		}
		linkErr.Err = err
		return linkErr
	}
	if isDotName(dstName) {
		linkErr.Err = syscall.EINVAL
		return linkErr
	}
	newSlash := hasTrailingSlash(newpath)
	if dstParent.Lookup(dstName) != nil {
		linkErr.Err = syscall.EEXIST
		if newSlash {
			// "file/" fails with ENOTDIR before memfs's refusal to
			// replace an existing entry, as it does in os.
			if _, err := fs.walk(newpath, true); err != nil {
				linkErr.Err = err
			}
		}
		return linkErr
	}
	if newSlash && !node.IsDir() {
		if err := errSlashRename(); err != nil {
			linkErr.Err = err
			return linkErr
		}
	}
	if node.IsDir() && fs.isWithin(dstParent, node) {
		linkErr.Err = syscall.EINVAL
		return linkErr
	}

	// Neither parent can fail here: both are directories and the source
	// entry was found above.
	dstParent.Link(dstName, node)
	srcParent.Unlink(srcName)
	if node.IsDir() && srcParent != dstParent {
		node.Link("..", dstParent)
	}
	return nil
}

// isDotName reports whether a final path element cannot be created, removed
// or renamed: the root's empty name, "." and "..".
func isDotName(name string) bool {
	return name == "" || name == "." || name == ".."
}

// isWithin reports whether dir is node or lies beneath it, following ".."
// entries from dir up to the root.
func (fs *FileSystem) isWithin(dir, node *inode.Inode) bool {
	for {
		if dir == node {
			return true
		}
		if dir == fs.root {
			return false
		}
		up := dir.Lookup("..")
		if up == nil || up.Inode == dir {
			return false
		}
		dir = up.Inode
	}
}

// Chdir changes the current working directory to the named directory.
//
// The directory must exist and be accessible. Both absolute and relative
// paths are supported. Returns an error if the path does not exist or is
// not a directory; for the latter the error is the one os.Chdir gives,
// ENOTDIR on Unix and ERROR_DIRECTORY on Windows. Symbolic links in name are
// resolved, and Getwd then reports the directory reached, as os.Getwd does:
// after Chdir("/link/..") it is the parent of the link's target.
func (fs *FileSystem) Chdir(name string) (err error) {
	node, err := fs.walk(name, true)
	if err != nil {
		return &os.PathError{Op: "chdir", Path: name, Err: err}
	}
	if !node.IsDir() {
		return &os.PathError{Op: "chdir", Path: name, Err: errChdirNotDir}
	}

	cwd, ok := fs.pathOf(node)
	if !ok {
		return &os.PathError{Op: "chdir", Path: name, Err: syscall.ENOENT}
	}
	fs.cwd = cwd
	fs.dir = node
	return nil
}

// pathOf returns the absolute path of the directory dir, without symbolic
// links, ".", or "..". Like a physical getwd it follows the ".." entries from
// dir up to the root and looks each directory up in its parent. It reports
// false if dir is no longer linked into the tree.
func (fs *FileSystem) pathOf(dir *inode.Inode) (string, bool) {
	var elems []string
	for dir != fs.root {
		up := dir.Lookup("..")
		if up == nil || up.Inode == dir {
			return "", false
		}
		name, ok := entryName(up.Inode, dir)
		if !ok {
			return "", false
		}
		elems = append(elems, name)
		dir = up.Inode
	}
	var b strings.Builder
	for i := len(elems) - 1; i >= 0; i-- {
		b.WriteString("/")
		b.WriteString(elems[i])
	}
	if b.Len() == 0 {
		return "/", true
	}
	return b.String(), true
}

// entryName returns the name under which parent holds child, other than "."
// and "..".
func entryName(parent, child *inode.Inode) (string, bool) {
	for _, e := range parent.ReadDir() {
		if e.Inode == child && e.Name() != "." && e.Name() != ".." {
			return e.Name(), true
		}
	}
	return "", false
}

// Getwd returns the current working directory path.
//
// The returned path is always an absolute path. This method never returns
// an error in the current implementation.
func (fs *FileSystem) Getwd() (dir string, err error) {
	return fs.cwd, nil
}

// TempDir returns the path to the temporary directory.
//
// This directory is typically used for temporary file storage. The default
// value is "/tmp", but can be configured via the Tempdir field.
func (fs *FileSystem) TempDir() string {
	return fs.Tempdir
}

// Open opens the named file for reading.
//
// This is equivalent to OpenFile(name, os.O_RDONLY, 0).
// Returns an error if the file does not exist or cannot be opened.
func (fs *FileSystem) Open(name string) (absfs.File, error) {
	return fs.OpenFile(name, os.O_RDONLY, 0)
}

// Create creates or truncates the named file for writing.
//
// This is equivalent to OpenFile(name, os.O_CREATE|os.O_RDWR|os.O_TRUNC, 0644).
// If the file already exists, it is truncated. Returns an error if the file
// cannot be created.
func (fs *FileSystem) Create(name string) (absfs.File, error) {
	return fs.OpenFile(name, os.O_CREATE|os.O_RDWR|os.O_TRUNC, 0644)
}

// OpenFile opens the named file with specified flags and permissions.
//
// Supported flags include os.O_RDONLY, os.O_WRONLY, os.O_RDWR, os.O_CREATE,
// os.O_EXCL, and os.O_TRUNC. The perm argument specifies the file permissions
// to use if a new file is created. Both absolute and relative paths are supported.
// Symlinks are followed automatically when opening files.
// Returns an error if the operation fails due to permission issues, missing
// parent directories, or flag conflicts.
func (fs *FileSystem) OpenFile(name string, flag int, perm os.FileMode) (absfs.File, error) {
	if name == "/" {
		return &File{fs: fs, name: name, flags: flag, node: fs.root}, nil
	}
	if name == "." {
		return &File{fs: fs, name: name, flags: flag, node: fs.dir}, nil
	}

	access := flag & absfs.O_ACCESS
	create := flag&os.O_CREATE != 0
	excl := flag&os.O_EXCL != 0
	truncate := flag&os.O_TRUNC != 0
	fail := func(err error) (absfs.File, error) {
		return &absfs.InvalidFile{Path: name}, &os.PathError{Op: "open", Path: name, Err: err}
	}

	slash := hasTrailingSlash(name)
	if create && slash && slashStyle == linuxSlash {
		// Linux refuses O_CREATE on "name/" once the parent resolves,
		// whatever name is.
		if _, _, err := fs.walkParent(name); err != nil {
			return fail(err)
		}
		return fail(syscall.EISDIR)
	}

	// Resolve the file, following symlinks. A path through something that
	// is not a directory fails here, with or without O_CREATE.
	node, err := fs.walk(name, true)
	switch {
	case err == nil:
		// Windows cannot open "dir\" for writing or creation at all.
		if node.IsDir() && slash && slashStyle == windowsSlash &&
			(create || truncate || access != os.O_RDONLY) {
			return fail(errNotDir)
		}
		// err if exclusive create is required
		if create && excl {
			return fail(syscall.EEXIST)
		}
		if node.IsDir() && (access != os.O_RDONLY || truncate) {
			return fail(syscall.EISDIR)
		}

		// if we must truncate the file
		if truncate {
			fs.store.Truncate(node.Ino, 0)
			node.Size = 0
		}

	case err == syscall.ENOENT && create:
		node, err = fs.create(name, excl, perm)
		if err != nil {
			return fail(err)
		}

	default:
		return fail(err)
	}

	// For existing files (not newly created), verify that the file's permission bits
	// allow the requested access mode. Check that:
	// - Read-only access requires read permission (OS_ALL_R)
	// - Write-only access requires write permission (OS_ALL_W)
	// - Read-write access requires both read and write permissions
	if !create {
		if access == os.O_RDONLY && node.Mode&absfs.OS_ALL_R == 0 ||
			access == os.O_WRONLY && node.Mode&absfs.OS_ALL_W == 0 ||
			access == os.O_RDWR && (node.Mode&absfs.OS_ALL_R == 0 || node.Mode&absfs.OS_ALL_W == 0) {
			return &absfs.InvalidFile{Path: name}, &os.PathError{Op: "open", Path: name, Err: os.ErrPermission}
		}
	}
	return &File{fs: fs, name: name, flags: flag, node: node}, nil
}

// create makes a new regular file at name, which walk reported missing. If
// name is a dangling symbolic link the file is created where the link points,
// as os does, unless excl is set, in which case the link counts as existing.
func (fs *FileSystem) create(name string, excl bool, perm os.FileMode) (*inode.Inode, error) {
	for links := 0; ; links++ {
		if hasTrailingSlash(name) {
			return nil, errSlashCreate()
		}
		parent, filename, err := fs.walkParent(name)
		if err != nil {
			return nil, err
		}
		entry := parent.Lookup(filename)
		if entry == nil {
			// Link cannot fail: parent is a directory. The store needs
			// no entry for an empty file.
			node := fs.ino.New(fs.Umask & perm)
			parent.Link(filename, node)
			return node, nil
		}

		// The name exists but did not resolve, so it is a dangling
		// symbolic link (walk would have resolved anything else).
		if excl {
			return nil, syscall.EEXIST
		}
		target, _ := fs.symlinks.Load(entry.Inode.Ino)
		t, _ := target.(string)
		if t == "" || links >= maxSymlinks {
			return nil, syscall.ENOENT
		}
		if !strings.HasPrefix(t, "/") {
			abs := strings.TrimRight(fs.absPath(name), "/")
			t = abs[:strings.LastIndex(abs, "/")+1] + t
		}
		name = t
	}
}

// Truncate changes the size of the named file.
//
// If the file is larger than size, it is truncated. If it is smaller,
// it is extended with zero bytes. Returns an error if the file does not
// exist.
func (fs *FileSystem) Truncate(name string, size int64) error {
	child, err := fs.walk(name, true)
	if err != nil {
		return &os.PathError{Op: "truncate", Path: name, Err: err}
	}
	if child.IsDir() {
		var err error = syscall.EISDIR
		if hasTrailingSlash(name) && slashStyle == windowsSlash {
			// As in OpenFile, Windows cannot open "dir\" for writing.
			err = errNotDir
		}
		return &os.PathError{Op: "truncate", Path: name, Err: err}
	}

	err = fs.store.Truncate(child.Ino, size)
	if err != nil {
		return &os.PathError{Op: "truncate", Path: name, Err: err}
	}
	child.Size = size
	return nil
}

// Mkdir creates a new directory with the specified name and permissions.
//
// The parent directory must already exist. Returns an error if the directory
// already exists, the parent directory does not exist, or the parent path
// runs through something that is not a directory.
func (fs *FileSystem) Mkdir(name string, perm os.FileMode) error {
	parent, filename, err := fs.walkParent(name)
	if err != nil {
		return &os.PathError{Op: "mkdir", Path: name, Err: err}
	}
	if filename == "" || parent.Lookup(filename) != nil {
		return &os.PathError{Op: "mkdir", Path: name, Err: fs.existsErr(name, true)}
	}

	// Link cannot fail: parent is a directory.
	child := fs.ino.NewDir(fs.Umask & perm)
	parent.Link(filename, child)
	child.Link("..", parent)
	// No need to initialize store - it handles non-existent files gracefully
	return nil
}

// MkdirAll creates a directory and all necessary parent directories.
//
// If the directory already exists, MkdirAll does nothing and returns nil.
// This is similar to the 'mkdir -p' command in Unix. As with os.MkdirAll, if
// name or any of its ancestors exists but is not a directory, MkdirAll fails
// with syscall.ENOTDIR, naming the offending path, and a path that cannot be
// resolved at all (a dangling or looping symbolic link) fails with the error
// Mkdir gives for it.
func (fs *FileSystem) MkdirAll(name string, perm os.FileMode) error {
	// This follows os.MkdirAll step for step, so that it fails, and names
	// the failing path, as os.MkdirAll does.
	if name == "" {
		// os.MkdirAll("") fails; memfs has always accepted it.
		return nil
	}
	node, err := fs.walk(name, true)
	if err == nil {
		if node.IsDir() {
			return nil
		}
		return &os.PathError{Op: "mkdir", Path: name, Err: syscall.ENOTDIR}
	}

	// Make sure the parent exists, then create name itself.
	i := len(name) - 1
	for i >= 0 && name[i] == '/' {
		i--
	}
	for i >= 0 && name[i] != '/' {
		i--
	}
	if i > 0 {
		if err := fs.MkdirAll(name[:i], perm); err != nil {
			return err
		}
	}
	if err := fs.Mkdir(name, perm); err != nil {
		// "dir/." exists although Mkdir cannot create it.
		if node, err1 := fs.walk(name, false); err1 == nil && node.IsDir() {
			return nil
		}
		return err
	}
	return nil
}

// cleanupData recursively cleans up store entries for a node and all its children.
func (fs *FileSystem) cleanupData(node *inode.Inode) {
	if node == nil {
		return
	}

	// If it's a directory, recursively clean up children first
	if node.IsDir() {
		for _, entry := range node.Dir {
			if entry.Name() != ".." && entry.Name() != "." {
				fs.cleanupData(entry.Inode)
			}
		}
	}

	// Clean up the data for this node
	fs.store.Remove(node.Ino)
}

// Remove deletes the named file or empty directory.
//
// Returns an error if the file does not exist, if it is a non-empty
// directory, or if the operation fails. Use RemoveAll to delete
// non-empty directories.
func (fs *FileSystem) Remove(name string) (err error) {
	parent, filename, err := fs.walkParent(name)
	if err != nil {
		return &os.PathError{Op: "remove", Path: name, Err: err}
	}
	if isDotName(filename) {
		return &os.PathError{Op: "remove", Path: name, Err: syscall.EINVAL}
	}
	entry := parent.Lookup(filename)
	if entry == nil {
		return &os.PathError{Op: "remove", Path: name, Err: syscall.ENOENT}
	}
	child := entry.Inode
	if err := fs.slashEntryErr(name, child); err != nil {
		return &os.PathError{Op: "remove", Path: name, Err: err}
	}

	// Directories always contain "." and ".." entries internally,
	// so an empty directory has exactly 2 entries
	if child.IsDir() && len(child.Dir) > 2 {
		return &os.PathError{Op: "remove", Path: name, Err: syscall.ENOTEMPTY}
	}

	// Clean up data before unlinking
	fs.cleanupData(child)
	parent.Unlink(filename)
	return nil
}

// RemoveAll removes the named file or directory and all its contents.
//
// Unlike Remove, RemoveAll will recursively delete directories and their
// contents. Symbolic links are removed, not followed. As with os.RemoveAll,
// a path that does not exist is not an error and yields nil. A path that runs
// through a regular file is an error on Unix (ENOTDIR) but, as with os on
// Windows, counts as not existing there.
func (fs *FileSystem) RemoveAll(name string) error {
	if slashStyle == windowsSlash && removeAllKeepsSlash && hasTrailingSlash(name) {
		// Before Go 1.25, os on Windows cannot remove "file\" and reports
		// why. On Unix, and on Windows since, os.RemoveAll drops the
		// trailing slash.
		if _, err := fs.walk(name, false); err != nil && !errors.Is(err, os.ErrNotExist) {
			return &os.PathError{Op: "remove", Path: name, Err: err}
		}
	}
	parent, filename, err := fs.walkParent(name)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return &os.PathError{Op: "remove", Path: name, Err: err}
	}
	if isDotName(filename) {
		return &os.PathError{Op: "remove", Path: name, Err: syscall.EINVAL}
	}
	entry := parent.Lookup(filename)
	if entry == nil {
		return nil
	}
	child := entry.Inode

	// Clean up data before unlinking
	fs.cleanupData(child)

	child.UnlinkAll()
	parent.Unlink(filename)
	return nil
}

// Chtimes changes the access and modification times of the named file,
// following symbolic links.
func (fs *FileSystem) Chtimes(name string, atime time.Time, mtime time.Time) error {
	node, err := fs.walk(name, true)
	if err != nil {
		return &os.PathError{Op: "chtimes", Path: name, Err: err}
	}

	node.SetAtime(atime)
	node.SetMtime(mtime)
	return nil
}

// Chown changes the owner and group ids of the named file, following
// symbolic links. Use Lchown to change a link itself.
func (fs *FileSystem) Chown(name string, uid, gid int) error {
	node, err := fs.walk(name, true)
	if err != nil {
		return &os.PathError{Op: "chown", Path: name, Err: err}
	}
	node.Uid = uint32(uid)
	node.Gid = uint32(gid)
	return nil
}

// Chmod changes the mode of the named file to mode, following symbolic
// links. As with os.Chmod, only the permission bits and the setuid,
// setgid and sticky bits of mode are used; the file's type is unchanged.
func (fs *FileSystem) Chmod(name string, mode os.FileMode) error {
	node, err := fs.walk(name, true)
	if err != nil {
		return &os.PathError{Op: "chmod", Path: name, Err: err}
	}
	const settable = os.ModePerm | os.ModeSetuid | os.ModeSetgid | os.ModeSticky
	node.Mode = node.Mode&^settable | mode&settable
	return nil
}

// Stat returns file information for the named file, following symbolic links.
//
// If the file is a symbolic link, Stat returns information about the file
// the link points to. Returns an error if the file does not exist, if the
// path runs through something that is not a directory, or if a symbolic
// link loop is detected.
func (fs *FileSystem) Stat(name string) (os.FileInfo, error) {
	node, err := fs.walk(name, true)
	if err != nil {
		return nil, &os.PathError{Op: "stat", Path: name, Err: err}
	}
	return &fileinfo{path.Base(name), node}, nil
}

// Lstat returns file information for the named file without following symbolic links.
//
// Unlike Stat, if the file is a symbolic link, Lstat returns information
// about the link itself. Returns an error if the file does not exist.
func (fs *FileSystem) Lstat(name string) (os.FileInfo, error) {
	node, err := fs.walk(name, false)
	if err != nil {
		return nil, &os.PathError{Op: "lstat", Path: name, Err: err}
	}
	return &fileinfo{path.Base(name), node}, nil
}

// Lchown changes the owner and group of the named file without following symbolic links.
//
// Unlike Chown, if the file is a symbolic link, Lchown changes the ownership
// of the link itself rather than the file it points to.
func (fs *FileSystem) Lchown(name string, uid, gid int) error {
	node, err := fs.walk(name, false)
	if err != nil {
		return &os.PathError{Op: "lchown", Path: name, Err: err}
	}

	node.Uid = uint32(uid)
	node.Gid = uint32(gid)
	return nil
}

// Readlink returns the target of the named symbolic link.
//
// Returns an error if the file does not exist. A file that exists but is
// not a symbolic link yields an empty target and a nil error.
func (fs *FileSystem) Readlink(name string) (string, error) {
	node, err := fs.walk(name, false)
	if err != nil {
		return "", &os.PathError{Op: "readlink", Path: name, Err: err}
	}

	targetVal, ok := fs.symlinks.Load(node.Ino)
	if !ok {
		return "", nil
	}
	return targetVal.(string), nil
}

// Symlink creates a symbolic link at newname pointing to oldname.
//
// The symlink stores oldname exactly as provided (it can be absolute or relative).
// Returns an *os.LinkError if newname already exists, if the parent directory
// of newname does not exist, or if that parent path runs through something
// that is not a directory. Note: Unlike some implementations, the target
// (oldname) does NOT need to exist - broken symlinks are valid.
func (fs *FileSystem) Symlink(oldname, newname string) error {
	linkErr := &os.LinkError{Op: "symlink", Old: oldname, New: newname}
	parent, filename, err := fs.walkParent(newname)
	if err != nil {
		linkErr.Err = err
		return linkErr
	}

	// Symlinks cannot overwrite existing files
	if filename == "" || parent.Lookup(filename) != nil {
		linkErr.Err = fs.existsErr(newname, false)
		return linkErr
	}
	if hasTrailingSlash(newname) {
		// A link is never a directory, so "new/" cannot name one.
		linkErr.Err = errSlashSymlink()
		return linkErr
	}

	// Create symlink inode - symlinks store path as-is, target doesn't need
	// to exist. Link cannot fail: parent is a directory.
	newNode := fs.ino.New(os.ModeSymlink | 0777)
	parent.Link(filename, newNode)
	fs.symlinks.Store(newNode.Ino, oldname)
	return nil
}

// ReadDir reads the named directory and returns a list of directory entries
// sorted by filename. This is compatible with io/fs.ReadDirFS.
func (fs *FileSystem) ReadDir(name string) ([]fs.DirEntry, error) {
	f, err := fs.Open(name)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	// Like os.ReadDir, which opens with O_DIRECTORY, fail at open time.
	if info, err := f.Stat(); err == nil && !info.IsDir() {
		return nil, &os.PathError{Op: "open", Path: name, Err: syscall.ENOTDIR}
	}
	return f.ReadDir(-1)
}

// ReadFile reads the named file and returns its contents.
// This is compatible with io/fs.ReadFileFS.
func (fs *FileSystem) ReadFile(name string) ([]byte, error) {
	f, err := fs.Open(name)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	// Get file size for efficient allocation
	info, err := f.Stat()
	if err != nil {
		return nil, err
	}

	// Read entire file
	data := make([]byte, info.Size())
	n, err := io.ReadFull(f, data)
	if err != nil && err != io.ErrUnexpectedEOF {
		return nil, err
	}

	return data[:n], nil
}

// Sub returns an fs.FS corresponding to the subtree rooted at dir.
// This is compatible with io/fs.SubFS.
func (fs *FileSystem) Sub(dir string) (fs.FS, error) {
	return absfs.FilerToFS(fs, dir)
}

// subFS is a sub-filesystem rooted at a specific directory.
type subFS struct {
	fs   *FileSystem
	root string
}

// joinPath joins the root with a relative path, ensuring we don't escape the subtree.
func (s *subFS) joinPath(name string) string {
	// Clean the name and ensure it doesn't escape
	name = path.Clean("/" + name)
	return path.Join(s.root, name)
}

func (s *subFS) OpenFile(name string, flag int, perm os.FileMode) (absfs.File, error) {
	return s.fs.OpenFile(s.joinPath(name), flag, perm)
}

func (s *subFS) Mkdir(name string, perm os.FileMode) error {
	return s.fs.Mkdir(s.joinPath(name), perm)
}

func (s *subFS) Remove(name string) error {
	return s.fs.Remove(s.joinPath(name))
}

func (s *subFS) Rename(oldpath, newpath string) error {
	return s.fs.Rename(s.joinPath(oldpath), s.joinPath(newpath))
}

func (s *subFS) Stat(name string) (os.FileInfo, error) {
	return s.fs.Stat(s.joinPath(name))
}

func (s *subFS) Chmod(name string, mode os.FileMode) error {
	return s.fs.Chmod(s.joinPath(name), mode)
}

func (s *subFS) Chtimes(name string, atime time.Time, mtime time.Time) error {
	return s.fs.Chtimes(s.joinPath(name), atime, mtime)
}

func (s *subFS) Chown(name string, uid, gid int) error {
	return s.fs.Chown(s.joinPath(name), uid, gid)
}

func (s *subFS) ReadDir(name string) ([]fs.DirEntry, error) {
	return s.fs.ReadDir(s.joinPath(name))
}

func (s *subFS) ReadFile(name string) ([]byte, error) {
	return s.fs.ReadFile(s.joinPath(name))
}

func (s *subFS) Sub(dir string) (fs.FS, error) {
	return absfs.FilerToFS(s, dir)
}
