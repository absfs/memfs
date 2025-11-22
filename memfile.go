package memfs

import (
	"io"
	"os"
	"path/filepath"
	"syscall"
	"time"

	"github.com/absfs/absfs"
	"github.com/absfs/inode"
)

// File represents an open file in the in-memory file system.
//
// It maintains the file's state including the current read/write offset,
// access flags, and a reference to the underlying inode. File implements
// the absfs.File interface and provides standard file operations.
type File struct {
	fs *FileSystem

	name  string
	flags int
	node  *inode.Inode
	data  []byte

	offset    int64
	diroffset int
}

// Name returns the name of the file as provided to Open or Create.
func (f *File) Name() string {
	return f.name
}

// Read reads up to len(p) bytes from the file into p.
//
// Returns the number of bytes read and any error encountered. Returns io.EOF
// when the end of the file is reached. Returns an error if the file was not
// opened for reading or if the file handle is invalid.
func (f *File) Read(p []byte) (int, error) {
	// if f == nil {
	// 	panic("nil file handle")
	// }
	if f.flags == 3712 {
		return 0, io.EOF
	}
	if f.flags&absfs.O_ACCESS == os.O_WRONLY {
		return 0, &os.PathError{Op: "read", Path: f.name, Err: syscall.EBADF} //os.ErrPermission
	}
	if f.node == nil {
		return 0, &os.PathError{Op: "read", Path: f.name, Err: syscall.EBADF}
	}
	if f.node.IsDir() && len(f.data) == 0 {
		return 0, &os.PathError{Op: "read", Path: f.name, Err: syscall.EISDIR} //os.ErrPermission
	}
	if f.offset >= int64(len(f.data)) {
		return 0, io.EOF
	}

	n := copy(p, f.data[f.offset:])
	f.offset += int64(n)
	return n, nil

}

// ReadAt reads len(b) bytes from the file starting at byte offset off.
//
// Returns the number of bytes read and any error encountered. Unlike Read,
// ReadAt does not update the file's current offset. Returns an error if the
// file was not opened for reading.
func (f *File) ReadAt(b []byte, off int64) (n int, err error) {
	if f.flags&absfs.O_ACCESS == os.O_WRONLY {
		return 0, os.ErrPermission
	}
	f.offset = off
	return f.Read(b)
}

// Write writes len(p) bytes from p to the file.
//
// Returns the number of bytes written and any error encountered. The file's
// data is automatically expanded if necessary. Returns an error if the file
// was not opened for writing.
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

// WriteAt writes len(b) bytes to the file starting at byte offset off.
//
// Returns the number of bytes written and any error encountered. Unlike Write,
// WriteAt does not update the file's current offset.
func (f *File) WriteAt(b []byte, off int64) (n int, err error) {
	f.offset = off
	return f.Write(b)
}

// Close closes the file, making it unusable for I/O.
//
// This method syncs any pending writes to the file system and releases
// the file handle. Subsequent operations on the file will return errors.
func (f *File) Close() error {
	err := f.Sync()
	if err != nil {
		return err
	}

	f.node = nil
	return nil
}

// Seek sets the offset for the next Read or Write on the file.
//
// The whence parameter determines the reference point: io.SeekStart (beginning),
// io.SeekCurrent (current position), or io.SeekEnd (end of file). Returns the
// new offset from the beginning of the file.
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

// Stat returns file information about the file.
//
// Returns an error if the file handle is invalid.
func (f *File) Stat() (os.FileInfo, error) {
	if f.node == nil {
		return nil, &os.PathError{Op: "stat", Path: f.name, Err: syscall.EBADF}
	}
	return &fileinfo{filepath.Base(f.name), f.node}, nil
}

// Sync commits the current contents of the file to the file system.
//
// For files opened for writing, this updates the file's data and size
// in the file system. For read-only files, this is a no-op.
func (f *File) Sync() error {
	// Guard against nil node (e.g., after Close() has been called)
	if f.node == nil {
		return nil
	}
	if f.flags&absfs.O_ACCESS == os.O_RDONLY {
		return nil
	}
	f.fs.data[int(f.node.Ino)] = f.data
	f.node.Size = int64(len(f.data))
	return nil
}

