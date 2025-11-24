package memfs

import (
	"errors"
	"io"
	"os"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/absfs/inode"
	"github.com/absfs/lockfs"
)

// TestReadAt tests the ReadAt function
func TestReadAt(t *testing.T) {
	fs, err := NewFS()
	if err != nil {
		t.Fatal(err)
	}

	// Create a file with some data
	f, err := fs.Create("/test.txt")
	if err != nil {
		t.Fatal(err)
	}

	data := []byte("Hello, World! This is a test file.")
	_, err = f.Write(data)
	if err != nil {
		t.Fatal(err)
	}
	f.Close()

	// Open for reading
	f, err = fs.Open("/test.txt")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	// Test ReadAt
	buf := make([]byte, 5)
	n, err := f.ReadAt(buf, 7)
	if err != nil {
		t.Fatal(err)
	}
	if n != 5 {
		t.Errorf("Expected to read 5 bytes, got %d", n)
	}
	if string(buf) != "World" {
		t.Errorf("Expected 'World', got '%s'", string(buf))
	}

	// Test ReadAt with write-only file
	wf, err := fs.OpenFile("/test2.txt", os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		t.Fatal(err)
	}
	defer wf.Close()

	_, err = wf.ReadAt(buf, 0)
	if err != os.ErrPermission {
		t.Errorf("Expected ErrPermission for write-only file, got %v", err)
	}
}

// TestWriteAt tests the WriteAt function
func TestWriteAt(t *testing.T) {
	fs, err := NewFS()
	if err != nil {
		t.Fatal(err)
	}

	// Create a file
	f, err := fs.Create("/test.txt")
	if err != nil {
		t.Fatal(err)
	}

	// Write initial data
	initial := []byte("0000000000")
	_, err = f.Write(initial)
	if err != nil {
		t.Fatal(err)
	}

	// Use WriteAt to write at offset
	data := []byte("TEST")
	n, err := f.WriteAt(data, 3)
	if err != nil {
		t.Fatal(err)
	}
	if n != 4 {
		t.Errorf("Expected to write 4 bytes, got %d", n)
	}

	// Close and reopen to verify
	f.Close()
	f, err = fs.Open("/test.txt")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	result := make([]byte, 10)
	f.Read(result)
	if string(result) != "000TEST000" {
		t.Errorf("Expected '000TEST000', got '%s'", string(result))
	}
}

// TestFileStat tests the Stat method on File
func TestFileStat(t *testing.T) {
	fs, err := NewFS()
	if err != nil {
		t.Fatal(err)
	}

	// Create a file
	f, err := fs.Create("/test.txt")
	if err != nil {
		t.Fatal(err)
	}
	f.Write([]byte("test data"))

	// Test Stat on open file
	info, err := f.Stat()
	if err != nil {
		t.Fatal(err)
	}
	if info.Name() != "test.txt" {
		t.Errorf("Expected name 'test.txt', got '%s'", info.Name())
	}
	f.Close()

	// Test Stat on closed file (node is nil)
	_, err = f.Stat()
	if err == nil {
		t.Error("Expected error for Stat on closed file")
	}
	pathErr, ok := err.(*os.PathError)
	if !ok {
		t.Errorf("Expected *os.PathError, got %T", err)
	} else if pathErr.Err != syscall.EBADF {
		t.Errorf("Expected EBADF error, got %v", pathErr.Err)
	}
}

