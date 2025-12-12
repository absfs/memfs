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
// Thread Safety: FileSystem uses a thread-safe ByteStore for file data
// and a sync.Map for symlinks, making it safe for concurrent use by multiple
// goroutines. The ByteStore handles all file data operations with its own
// internal synchronization.
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
// already exists, or if the operation violates file system constraints.
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

	if !path.IsAbs(oldpath) {
		oldpath = path.Join(fs.cwd, oldpath)
	}

	if !path.IsAbs(newpath) {
		newpath = path.Join(fs.cwd, newpath)
	}
	err := fs.root.Rename(oldpath, newpath)
	if err != nil {
		linkErr.Err = err
		return linkErr
	}
	return nil
}

// Chdir changes the current working directory to the named directory.
//
// The directory must exist and be accessible. Both absolute and relative
// paths are supported. Returns an error if the path does not exist or is
// not a directory.
func (fs *FileSystem) Chdir(name string) (err error) {
	if name == "/" {
		fs.cwd = "/"
		fs.dir = fs.root
		return nil
	}
	wd := fs.root
	cwd := name
	if !path.IsAbs(name) {
		cwd = path.Join(fs.cwd, name)
		wd = fs.dir
	}

	node, err := wd.Resolve(name)
	if err != nil {
		return &os.PathError{Op: "chdir", Path: name, Err: err}
	}
	if !node.IsDir() {
		return &os.PathError{Op: "chdir", Path: name, Err: syscall.ENOTDIR}
	}

	fs.cwd = cwd
	fs.dir = node
	return nil
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

	wd := fs.root
	if !path.IsAbs(name) {
		wd = fs.dir
	}

	// First check if the file exists (without following symlinks for the final component)
	var exists bool
	node, err := wd.Resolve(name)
	if err == nil {
		exists = true
		// Follow symlinks for file operations
		if node.Mode&os.ModeSymlink != 0 {
			resolvedNode, err := fs.fileStat(fs.cwd, name)
			if err != nil {
				// Symlink exists but target doesn't - treat as not found unless creating
				if flag&os.O_CREATE == 0 {
					return &absfs.InvalidFile{Path: name}, err
				}
				// For O_CREATE with broken symlink, we can't create a file through a broken symlink
				return &absfs.InvalidFile{Path: name}, err
			}
			node = resolvedNode
		}
	}

	dir, filename := path.Split(name)
	dir = path.Clean(dir)
	parent, err := wd.Resolve(dir)
	if err != nil {
		return nil, err
	}

	access := flag & absfs.O_ACCESS
	create := flag&os.O_CREATE != 0
	truncate := flag&os.O_TRUNC != 0

	// error if it does not exist, and we are not allowed to create it.
	if !exists && !create {
		return &absfs.InvalidFile{Path: name}, &os.PathError{Op: "open", Path: name, Err: syscall.ENOENT}
	}
	if exists {
		// err if exclusive create is required
		if create && flag&os.O_EXCL != 0 {
			return &absfs.InvalidFile{Path: name}, &os.PathError{Op: "open", Path: name, Err: syscall.EEXIST}
		}
		if node.IsDir() {
			if access != os.O_RDONLY || truncate {
				return &absfs.InvalidFile{Path: name}, &os.PathError{Op: "open", Path: name, Err: syscall.EISDIR} // os.ErrNotExist}
			}
		}

		// if we must truncate the file
		if truncate {
			fs.store.Truncate(node.Ino, 0)
			node.Size = 0
		}

	} else { // !exists
		// error if we cannot create the file
		if !create {
			return &absfs.InvalidFile{Path: name}, &os.PathError{Op: "open", Path: name, Err: syscall.ENOENT} //os.ErrNotExist}
		}

		// Create write-able file
		node = fs.ino.New(fs.Umask & perm)
		err := parent.Link(filename, node)
		if err != nil {
			return &absfs.InvalidFile{Path: name}, &os.PathError{Op: "open", Path: name, Err: err}
		}
		// No need to initialize store - it handles non-existent files gracefully
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

// Truncate changes the size of the named file.
//
// If the file is larger than size, it is truncated. If it is smaller,
// it is extended with zero bytes. Returns an error if the file does not
// exist.
func (fs *FileSystem) Truncate(name string, size int64) error {
	path := inode.Abs(fs.cwd, name)
	child, err := fs.root.Resolve(path)
	if err != nil {
		return err
	}

	err = fs.store.Truncate(child.Ino, size)
	if err != nil {
		return err
	}
	child.Size = size
	return nil
}

// Mkdir creates a new directory with the specified name and permissions.
//
// The parent directory must already exist. Returns an error if the directory
// already exists, the parent directory does not exist, or if the operation
// fails.
func (fs *FileSystem) Mkdir(name string, perm os.FileMode) error {
	wd := fs.root
	abs := name
	if !path.IsAbs(abs) {
		abs = path.Join(fs.cwd, abs)
		wd = fs.dir
	}
	_, err := wd.Resolve(name)
	if err == nil {
		return &os.PathError{Op: "mkdir", Path: name, Err: syscall.EEXIST}
	}

	parent := fs.root
	dir, filename := path.Split(abs)
	dir = path.Clean(dir)
	if dir != "/" {
		parent, err = fs.root.Resolve(strings.TrimLeft(dir, "/"))
		if err != nil {
			return &os.PathError{Op: "mkdir", Path: dir, Err: err}
		}
	}

	child := fs.ino.NewDir(fs.Umask & perm)
	parent.Link(filename, child)
	child.Link("..", parent)
	// No need to initialize store - it handles non-existent files gracefully
	return nil
}

// MkdirAll creates a directory and all necessary parent directories.
//
// If the directory already exists, MkdirAll does nothing and returns nil.
// This is similar to the 'mkdir -p' command in Unix.
func (fs *FileSystem) MkdirAll(name string, perm os.FileMode) error {
	name = inode.Abs(fs.cwd, name)
	dirPath := ""
	for _, p := range strings.Split(name, "/") {
		if p == "" {
			p = "/"
		}
		dirPath = path.Join(dirPath, p)
		fs.Mkdir(dirPath, perm)
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
	wd := fs.root
	abs := name
	if !path.IsAbs(abs) {
		abs = path.Join(fs.cwd, abs)
		wd = fs.dir
	}
	child, err := wd.Resolve(name)
	if err != nil {
		return &os.PathError{Op: "remove", Path: name, Err: err}
	}

	if child.IsDir() {
		// Directories always contain "." and ".." entries internally,
		// so an empty directory has exactly 2 entries
		if len(child.Dir) > 2 {
			return &os.PathError{Op: "remove", Path: name, Err: syscall.ENOTEMPTY}
		}
	}

	parent := fs.root
	dir, filename := path.Split(abs)
	dir = path.Clean(dir)
	if dir != "/" {
		parent, err = fs.root.Resolve(strings.TrimLeft(dir, "/"))
		if err != nil {
			return &os.PathError{Op: "remove", Path: dir, Err: err}
		}
	}

	// Clean up data before unlinking
	fs.cleanupData(child)

	return parent.Unlink(filename)
}

// RemoveAll removes the named file or directory and all its contents.
//
// Unlike Remove, RemoveAll will recursively delete directories and their
// contents. Returns an error if the file does not exist or if the operation
// fails.
func (fs *FileSystem) RemoveAll(name string) error {
	wd := fs.root
	abs := name
	if !path.IsAbs(abs) {
		abs = path.Join(fs.cwd, abs)
		wd = fs.dir
	}
	child, err := wd.Resolve(name)
	if err != nil {
		return &os.PathError{Op: "remove", Path: name, Err: err}
	}

	parent := fs.root
	dir, filename := path.Split(abs)
	dir = path.Clean(dir)
	if dir != "/" {
		parent, err = fs.root.Resolve(strings.TrimLeft(dir, "/"))
		if err != nil {
			return &os.PathError{Op: "remove", Path: dir, Err: err}
		}
	}

	// Clean up data before unlinking
	fs.cleanupData(child)

	child.UnlinkAll()
	return parent.Unlink(filename)
}

//Chtimes changes the access and modification times of the named file
func (fs *FileSystem) Chtimes(name string, atime time.Time, mtime time.Time) error {
	var err error
	node := fs.root

	name = inode.Abs(fs.cwd, name)
	if name != "/" {
		node, err = fs.root.Resolve(strings.TrimLeft(name, "/"))
		if err != nil {
			return err
		}
	}

	node.SetAtime(atime)
	node.SetMtime(mtime)
	return nil
}

//Chown changes the owner and group ids of the named file
func (fs *FileSystem) Chown(name string, uid, gid int) error {
	var err error
	node := fs.root

	name = inode.Abs(fs.cwd, name)
	if name != "/" {
		node, err = fs.root.Resolve(name)
		if err != nil {
			return err
		}
	}
	node.Uid = uint32(uid)
	node.Gid = uint32(gid)
	return nil
}

//Chmod changes the mode of the named file to mode.
func (fs *FileSystem) Chmod(name string, mode os.FileMode) error {
	var err error
	node := fs.root

	name = inode.Abs(fs.cwd, name)

	if name != "/" {
		node, err = fs.root.Resolve(strings.TrimLeft(name, "/"))
		if err != nil {
			return err
		}
	}
	node.Mode = mode
	return nil
}

// fileStat resolves symlinks and returns the final inode, with cycle detection
func (fs *FileSystem) fileStat(cwd, name string) (*inode.Inode, error) {
	// Initialize the visited map to track inodes we've seen during symlink traversal.
	// This prevents infinite loops when symlinks form a cycle (e.g., a -> b -> a).
	visited := make(map[uint64]bool)
	return fs.fileStatWithVisited(cwd, name, visited)
}

// fileStatWithVisited is the internal implementation with cycle detection.
// It recursively follows symlinks until reaching a non-symlink inode, tracking
// visited inodes to detect and prevent infinite loops.
func (fs *FileSystem) fileStatWithVisited(cwd, name string, visited map[uint64]bool) (*inode.Inode, error) {
	name = inode.Abs(cwd, name)
	node, err := fs.root.Resolve(strings.TrimLeft(name, "/"))
	if err != nil {
		return nil, &os.PathError{Op: "stat", Path: name, Err: err}
	}

	// If this is not a symlink, we've found the final target - return it
	if node.Mode&os.ModeSymlink == 0 {
		return node, nil
	}

	// Detect symlink cycles: if we've already visited this inode during the current
	// resolution chain, we have a loop (e.g., link1 -> link2 -> link1)
	if visited[node.Ino] {
		return nil, &os.PathError{Op: "stat", Path: name, Err: syscall.ELOOP}
	}

	// Mark this inode as visited to detect cycles in subsequent recursive calls
	visited[node.Ino] = true

	// Recursively resolve the symlink target. The target path is stored in fs.symlinks,
	// and we resolve it relative to the symlink's directory (not the original cwd).
	targetVal, ok := fs.symlinks.Load(node.Ino)
	if !ok {
		return nil, &os.PathError{Op: "stat", Path: name, Err: syscall.EINVAL}
	}
	target := targetVal.(string)
	return fs.fileStatWithVisited(path.Dir(name), target, visited)
}

// Stat returns file information for the named file, following symbolic links.
//
// If the file is a symbolic link, Stat returns information about the file
// the link points to. Returns an error if the file does not exist or if
// a symbolic link loop is detected.
func (fs *FileSystem) Stat(name string) (os.FileInfo, error) {
	if name == "/" {
		return &fileinfo{"/", fs.root}, nil
	}
	node, err := fs.fileStat(fs.cwd, name)
	return &fileinfo{path.Base(name), node}, err
}

// Lstat returns file information for the named file without following symbolic links.
//
// Unlike Stat, if the file is a symbolic link, Lstat returns information
// about the link itself. Returns an error if the file does not exist.
func (fs *FileSystem) Lstat(name string) (os.FileInfo, error) {
	if name == "/" {
		return &fileinfo{"/", fs.root}, nil
	}
	name = inode.Abs(fs.cwd, name)
	node, err := fs.root.Resolve(strings.TrimLeft(name, "/"))
	if err != nil {
		return nil, &os.PathError{Op: "remove", Path: name, Err: err}
	}

	return &fileinfo{path.Base(name), node}, nil
}

// Lchown changes the owner and group of the named file without following symbolic links.
//
// Unlike Chown, if the file is a symbolic link, Lchown changes the ownership
// of the link itself rather than the file it points to.
func (fs *FileSystem) Lchown(name string, uid, gid int) error {
	if name == "/" {
		fs.root.Uid = uint32(uid)
		fs.root.Gid = uint32(gid)
		return nil
	}
	name = inode.Abs(fs.cwd, name)
	node, err := fs.root.Resolve(strings.TrimLeft(name, "/"))
	if err != nil {
		return err
	}

	node.Uid = uint32(uid)
	node.Gid = uint32(gid)
	return nil
}

// Readlink returns the target of the named symbolic link.
//
// Returns an error if the file does not exist or is not a symbolic link.
func (fs *FileSystem) Readlink(name string) (string, error) {
	var ino uint64
	if name == "/" {
		ino = fs.root.Ino
	} else {
		node, err := fs.root.Resolve(strings.TrimLeft(name, "/"))
		if err != nil {
			return "", err
		}
		ino = node.Ino
	}

	targetVal, ok := fs.symlinks.Load(ino)
	if !ok {
		return "", nil
	}
	return targetVal.(string), nil
}

// Symlink creates a symbolic link at newname pointing to oldname.
//
// The symlink stores oldname exactly as provided (it can be absolute or relative).
// Returns an error if newname already exists or if the parent directory of
// newname does not exist. Note: Unlike some implementations, the target (oldname)
// does NOT need to exist - broken symlinks are valid.
func (fs *FileSystem) Symlink(oldname, newname string) error {
	wd := fs.root
	abs := newname
	if !path.IsAbs(newname) {
		abs = path.Join(fs.cwd, newname)
		wd = fs.dir
	}

	// Check if newname already exists - symlinks cannot overwrite existing files
	_, err := wd.Resolve(newname)
	if err == nil {
		return &os.PathError{Op: "symlink", Path: newname, Err: syscall.EEXIST}
	}

	// Resolve parent directory
	dir, filename := path.Split(abs)
	dir = path.Clean(dir)
	parent := fs.root
	if dir != "/" {
		parent, err = fs.root.Resolve(strings.TrimLeft(dir, "/"))
		if err != nil {
			return &os.PathError{Op: "symlink", Path: newname, Err: err}
		}
	}

	// Create symlink inode - symlinks store path as-is, target doesn't need to exist
	newNode := fs.ino.New(os.ModeSymlink | 0777)

	err = parent.Link(filename, newNode)
	if err != nil {
		return &os.PathError{Op: "symlink", Path: newname, Err: err}
	}
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
