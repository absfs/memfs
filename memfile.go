package memfs

import (
	"errors"
	"io"
	"io/fs"
	"os"
	"path"
	"sort"
	"syscall"
	"time"

	"github.com/absfs/absfs"
	"github.com/absfs/inode"
)

// closedFileSentinel is a sentinel value used to detect operations on closed files.
// Note: This value is currently unused as flags are never set to this value.
const closedFileSentinel = 3712

// errClosed is returned by Readdir and Readdirnames when called on a closed file.
// This matches the error message returned by the os package for these operations.
var errClosed = errors.New("use of closed file")

// File represents an open file in the in-memory file system.
//
// It maintains the file's state including the current read/write offset,
// access flags, and a reference to the underlying inode. File implements
// the absfs.File interface and provides standard file operations.
//
// File operations use the FileSystem's ByteStore for all data access,
// which provides thread-safe read and write operations.
type File struct {
	fs *FileSystem

	name  string
	flags int
	node  *inode.Inode

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
	// Check for sentinel value (currently unused but kept for backwards compatibility)
	if f.flags == closedFileSentinel {
		return 0, io.EOF
	}
	if f.flags&absfs.O_ACCESS == os.O_WRONLY {
		return 0, &os.PathError{Op: "read", Path: f.name, Err: syscall.EBADF}
	}
	if f.node == nil {
		return 0, &os.PathError{Op: "read", Path: f.name, Err: syscall.EBADF}
	}
	if f.node.IsDir() {
		// Check if directory is empty
		size, _ := f.fs.store.Stat(f.node.Ino)
		if size == 0 {
			return 0, &os.PathError{Op: "read", Path: f.name, Err: syscall.EISDIR}
		}
	}

	n, err := f.fs.store.ReadAt(f.node.Ino, p, f.offset)
	f.offset += int64(n)

	// If we got io.EOF but read some data, suppress EOF for this call.
	// Next read will return (0, io.EOF). This matches standard Read() behavior.
	if err == io.EOF && n > 0 {
		return n, nil
	}
	return n, err
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
	if f.node == nil {
		return 0, &os.PathError{Op: "read", Path: f.name, Err: syscall.EBADF}
	}
	return f.fs.store.ReadAt(f.node.Ino, b, off)
}

// Write writes len(p) bytes from p to the file.
//
// Returns the number of bytes written and any error encountered. The file's
// data is automatically expanded if necessary. Returns an error if the file
// was not opened for writing. If the file was opened with O_APPEND, writes
// always append to the end of the file regardless of the current offset.
func (f *File) Write(p []byte) (int, error) {
	if f.flags&absfs.O_ACCESS == os.O_RDONLY {
		return 0, &os.PathError{Op: "write", Path: f.name, Err: syscall.EBADF}
	}
	if f.node == nil {
		return 0, &os.PathError{Op: "write", Path: f.name, Err: syscall.EBADF}
	}

	// O_APPEND: always write at end of file
	writeOffset := f.offset
	if f.flags&os.O_APPEND != 0 {
		writeOffset = f.node.Size
	}

	n, err := f.fs.store.WriteAt(f.node.Ino, p, writeOffset)
	if err != nil {
		return n, err
	}

	// Update offset: for O_APPEND, move to end of written data
	newOffset := writeOffset + int64(n)
	f.offset = newOffset

	// Update inode size if we wrote beyond the current size
	if newOffset > f.node.Size {
		f.node.Size = newOffset
	}

	return n, nil
}

// WriteAt writes len(b) bytes to the file starting at byte offset off.
//
// Returns the number of bytes written and any error encountered. Unlike Write,
// WriteAt does not update the file's current offset.
func (f *File) WriteAt(b []byte, off int64) (n int, err error) {
	if f.flags&absfs.O_ACCESS == os.O_RDONLY {
		return 0, &os.PathError{Op: "write", Path: f.name, Err: syscall.EBADF}
	}
	if f.node == nil {
		return 0, &os.PathError{Op: "write", Path: f.name, Err: syscall.EBADF}
	}

	n, err = f.fs.store.WriteAt(f.node.Ino, b, off)
	if err != nil {
		return n, err
	}

	// Update inode size if we wrote beyond the current size
	newSize := off + int64(n)
	if newSize > f.node.Size {
		f.node.Size = newSize
	}

	return n, nil
}

// Close closes the file, making it unusable for I/O.
//
// This releases the file handle. Subsequent operations on the file will return errors.
// Note: Since we use ByteStore directly, there's no buffering to sync.
func (f *File) Close() error {
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
		size, err := f.fs.store.Stat(f.node.Ino)
		if err != nil {
			return 0, err
		}
		f.offset = size + offset
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
	return &fileinfo{path.Base(f.name), f.node}, nil
}

