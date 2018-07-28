package memfs

import (
	"errors"
	"os"
	filepath "path" // force forward slash separators on all OSs.
	"strings"
	"syscall"
	"time"

	"github.com/absfs/absfs"
)

type FileSystem struct {
	Umask   os.FileMode
	Tempdir string

	root *inode
	cwd  string
	dir  *inode
	ino  *iNo
}

func NewFS() (*FileSystem, error) {
	fs := new(FileSystem)
	fs.ino = new(iNo)
	fs.Tempdir = "/tmp"

	fs.Umask = 0755
	fs.root = fs.ino.newDir(fs.Umask)
	fs.cwd = "/"
	fs.dir = fs.root
	return fs, nil
}

func (fs *FileSystem) Separator() uint8 {
	return '/'
}

func (fs *FileSystem) ListSeparator() uint8 {
	return ':'
}

func (fs *FileSystem) Chdir(name string) (err error) {
	if name == "/" {
		fs.cwd = "/"
		fs.dir = fs.root
		return nil
	}
	wd := fs.root
	if !filepath.IsAbs(name) {
		wd = fs.dir
	}

	node, err := wd.resolve(name)
	if err != nil {
		return &os.PathError{Op: "chdir", Path: name, Err: err}
	}
	if !node.IsDir() {
		return &os.PathError{Op: "chdir", Path: name, Err: errors.New("not a directory")}
	}

	fs.cwd = name
	fs.dir = node
	return nil
}

func (fs *FileSystem) Getwd() (dir string, err error) {
	return fs.cwd, nil
}

func (fs *FileSystem) TempDir() string {
	return fs.Tempdir
}

func (fs *FileSystem) Open(name string) (absfs.File, error) {
	return fs.OpenFile(name, os.O_RDONLY, 0)
}

func (fs *FileSystem) Create(name string) (absfs.File, error) {
	return fs.OpenFile(name, os.O_CREATE|os.O_RDWR|os.O_TRUNC, 0777)
}

func (fs *FileSystem) OpenFile(name string, flag int, perm os.FileMode) (absfs.File, error) {
	if name == "/" {
		return &File{fs: fs, name: name, flags: flag, node: fs.root}, nil
	}
	if name == "." {
		return &File{fs: fs, name: name, flags: flag, node: fs.dir}, nil
	}

	wd := fs.root
	if !filepath.IsAbs(name) {
		wd = fs.dir
	}
	var exists bool
	node, err := wd.resolve(name)
	if err == nil {
		exists = true
	}

	dir, filename := filepath.Split(name)
	dir = filepath.Clean(dir)
	parent, err := wd.resolve(dir)
	if err != nil {
		exists = false
	}
	access := flag & absfs.O_ACCESS
	create := flag&os.O_CREATE != 0
	truncate := flag&os.O_TRUNC != 0

	// error if it does not exist, and we are not allowed to create it.
	if !exists && !create {
		return &absfs.InvalidFile{name}, &os.PathError{Op: "open", Path: name, Err: syscall.ENOENT}
	}

	if exists {

		// err if exclusive create is required
		if create && flag&os.O_EXCL != 0 {
			return &absfs.InvalidFile{name}, &os.PathError{Op: "open", Path: name, Err: syscall.EEXIST}
		}
		if node.IsDir() {
			if access != os.O_RDONLY || truncate {
				return &absfs.InvalidFile{name}, &os.PathError{Op: "open", Path: name, Err: syscall.EISDIR} // os.ErrNotExist}
			}
		}

		// if we must truncate the file
		if truncate {
			node.Data = node.Data[:0]
		}

	} else { // !exists
		// error if we cannot create the file
		if !create {
			return &absfs.InvalidFile{name}, &os.PathError{Op: "open", Path: name, Err: syscall.ENOENT} //os.ErrNotExist}
		}

		// Create write-able file
		node = fs.ino.newFile(fs.Umask & perm)
		err := parent.Link(filename, node)
		if err != nil {
			return &absfs.InvalidFile{name}, &os.PathError{Op: "open", Path: name, Err: err}
		}

	}

	if !create {
		if access == os.O_RDONLY && node.Mode&absfs.OS_ALL_R == 0 ||
			access == os.O_WRONLY && node.Mode&absfs.OS_ALL_W == 0 ||
			access == os.O_RDWR && node.Mode&(absfs.OS_ALL_W|absfs.OS_ALL_R) == 0 {
			return &absfs.InvalidFile{name}, &os.PathError{Op: "open", Path: name, Err: os.ErrPermission}
		}
	}
	return &File{fs: fs, name: name, flags: flag, node: node}, nil
}

