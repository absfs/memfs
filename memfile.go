package memfs

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"syscall"
	"time"

	"github.com/absfs/absfs"
	"github.com/absfs/inode"
)

type File struct {
	fs *FileSystem

	name  string
	flags int
	node  *inode.Inode
	data  []byte

	offset    int64
	diroffset int
}

func (f *File) Name() string {
	return f.name
}

func (f *File) Read(p []byte) (int, error) {
	// if f == nil {
	// 	panic("nil file handle")
	// }
	if f.flags == 3712 {
		return 0, io.EOF
	}
	if f.node.IsDir() {
		return 0, &os.PathError{Op: "read", Path: f.name, Err: syscall.EISDIR} //os.ErrPermission
	}
	if f.offset >= int64(len(f.data)) {
		return 0, io.EOF
	}
	if f.flags&absfs.O_ACCESS == os.O_WRONLY {
		return 0, &os.PathError{Op: "read", Path: f.name, Err: syscall.EBADF} //os.ErrPermission
	}

	n := copy(p, f.data[f.offset:])
	f.offset += int64(n)
	return n, nil

}

func (f *File) ReadAt(b []byte, off int64) (n int, err error) {
	if f.flags&absfs.O_ACCESS == os.O_WRONLY {
		return 0, os.ErrPermission
	}
	f.offset = off
	return f.Read(b)
}

func (f *File) Write(p []byte) (int, error) {

	if f.flags&absfs.O_ACCESS == os.O_RDONLY {
		return 0, &os.PathError{Op: "write", Path: f.name, Err: syscall.EBADF}
	}
	data := f.data
	size := len(p) + int(f.offset)
	if size > len(data) {
		data = make([]byte, size)
		copy(data, f.data)
	}
	n := copy(data[int(f.offset):], p)
	f.offset += int64(n)
	f.data = data
	return n, nil
}

func (f *File) WriteAt(b []byte, off int64) (n int, err error) {
	f.offset = off
	return f.Write(b)
}

func (f *File) Close() error {
	err := f.Sync()
	if err != nil {
		return err
	}

	f.node = nil
	return nil
}

func (f *File) Seek(offset int64, whence int) (ret int64, err error) {
	switch whence {
	case io.SeekStart:
		f.offset = offset
	case io.SeekCurrent:
		f.offset += offset
	case io.SeekEnd:
		f.offset = int64(len(f.data)) + offset
	}
	if f.offset < 0 {
		f.offset = 0
	}
	return f.offset, nil
}

func (f *File) Stat() (os.FileInfo, error) {
	return &fileinfo{filepath.Base(f.name), f.node}, nil
}

func (f *File) Sync() error {
	if f.flags&absfs.O_ACCESS == os.O_RDONLY {
		return nil
	}
	f.fs.data[int(f.node.Ino)] = f.data

	return nil
}

func (f *File) Readdir(n int) ([]os.FileInfo, error) {
	if f.flags&absfs.O_ACCESS == os.O_WRONLY {
		return nil, os.ErrPermission
	}
	if !f.node.IsDir() {
		return nil, errors.New("not a directory")
	}
	dirs := f.node.Dir
	if f.diroffset >= len(dirs) {
		return nil, io.EOF
	}
	if n < 1 {
		n = len(dirs)
	}
	infos := make([]os.FileInfo, n-f.diroffset)
	for i, entry := range dirs[f.diroffset:n] {
		infos[i] = &fileinfo{entry.Name, entry.Inode}
	}
	f.diroffset += n
	return infos, nil
}

func (f *File) Readdirnames(n int) ([]string, error) {
	var list []string
	if f.flags&absfs.O_ACCESS == os.O_WRONLY {
		return list, os.ErrPermission
	}
	if !f.node.IsDir() {
		return list, errors.New("not a directory")
	}
	dirs := f.node.Dir
	if f.diroffset >= len(dirs) {
		return list, io.EOF
	}
	if n < 1 {
		n = len(dirs)
	}
	list = make([]string, n-f.diroffset)
	for i, entry := range dirs[f.diroffset:n] {
		list[i] = entry.Name
	}
	f.diroffset += n
	return list, nil
}

func (f *File) Truncate(size int64) error {
	if f.flags&absfs.O_ACCESS == os.O_RDONLY {
		return os.ErrPermission
	}
	if int(size) <= len(f.data) {
		f.data = f.data[:int(size)]
		return nil
	}
	data := make([]byte, int(size))
	copy(data, f.data)
	f.data = data
	return nil
}

func (f *File) WriteString(s string) (n int, err error) {
	return f.Write([]byte(s))
}

type fileinfo struct {
	name string
	node *inode.Inode
}

func (i *fileinfo) Name() string {
	return i.name
}

func (i *fileinfo) Size() int64 {
	return i.node.Size
}

func (i *fileinfo) ModTime() time.Time {
	return i.node.Mtime
}

func (i *fileinfo) Mode() os.FileMode {
	return i.node.Mode
}

func (i *fileinfo) Sys() interface{} {
	return i.node
}

func (i *fileinfo) IsDir() bool {
	return i.node.IsDir()
}