// TestFileTruncate tests the Truncate method on File
func TestFileTruncate(t *testing.T) {
	fs, err := NewFS()
	if err != nil {
		t.Fatal(err)
	}

	// Create a file with data
	f, err := fs.Create("/test.txt")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	data := []byte("Hello, World!")
	f.Write(data)

	// Truncate to smaller size
	err = f.Truncate(5)
	if err != nil {
		t.Fatal(err)
	}
	f.Sync()
	f.Close()

	// Verify truncation
	f, err = fs.Open("/test.txt")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	buf := make([]byte, 20)
	n, _ := f.Read(buf)
	if n != 5 || string(buf[:n]) != "Hello" {
		t.Errorf("Expected 'Hello', got '%s'", string(buf[:n]))
	}

	// Test truncate to larger size
	f2, err := fs.OpenFile("/test2.txt", os.O_CREATE|os.O_RDWR, 0644)
	if err != nil {
		t.Fatal(err)
	}
	defer f2.Close()

	f2.Write([]byte("Hi"))
	err = f2.Truncate(10)
	if err != nil {
		t.Fatal(err)
	}
	f2.Sync()
	f2.Close()

	info, _ := fs.Stat("/test2.txt")
	if info.Size() != 10 {
		t.Errorf("Expected size 10, got %d", info.Size())
	}

	// Test truncate on read-only file
	rf, err := fs.Open("/test.txt")
	if err != nil {
		t.Fatal(err)
	}
	defer rf.Close()

	err = rf.Truncate(1)
	if err != os.ErrPermission {
		t.Errorf("Expected ErrPermission for read-only file, got %v", err)
	}
}

// TestModTime tests the ModTime method
func TestModTime(t *testing.T) {
	fs, err := NewFS()
	if err != nil {
		t.Fatal(err)
	}

	f, err := fs.Create("/test.txt")
	if err != nil {
		t.Fatal(err)
	}
	f.Close()

	info, err := fs.Stat("/test.txt")
	if err != nil {
		t.Fatal(err)
	}

	modTime := info.ModTime()
	if modTime.IsZero() {
		t.Error("ModTime should not be zero")
	}
}

// TestListSeparator tests the ListSeparator method
func TestListSeparator(t *testing.T) {
	fs, err := NewFS()
	if err != nil {
		t.Fatal(err)
	}

	sep := fs.ListSeparator()
	if sep != ':' {
		t.Errorf("Expected ':', got '%c'", sep)
	}
}

// TestRename tests the Rename method
func TestRename(t *testing.T) {
	fs, err := NewFS()
	if err != nil {
		t.Fatal(err)
	}

	// Create a file
	f, err := fs.Create("/old.txt")
	if err != nil {
		t.Fatal(err)
	}
	f.Write([]byte("test data"))
	f.Close()

	// Test successful rename
	err = fs.Rename("/old.txt", "/new.txt")
	if err != nil {
		t.Fatal(err)
	}

	// Verify old file doesn't exist
	_, err = fs.Stat("/old.txt")
	if err == nil {
		t.Error("Old file should not exist")
	}

	// Verify new file exists
	info, err := fs.Stat("/new.txt")
	if err != nil {
		t.Fatal(err)
	}
	if info.Size() != 9 {
		t.Errorf("Expected size 9, got %d", info.Size())
	}

	// Test rename with relative paths
	fs.Chdir("/")
	fs.Mkdir("/dir", 0755)
	err = fs.Rename("new.txt", "dir/renamed.txt")
	if err != nil {
		t.Fatal(err)
	}

	// Test rename of root (should fail)
	err = fs.Rename("/", "/newroot")
	if err == nil {
		t.Error("Should not be able to rename root")
	}

	// Test rename of non-existent file
	err = fs.Rename("/nonexistent.txt", "/other.txt")
	if err == nil {
		t.Error("Should fail to rename non-existent file")
	}
}

// TestFileSystemTruncate tests the Truncate method on FileSystem
func TestFileSystemTruncate(t *testing.T) {
	fs, err := NewFS()
	if err != nil {
		t.Fatal(err)
	}

	// Create a file
	f, err := fs.Create("/test.txt")
	if err != nil {
		t.Fatal(err)
	}
	f.Write([]byte("Hello, World!"))
	f.Close()

	// Truncate to smaller size
	err = fs.Truncate("/test.txt", 5)
	if err != nil {
		t.Fatal(err)
	}

	info, err := fs.Stat("/test.txt")
	if err != nil {
		t.Fatal(err)
	}
	if info.Size() != 5 {
		t.Errorf("Expected size 5, got %d", info.Size())
	}

	// Truncate to larger size
	err = fs.Truncate("/test.txt", 20)
	if err != nil {
		t.Fatal(err)
	}

	info, err = fs.Stat("/test.txt")
	if err != nil {
		t.Fatal(err)
	}
	if info.Size() != 20 {
		t.Errorf("Expected size 20, got %d", info.Size())
	}

	// Test with relative path
	fs.Chdir("/")
	fs.Create("/test2.txt")
	err = fs.Truncate("test2.txt", 10)
	if err != nil {
		t.Fatal(err)
	}

	// Test truncate non-existent file
	err = fs.Truncate("/nonexistent.txt", 10)
	if err == nil {
		t.Error("Should fail to truncate non-existent file")
	}
}