// Sync commits the current contents of the file to the file system.
//
// Since we use ByteStore directly without buffering, this is a no-op.
// All writes are immediately visible.
func (f *File) Sync() error {
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
		return nil, &os.PathError{Op: "readdir", Path: f.name, Err: errClosed}
	}
	if !f.node.IsDir() {
		return nil, syscall.ENOTDIR
	}

	// Filter out "." and ".." entries to match os package behavior
	var dirs []*inode.DirEntry
	for _, entry := range f.node.Dir {
		if entry.Name() != "." && entry.Name() != ".." {
			dirs = append(dirs, entry)
		}
	}

	// When n <= 0, read all entries and reset offset for next full read
	if n < 1 {
		infos := make([]os.FileInfo, len(dirs))
		for i, entry := range dirs {
			infos[i] = &fileinfo{entry.Name(), entry.Inode}
		}
		f.diroffset = len(dirs)
		return infos, nil
	}

	// Check if we've already read all directory entries (only for n > 0)
	if f.diroffset >= len(dirs) {
		return nil, io.EOF
	}

	// Calculate the end index, capping at the total number of entries.
	// The count represents how many entries we'll actually return.
	end := f.diroffset + n
	if end > len(dirs) {
		end = len(dirs)
	}
	count := end - f.diroffset

	infos := make([]os.FileInfo, count)
	for i, entry := range dirs[f.diroffset:end] {
		infos[i] = &fileinfo{entry.Name(), entry.Inode}
	}
	// Update offset for next read to continue from where we left off
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
		return list, &os.PathError{Op: "readdirnames", Path: f.name, Err: errClosed}
	}
	if !f.node.IsDir() {
		return list, syscall.ENOTDIR
	}

	// Filter out "." and ".." entries to match os package behavior
	var dirs []*inode.DirEntry
	for _, entry := range f.node.Dir {
		if entry.Name() != "." && entry.Name() != ".." {
			dirs = append(dirs, entry)
		}
	}

	// When n <= 0, read all entries and reset offset for next full read
	if n < 1 {
		list = make([]string, len(dirs))
		for i, entry := range dirs {
			list[i] = entry.Name()
		}
		f.diroffset = len(dirs)
		return list, nil
	}

	if f.diroffset >= len(dirs) {
		return list, io.EOF
	}

	// Calculate the number of remaining entries and the end index
	end := f.diroffset + n
	if end > len(dirs) {
		end = len(dirs)
	}
	count := end - f.diroffset

	list = make([]string, count)
	for i, entry := range dirs[f.diroffset:end] {
		list[i] = entry.Name()
	}
	f.diroffset = end
	return list, nil
}

// ReadDir reads the contents of the directory and returns a slice of up to n
// DirEntry values in directory order. This is the modern Go 1.16+ equivalent
// of Readdir that returns lightweight DirEntry values instead of full FileInfo.
//
// If n > 0, ReadDir returns at most n entries. In this case, if ReadDir
// returns an empty slice, it will return a non-nil error explaining why.
// At the end of a directory, the error is io.EOF.
//
// If n <= 0, ReadDir returns all entries from the directory in a single slice.
// In this case, if ReadDir succeeds (reads all the way to the end of the
// directory), it returns the slice and a nil error.
func (f *File) ReadDir(n int) ([]fs.DirEntry, error) {
	if f.node == nil {
		return nil, &os.PathError{Op: "readdir", Path: f.name, Err: syscall.EBADF}
	}
	if !f.node.IsDir() {
		return nil, syscall.ENOTDIR
	}

	// Filter out "." and ".." entries to match os package behavior
	var dirs []*inode.DirEntry
	for _, entry := range f.node.Dir {
		if entry.Name() != "." && entry.Name() != ".." {
			dirs = append(dirs, entry)
		}
	}

	// Sort entries by name
	sort.Slice(dirs, func(i, j int) bool {
		return dirs[i].Name() < dirs[j].Name()
	})

	// When n <= 0, read all remaining entries and return nil error
	if n < 1 {
		// If already at end, return empty slice with nil error (not EOF)
		// This matches the behavior of os.ReadDir
		if f.diroffset >= len(dirs) {
			return nil, nil
		}
		remaining := len(dirs) - f.diroffset
		entries := make([]fs.DirEntry, remaining)
		for i, entry := range dirs[f.diroffset:] {
			entries[i] = &dirEntry{
				name: entry.Name(),
				info: &fileinfo{entry.Name(), entry.Inode},
			}
		}
		f.diroffset = len(dirs)
		return entries, nil
	}

	// For n > 0, check if we've already read all directory entries
	if f.diroffset >= len(dirs) {
		return nil, io.EOF
	}

	// Calculate the end index, capping at the total number of entries
	end := f.diroffset + n
	if end > len(dirs) {
		end = len(dirs)
	}
	count := end - f.diroffset

	entries := make([]fs.DirEntry, count)
	for i, entry := range dirs[f.diroffset:end] {
		entries[i] = &dirEntry{
			name: entry.Name(),
			info: &fileinfo{entry.Name(), entry.Inode},
		}
	}
	// Update offset for next read to continue from where we left off
	f.diroffset = end
	return entries, nil
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
	if f.node == nil {
		return &os.PathError{Op: "truncate", Path: f.name, Err: syscall.EBADF}
	}

	err := f.fs.store.Truncate(f.node.Ino, size)
	if err != nil {
		return err
	}
	f.node.Size = size
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
	return i.node.Mtime()
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

// dirEntry implements fs.DirEntry for directory entries in memfs.
type dirEntry struct {
	name string
	info *fileinfo
}

// Name returns the name of the file (or subdirectory) described by the entry.
func (d *dirEntry) Name() string {
	return d.name
}

// IsDir reports whether the entry describes a directory.
func (d *dirEntry) IsDir() bool {
	return d.info.IsDir()
}

// Type returns the type bits for the entry.
func (d *dirEntry) Type() fs.FileMode {
	return d.info.Mode().Type()
}

// Info returns the FileInfo for the file or subdirectory described by the entry.
func (d *dirEntry) Info() (fs.FileInfo, error) {
	return d.info, nil
}