func (fs *FileSystem) Truncate(name string, size int64) error {
	path := abs(fs.cwd, name)
	child, err := fs.root.resolve(path)
	if err != nil {
		return err
	}

	if size <= int64(len(child.Data)) {
		child.Data = child.Data[:int(size)]
		return nil
	}
	data := make([]byte, int(size))
	copy(data, child.Data)
	child.Data = data
	return nil
}

func (fs *FileSystem) Mkdir(name string, perm os.FileMode) error {
	wd := fs.root
	if !filepath.IsAbs(name) {
		wd = fs.dir
	}
	_, err := wd.resolve(name)
	if err == nil {
		return &os.PathError{Op: "mkdir", Path: name, Err: os.ErrExist}
	}

	dir, filename := filepath.Split(name)
	dir = filepath.Clean(dir)
	parent, err := wd.resolve(dir)
	if err != nil {
		return &os.PathError{Op: "mkdir", Path: dir, Err: err}
	}

	child := fs.ino.newDir(fs.Umask & perm)
	parent.Link(filename, child)
	child.Link("..", parent)
	return nil
}

func (fs *FileSystem) MkdirAll(name string, perm os.FileMode) error {
	name = abs(fs.cwd, name)
	path := ""
	for _, p := range strings.Split(name, string(fs.Separator())) {
		if p == "" {
			p = "/"
		}
		path = filepath.Join(path, p)
		fs.Mkdir(path, perm)
	}
	return nil
}

func (fs *FileSystem) Remove(name string) (err error) {
	path := abs(fs.cwd, name)
	parent := fs.root
	dir := filepath.Dir(path)
	if dir != "/" {
		parent, err = fs.root.resolve(strings.TrimLeft(dir, "/"))
		if err != nil {
			return &os.PathError{Op: "remove", Path: dir, Err: err}
		}
	}
	return parent.Unlink(filepath.Base(path))
}

func (fs *FileSystem) RemoveAll(name string) error {
	path := abs(fs.cwd, name)

	if path == "/" {
		fs.root.UnlinkAll()
		return nil
	}
	node, err := fs.root.resolve(strings.TrimLeft(path, "/"))

	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return &os.PathError{Op: "removeall", Path: path, Err: err}
	}

	node.UnlinkAll()
	return nil
}

func (fs *FileSystem) Stat(name string) (os.FileInfo, error) {
	name = abs(fs.cwd, name)

	if name == "/" {
		return &fileinfo{"/", fs.root}, nil
	}
	node, err := fs.root.resolve(strings.TrimLeft(name, "/"))
	if err != nil {
		return nil, &os.PathError{Op: "remove", Path: name, Err: err}
	}
	return &fileinfo{filepath.Base(name), node}, nil
}

//Chmod changes the mode of the named file to mode.
func (fs *FileSystem) Chmod(name string, mode os.FileMode) error {
	name = abs(fs.cwd, name)
	node, err := fs.root.resolve(strings.TrimLeft(name, "/"))
	if err != nil {
		return err
	}
	node.Mode = mode
	return nil
}

//Chtimes changes the access and modification times of the named file
func (fs *FileSystem) Chtimes(name string, atime time.Time, mtime time.Time) error {
	name = abs(fs.cwd, name)
	node, err := fs.root.resolve(strings.TrimLeft(name, "/"))
	if err != nil {
		return err
	}
	node.Atime = atime
	node.Mtime = mtime
	return nil
}

//Chown changes the owner and group ids of the named file
func (fs *FileSystem) Chown(name string, uid, gid int) error {
	name = abs(fs.cwd, name)
	node, err := fs.root.resolve(name)
	if err != nil {
		return err
	}
	node.Uid = uint32(uid)
	node.Gid = uint32(gid)
	return nil
}