// TestChtimes tests the Chtimes method
func TestChtimes(t *testing.T) {
	fs, err := NewFS()
	if err != nil {
		t.Fatal(err)
	}

	// Create a file
	f, err := fs.Create("/test.txt")
	if err != nil {
		t.Fatal(err)
	}
	f.Close()

	// Set times
	atime := time.Now().Add(-1 * time.Hour)
	mtime := time.Now().Add(-2 * time.Hour)

	err = fs.Chtimes("/test.txt", atime, mtime)
	if err != nil {
		t.Fatal(err)
	}

	info, err := fs.Stat("/test.txt")
	if err != nil {
		t.Fatal(err)
	}

	if !info.ModTime().Equal(mtime) {
		t.Errorf("Expected mtime %v, got %v", mtime, info.ModTime())
	}

	// Test with root
	err = fs.Chtimes("/", atime, mtime)
	if err != nil {
		t.Fatal(err)
	}

	// Test with non-existent file
	err = fs.Chtimes("/nonexistent.txt", atime, mtime)
	if err == nil {
		t.Error("Should fail for non-existent file")
	}
}

// TestChown tests the Chown method
func TestChown(t *testing.T) {
	fs, err := NewFS()
	if err != nil {
		t.Fatal(err)
	}

	// Create a file
	f, err := fs.Create("/test.txt")
	if err != nil {
		t.Fatal(err)
	}
	f.Close()

	// Change ownership
	err = fs.Chown("/test.txt", 1000, 1000)
	if err != nil {
		t.Fatal(err)
	}

	info, err := fs.Stat("/test.txt")
	if err != nil {
		t.Fatal(err)
	}

	node := info.Sys().(*inode.Inode)
	if node.Uid != 1000 || node.Gid != 1000 {
		t.Errorf("Expected uid=1000, gid=1000, got uid=%d, gid=%d", node.Uid, node.Gid)
	}

	// Test with root
	err = fs.Chown("/", 2000, 2000)
	if err != nil {
		t.Fatal(err)
	}

	// Test with non-existent file
	err = fs.Chown("/nonexistent.txt", 1000, 1000)
	if err == nil {
		t.Error("Should fail for non-existent file")
	}
}

// TestLstat tests the Lstat method
func TestLstat(t *testing.T) {
	fs, err := NewFS()
	if err != nil {
		t.Fatal(err)
	}

	// Create a file and a symlink
	f, err := fs.Create("/real.txt")
	if err != nil {
		t.Fatal(err)
	}
	f.Write([]byte("real data"))
	f.Close()

	err = fs.Symlink("/real.txt", "/link.txt")
	if err != nil {
		t.Fatal(err)
	}

	// Lstat should return info about the link itself
	info, err := fs.Lstat("/link.txt")
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode()&os.ModeSymlink == 0 {
		t.Error("Expected symlink mode")
	}

	// Stat should follow the link
	info, err = fs.Stat("/link.txt")
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		t.Error("Stat should follow symlink")
	}

	// Test Lstat on root
	info, err = fs.Lstat("/")
	if err != nil {
		t.Fatal(err)
	}
	if info.Name() != "/" {
		t.Errorf("Expected name '/', got '%s'", info.Name())
	}

	// Test Lstat on non-existent file
	_, err = fs.Lstat("/nonexistent.txt")
	if err == nil {
		t.Error("Should fail for non-existent file")
	}
}