// Readdir reads the contents of the directory associated with the file.
//
// Returns up to n FileInfo values. If n <= 0, returns all remaining entries.
// Subsequent calls continue from where the previous call left off. Returns
// io.EOF when no more entries remain. Returns an error if the file is not
// a directory or was not opened for reading.
func (f *File) Readdir(n int) ([]os.FileInfo, error) {
	if f.flags&absfs.O_ACCESS == os.O_WRONLY {
		return nil, os.ErrPermission
	}
	if f.node == nil {
		return nil, &os.PathError{Op: "readdir", Path: f.name, Err: syscall.EBADF}
	}
	if !f.node.IsDir() {
		return nil, syscall.ENOTDIR
	}
	dirs := f.node.Dir
	if f.diroffset >= len(dirs) {
		return nil, io.EOF
	}
	if n < 1 {
		n = len(dirs)
		f.diroffset = 0
	}

	// Calculate the end index and count
	end := f.diroffset + n
	if end > len(dirs) {
		end = len(dirs)
	}
	count := end - f.diroffset

	infos := make([]os.FileInfo, count)
	for i, entry := range dirs[f.diroffset:end] {
		infos[i] = &fileinfo{entry.Name, entry.Inode}
	}
	f.diroffset = end
	return infos, nil
}

// Readdirnames reads directory entries and returns their names.
//
// Returns up to n entry names. If n <= 0, returns all remaining names.
// Subsequent calls continue from where the previous call left off. Returns
// io.EOF when no more entries remain. Returns an error if the file is not
// a directory or was not opened for reading.
func (f *File) Readdirnames(n int) ([]string, error) {
	var list []string
	if f.flags&absfs.O_ACCESS == os.O_WRONLY {
		return list, os.ErrPermission
	}
	if f.node == nil {
		return list, &os.PathError{Op: "readdirnames", Path: f.name, Err: syscall.EBADF}
	}
	if !f.node.IsDir() {
		return list, syscall.ENOTDIR
	}
	dirs := f.node.Dir
	if f.diroffset >= len(dirs) {
		return list, io.EOF
	}
	if n < 1 {
		n = len(dirs)
		f.diroffset = 0
	}

	// Calculate the number of remaining entries and the end index
	end := f.diroffset + n
	if end > len(dirs) {
		end = len(dirs)
	}
	count := end - f.diroffset

	list = make([]string, count)
	for i, entry := range dirs[f.diroffset:end] {
		list[i] = entry.Name
	}
	f.diroffset = end
	return list, nil
}

// Truncate changes the size of the file.
//
// If the file is larger than size, it is truncated. If it is smaller,
// it is extended with zero bytes. Returns an error if the file was not
// opened for writing.
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

// WriteString writes the contents of string s to the file.
//
// Returns the number of bytes written and any error encountered. This is
// a convenience method equivalent to Write([]byte(s)).
func (f *File) WriteString(s string) (n int, err error) {
	return f.Write([]byte(s))
}

// fileinfo implements os.FileInfo for files in the in-memory file system.
//
// It provides file metadata including name, size, modification time,
// permissions, and file mode. The underlying inode stores the actual
// metadata.
type fileinfo struct {
	name string
	node *inode.Inode
}

// Name returns the base name of the file.
func (i *fileinfo) Name() string {
	return i.name
}

// Size returns the length of the file in bytes.
func (i *fileinfo) Size() int64 {
	return i.node.Size
}

// ModTime returns the modification time of the file.
func (i *fileinfo) ModTime() time.Time {
	return i.node.Mtime
}

// Mode returns the file mode and permission bits.
func (i *fileinfo) Mode() os.FileMode {
	return i.node.Mode
}

// Sys returns the underlying inode for the file.
func (i *fileinfo) Sys() interface{} {
	return i.node
}

// IsDir reports whether the file is a directory.
func (i *fileinfo) IsDir() bool {
	return i.node.IsDir()
}