// TestLchown tests the Lchown method
func TestLchown(t *testing.T) {
	fs, err := NewFS()
	if err != nil {
		t.Fatal(err)
	}

	// Create a file and a symlink
	f, err := fs.Create("/real.txt")
	if err != nil {
		t.Fatal(err)
	}
	f.Close()

	err = fs.Symlink("/real.txt", "/link.txt")
	if err != nil {
		t.Fatal(err)
	}

	// Lchown should change the link's ownership, not the target
	err = fs.Lchown("/link.txt", 3000, 3000)
	if err != nil {
		t.Fatal(err)
	}

	linkInfo, err := fs.Lstat("/link.txt")
	if err != nil {
		t.Fatal(err)
	}
	linkNode := linkInfo.Sys().(*inode.Inode)
	if linkNode.Uid != 3000 || linkNode.Gid != 3000 {
		t.Errorf("Expected link uid=3000, gid=3000, got uid=%d, gid=%d", linkNode.Uid, linkNode.Gid)
	}

	// Test Lchown on root
	err = fs.Lchown("/", 4000, 4000)
	if err != nil {
		t.Fatal(err)
	}

	// Test Lchown on non-existent file
	err = fs.Lchown("/nonexistent.txt", 1000, 1000)
	if err == nil {
		t.Error("Should fail for non-existent file")
	}
}

// TestReadlink tests the Readlink method
func TestReadlink(t *testing.T) {
	fs, err := NewFS()
	if err != nil {
		t.Fatal(err)
	}

	// Create a file and a symlink
	f, err := fs.Create("/target.txt")
	if err != nil {
		t.Fatal(err)
	}
	f.Close()

	err = fs.Symlink("/target.txt", "/link.txt")
	if err != nil {
		t.Fatal(err)
	}

	// Readlink should return the target path
	target, err := fs.Readlink("/link.txt")
	if err != nil {
		t.Fatal(err)
	}
	if target != "/target.txt" {
		t.Errorf("Expected '/target.txt', got '%s'", target)
	}

	// Test Readlink on root (returns empty string for non-symlink)
	target, err = fs.Readlink("/")
	if err != nil {
		t.Fatal(err)
	}
	if target != "" {
		t.Errorf("Expected empty string for root, got '%s'", target)
	}

	// Test Readlink on non-existent file
	_, err = fs.Readlink("/nonexistent.txt")
	if err == nil {
		t.Error("Should fail for non-existent file")
	}
}

// TestSeekEdgeCases tests edge cases for Seek
func TestSeekEdgeCases(t *testing.T) {
	fs, err := NewFS()
	if err != nil {
		t.Fatal(err)
	}

	f, err := fs.Create("/test.txt")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	data := []byte("0123456789")
	f.Write(data)

	// Test SeekStart
	offset, err := f.Seek(5, io.SeekStart)
	if err != nil || offset != 5 {
		t.Errorf("Expected offset 5, got %d, err: %v", offset, err)
	}

	// Test SeekCurrent
	offset, err = f.Seek(2, io.SeekCurrent)
	if err != nil || offset != 7 {
		t.Errorf("Expected offset 7, got %d, err: %v", offset, err)
	}

	// Test SeekEnd
	offset, err = f.Seek(-3, io.SeekEnd)
	if err != nil || offset != 7 {
		t.Errorf("Expected offset 7, got %d, err: %v", offset, err)
	}

	// Test negative offset (should clamp to 0)
	offset, err = f.Seek(-100, io.SeekStart)
	if err != nil || offset != 0 {
		t.Errorf("Expected offset 0, got %d, err: %v", offset, err)
	}
}

// TestChdirEdgeCases tests edge cases for Chdir
func TestChdirEdgeCases(t *testing.T) {
	fs, err := NewFS()
	if err != nil {
		t.Fatal(err)
	}

	// Test chdir to root
	err = fs.Chdir("/")
	if err != nil {
		t.Fatal(err)
	}

	cwd, _ := fs.Getwd()
	if cwd != "/" {
		t.Errorf("Expected cwd '/', got '%s'", cwd)
	}

	// Create directories
	fs.MkdirAll("/a/b/c", 0755)

	// Test absolute path
	err = fs.Chdir("/a/b")
	if err != nil {
		t.Fatal(err)
	}

	cwd, _ = fs.Getwd()
	if cwd != "/a/b" {
		t.Errorf("Expected cwd '/a/b', got '%s'", cwd)
	}

	// Test relative path
	err = fs.Chdir("c")
	if err != nil {
		t.Fatal(err)
	}

	cwd, _ = fs.Getwd()
	if cwd != "/a/b/c" {
		t.Errorf("Expected cwd '/a/b/c', got '%s'", cwd)
	}

	// Test chdir to non-existent directory
	err = fs.Chdir("/nonexistent")
	if err == nil {
		t.Error("Should fail for non-existent directory")
	}

	// Test chdir to a file (should fail)
	fs.Create("/file.txt")
	err = fs.Chdir("/file.txt")
	if err == nil {
		t.Error("Should fail when trying to chdir to a file")
	}
}

// TestOpenFileEdgeCases tests edge cases for OpenFile
func TestOpenFileEdgeCases(t *testing.T) {
	fs, err := NewFS()
	if err != nil {
		t.Fatal(err)
	}

	// Test opening root directory
	f, err := fs.OpenFile("/", os.O_RDONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	f.Close()

	// Test opening current directory
	f, err = fs.OpenFile(".", os.O_RDONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	f.Close()

	// Test O_EXCL with existing file
	fs.Create("/existing.txt")
	_, err = fs.OpenFile("/existing.txt", os.O_CREATE|os.O_EXCL, 0644)
	if err == nil {
		t.Error("Should fail with O_EXCL on existing file")
	}

	// Test opening directory with write flag
	fs.Mkdir("/testdir", 0755)
	_, err = fs.OpenFile("/testdir", os.O_RDWR, 0)
	if err == nil {
		t.Error("Should fail when opening directory with write flag")
	}

	// Test opening directory with O_TRUNC
	_, err = fs.OpenFile("/testdir", os.O_RDONLY|os.O_TRUNC, 0)
	if err == nil {
		t.Error("Should fail when opening directory with O_TRUNC")
	}

	// Test permission check for read-only file
	f, err = fs.Create("/readonly.txt")
	if err != nil {
		t.Fatal(err)
	}
	f.Close()

	fs.Chmod("/readonly.txt", 0000) // No permissions
	_, err = fs.OpenFile("/readonly.txt", os.O_RDONLY, 0)
	if err == nil {
		t.Error("Should fail when opening file without read permission")
	}

	// Test permission check for write
	fs.Chmod("/readonly.txt", 0644)
	fs.Chmod("/readonly.txt", 0444) // Read-only
	_, err = fs.OpenFile("/readonly.txt", os.O_WRONLY, 0)
	if err == nil {
		t.Error("Should fail when opening read-only file for writing")
	}

	// Test permission check for O_RDWR
	_, err = fs.OpenFile("/readonly.txt", os.O_RDWR, 0)
	if err == nil {
		t.Error("Should fail when opening read-only file for read-write")
	}
}

// TestReadEdgeCases tests edge cases for Read
func TestReadEdgeCases(t *testing.T) {
	fs, err := NewFS()
	if err != nil {
		t.Fatal(err)
	}

	// Create a file with data
	f, err := fs.Create("/test.txt")
	if err != nil {
		t.Fatal(err)
	}
	f.Write([]byte("test"))
	f.Close()

	// Test read on write-only file
	wf, err := fs.OpenFile("/test.txt", os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	buf := make([]byte, 10)
	_, err = wf.Read(buf)
	if err == nil {
		t.Error("Should fail to read from write-only file")
	}
	wf.Close()

	// Test read on directory
	fs.Mkdir("/testdir", 0755)
	df, err := fs.Open("/testdir")
	if err != nil {
		t.Fatal(err)
	}
	_, err = df.Read(buf)
	if err == nil {
		t.Error("Should fail to read from directory")
	}
	df.Close()

	// Test read past EOF
	rf, err := fs.Open("/test.txt")
	if err != nil {
		t.Fatal(err)
	}
	rf.Seek(0, io.SeekEnd)
	_, err = rf.Read(buf)
	if err != io.EOF {
		t.Errorf("Expected EOF, got %v", err)
	}
	rf.Close()

	// Test read with nil node (after close)
	f, err = fs.Open("/test.txt")
	if err != nil {
		t.Fatal(err)
	}
	f.Close()
	_, err = f.Read(buf)
	if err == nil {
		t.Error("Should fail to read from closed file")
	}
}

// TestRemoveEdgeCases tests edge cases for Remove
func TestRemoveEdgeCases(t *testing.T) {
	fs, err := NewFS()
	if err != nil {
		t.Fatal(err)
	}

	// Test remove non-empty directory
	fs.MkdirAll("/dir/subdir", 0755)
	err = fs.Remove("/dir")
	if err == nil {
		t.Error("Should fail to remove non-empty directory")
	}

	// Test remove with relative path
	fs.Chdir("/")
	fs.Create("/file.txt")
	err = fs.Remove("file.txt")
	if err != nil {
		t.Fatal(err)
	}

	// Test remove non-existent file with relative path
	fs.Chdir("/dir")
	err = fs.Remove("nonexistent.txt")
	if err == nil {
		t.Error("Should fail to remove non-existent file")
	}
}

// TestRemoveAllEdgeCases tests edge cases for RemoveAll
func TestRemoveAllEdgeCases(t *testing.T) {
	fs, err := NewFS()
	if err != nil {
		t.Fatal(err)
	}

	// Test RemoveAll with relative path
	fs.Chdir("/")
	fs.MkdirAll("/testdir/sub", 0755)
	fs.Create("/testdir/file.txt")

	err = fs.RemoveAll("testdir")
	if err != nil {
		t.Fatal(err)
	}

	_, err = fs.Stat("/testdir")
	if err == nil {
		t.Error("Directory should be removed")
	}

	// Test RemoveAll on non-existent path
	err = fs.RemoveAll("/nonexistent")
	if err == nil {
		t.Error("Should fail for non-existent path")
	}
}

// TestConcurrentFileAccess tests concurrent access to files using lockfs.
// memfs is not thread-safe by design; use lockfs for concurrent access.
func TestConcurrentFileAccess(t *testing.T) {
	raw, err := NewFS()
	if err != nil {
		t.Fatal(err)
	}
	fs, err := lockfs.NewFS(raw)
	if err != nil {
		t.Fatal(err)
	}

	// Create files first (serially to avoid race on fs.data)
	for i := 0; i < 10; i++ {
		name := "/file" + string(rune('0'+i)) + ".txt"
		f, err := fs.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		f.Close()
	}

	var wg sync.WaitGroup
	errors := make(chan error, 20)

	// Concurrent reads
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			name := "/file" + string(rune('0'+n)) + ".txt"
			f, err := fs.Open(name)
			if err != nil {
				errors <- err
				return
			}
			defer f.Close()
			buf := make([]byte, 100)
			f.Read(buf)
		}(i)
	}

	// Concurrent writes
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			name := "/file" + string(rune('0'+n)) + ".txt"
			f, err := fs.OpenFile(name, os.O_WRONLY, 0)
			if err != nil {
				errors <- err
				return
			}
			defer f.Close()
			f.Write([]byte("test data"))
		}(i)
	}

	wg.Wait()
	close(errors)

	for err := range errors {
		t.Error(err)
	}
}

// TestConcurrentDirectoryOps tests concurrent directory operations using lockfs.
// memfs is not thread-safe by design; use lockfs for concurrent access.
func TestConcurrentDirectoryOps(t *testing.T) {
	raw, err := NewFS()
	if err != nil {
		t.Fatal(err)
	}
	fs, err := lockfs.NewFS(raw)
	if err != nil {
		t.Fatal(err)
	}

	var wg sync.WaitGroup
	errors := make(chan error, 30)

	// Concurrent mkdir
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			name := "/dir" + string(rune('0'+n))
			err := fs.Mkdir(name, 0755)
			if err != nil {
				errors <- err
			}
		}(i)
	}

	wg.Wait()

	// Concurrent stat
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			name := "/dir" + string(rune('0'+n))
			_, err := fs.Stat(name)
			if err != nil {
				errors <- err
			}
		}(i)
	}

	wg.Wait()

	// Concurrent remove (use RemoveAll to avoid issues with empty check)
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			name := "/dir" + string(rune('0'+n))
			err := fs.RemoveAll(name)
			if err != nil {
				errors <- err
			}
		}(i)
	}

	wg.Wait()
	close(errors)

	for err := range errors {
		t.Error(err)
	}
}

// TestSymlinkEdgeCases tests edge cases for symlinks
func TestSymlinkEdgeCases(t *testing.T) {
	fs, err := NewFS()
	if err != nil {
		t.Fatal(err)
	}

	// Create target file
	f, err := fs.Create("/target.txt")
	if err != nil {
		t.Fatal(err)
	}
	f.Close()

	// Test creating symlink with relative path
	fs.Chdir("/")
	err = fs.Symlink("target.txt", "link.txt")
	if err != nil {
		t.Fatal(err)
	}

	// Test symlink to non-existent file
	err = fs.Symlink("/nonexistent.txt", "/badlink.txt")
	if err == nil {
		t.Error("Should fail to create symlink to non-existent file")
	}

	// Test updating existing symlink
	err = fs.Symlink("/target.txt", "/link.txt")
	if err != nil {
		t.Fatal(err)
	}

	// Test symlink when newname exists as non-symlink
	fs.Create("/regular.txt")
	err = fs.Symlink("/target.txt", "/regular.txt")
	if err == nil {
		t.Error("Should fail when newname exists as non-symlink")
	}

	// Test symlink without parent directory
	err = fs.Symlink("/target.txt", "/nonexistent/link.txt")
	if err == nil {
		t.Error("Should fail when parent directory doesn't exist")
	}
}

// TestWalkEdgeCases tests edge cases for Walk
func TestWalkEdgeCases(t *testing.T) {
	fs, err := NewFS()
	if err != nil {
		t.Fatal(err)
	}

	// Test walk with error in callback
	fs.MkdirAll("/a/b", 0755)
	expectedErr := errors.New("stop walking")

	err = fs.Walk("/", func(path string, info os.FileInfo, err error) error {
		if path == "/a" {
			return expectedErr
		}
		return nil
	})

	if err != expectedErr {
		t.Errorf("Expected error %v, got %v", expectedErr, err)
	}

	// Test walk on non-existent path
	err = fs.Walk("/nonexistent", func(path string, info os.FileInfo, err error) error {
		return nil
	})

	if err == nil {
		t.Error("Should fail to walk non-existent path")
	}
}

// TestMkdirEdgeCases tests edge cases for Mkdir
func TestMkdirEdgeCases(t *testing.T) {
	fs, err := NewFS()
	if err != nil {
		t.Fatal(err)
	}

	// Test mkdir with relative path
	fs.Chdir("/")
	err = fs.Mkdir("testdir", 0755)
	if err != nil {
		t.Fatal(err)
	}

	// Test mkdir when path already exists
	err = fs.Mkdir("/testdir", 0755)
	if err == nil {
		t.Error("Should fail when directory already exists")
	}

	// Test mkdir without parent
	err = fs.Mkdir("/nonexistent/subdir", 0755)
	if err == nil {
		t.Error("Should fail when parent directory doesn't exist")
	}
}

// TestChmodEdgeCases tests edge cases for Chmod
func TestChmodEdgeCases(t *testing.T) {
	fs, err := NewFS()
	if err != nil {
		t.Fatal(err)
	}

	// Create a file first
	f, err := fs.Create("/test.txt")
	if err != nil {
		t.Fatal(err)
	}
	f.Close()

	// Change mode
	err = fs.Chmod("/test.txt", 0600)
	if err != nil {
		t.Fatal(err)
	}

	info, err := fs.Stat("/test.txt")
	if err != nil {
		t.Fatal(err)
	}

	if info.Mode() != 0600 {
		t.Errorf("Expected mode 0600, got %o", info.Mode())
	}

	// Test chmod on root (preserve directory bit)
	err = fs.Chmod("/", os.ModeDir|0755)
	if err != nil {
		t.Fatal(err)
	}

	// Test chmod on non-existent file
	err = fs.Chmod("/nonexistent.txt", 0644)
	if err == nil {
		t.Error("Should fail for non-existent file")
	}
}

// TestCloseEdgeCases tests edge cases for Close
func TestCloseEdgeCases(t *testing.T) {
	fs, err := NewFS()
	if err != nil {
		t.Fatal(err)
	}

	// Create and write to a file
	f, err := fs.Create("/test.txt")
	if err != nil {
		t.Fatal(err)
	}

	f.Write([]byte("test data"))

	// Close should sync data
	err = f.Close()
	if err != nil {
		t.Fatal(err)
	}

	// Verify data was written
	info, err := fs.Stat("/test.txt")
	if err != nil {
		t.Fatal(err)
	}
	if info.Size() != 9 {
		t.Errorf("Expected size 9, got %d", info.Size())
	}

	// Close again should be safe (node is nil)
	err = f.Close()
	if err != nil {
		t.Error("Second close should not error")
	}
}

// TestSyncEdgeCases tests edge cases for Sync
func TestSyncEdgeCases(t *testing.T) {
	fs, err := NewFS()
	if err != nil {
		t.Fatal(err)
	}

	// Test sync on read-only file
	fs.Create("/test.txt")
	rf, err := fs.Open("/test.txt")
	if err != nil {
		t.Fatal(err)
	}

	err = rf.Sync()
	if err != nil {
		t.Error("Sync on read-only file should not error")
	}
	rf.Close()

	// Test sync after close (node is nil)
	err = rf.Sync()
	if err != nil {
		t.Error("Sync after close should not error")
	}
}

// TestReaddirEdgeCases tests edge cases for Readdir
func TestReaddirEdgeCases(t *testing.T) {
	fs, err := NewFS()
	if err != nil {
		t.Fatal(err)
	}

	// Test readdir on write-only directory
	fs.Mkdir("/testdir", 0755)
	wf, err := fs.OpenFile("/testdir", os.O_WRONLY, 0)
	if err == nil {
		// If we can open directory with O_WRONLY, test that readdir fails
		_, err = wf.Readdir(-1)
		if err != os.ErrPermission {
			t.Errorf("Expected ErrPermission, got %v", err)
		}
		wf.Close()
	}

	// Test readdir on non-directory
	f, err := fs.Create("/file.txt")
	if err != nil {
		t.Fatal(err)
	}
	f.Close()

	ndf, err := fs.Open("/file.txt")
	if err != nil {
		t.Fatal(err)
	}
	_, err = ndf.Readdir(-1)
	if err == nil {
		t.Error("Should fail to readdir on non-directory")
	}
	ndf.Close()

	// Test readdir with n=0 (special case)
	df, err := fs.Open("/testdir")
	if err != nil {
		t.Fatal(err)
	}
	defer df.Close()

	// Create some files
	for i := 0; i < 3; i++ {
		fs.Create("/testdir/file" + string(rune('0'+i)) + ".txt")
	}

	// Readdir with n=0 should reset and return all
	entries, err := df.Readdir(0)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) < 3 {
		t.Errorf("Expected at least 3 entries with n=0, got %d", len(entries))
	}
}

// TestReaddirnamesEdgeCases tests edge cases for Readdirnames
func TestReaddirnamesEdgeCases(t *testing.T) {
	fs, err := NewFS()
	if err != nil {
		t.Fatal(err)
	}

	// Test readdirnames on write-only directory
	fs.Mkdir("/testdir", 0755)
	wf, err := fs.OpenFile("/testdir", os.O_WRONLY, 0)
	if err == nil {
		_, err = wf.Readdirnames(-1)
		if err != os.ErrPermission {
			t.Errorf("Expected ErrPermission, got %v", err)
		}
		wf.Close()
	}

	// Test readdirnames on non-directory
	f, err := fs.Create("/file.txt")
	if err != nil {
		t.Fatal(err)
	}
	f.Close()

	ndf, err := fs.Open("/file.txt")
	if err != nil {
		t.Fatal(err)
	}
	_, err = ndf.Readdirnames(-1)
	if err == nil {
		t.Error("Should fail to readdirnames on non-directory")
	}
	ndf.Close()

	// Test with nil node (closed file)
	df, err := fs.Open("/testdir")
	if err != nil {
		t.Fatal(err)
	}
	df.Close()

	_, err = df.Readdirnames(-1)
	if err == nil {
		t.Error("Should fail on closed file")
	}
}
