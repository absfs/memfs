package memfs

import (
	"testing"

	"github.com/absfs/inode"
)

// TestDataCleanupOnRemove verifies that fs.data is cleaned up when files are removed
func TestDataCleanupOnRemove(t *testing.T) {
	fs, err := NewFS()
	if err != nil {
		t.Fatalf("Failed to create filesystem: %v", err)
	}

	// Create a file
	f, err := fs.Create("/testfile.txt")
	if err != nil {
		t.Fatalf("Failed to create file: %v", err)
	}

	// Write some data
	data := []byte("test data")
	_, err = f.Write(data)
	if err != nil {
		t.Fatalf("Failed to write to file: %v", err)
	}
	f.Close()

	// Get the inode number
	stat, err := fs.Stat("/testfile.txt")
	if err != nil {
		t.Fatalf("Failed to stat file: %v", err)
	}
	ino := stat.Sys().(*inode.Inode).Ino

	// Verify data exists
	if int(ino) >= len(fs.data) {
		t.Fatalf("Inode %d is out of bounds for fs.data (len=%d)", ino, len(fs.data))
	}
	if fs.data[int(ino)] == nil {
		t.Fatalf("Expected data to exist before removal")
	}

	// Remove the file
	err = fs.Remove("/testfile.txt")
	if err != nil {
		t.Fatalf("Failed to remove file: %v", err)
	}

	// Verify data is cleaned up
	if fs.data[int(ino)] != nil {
		t.Errorf("Expected data to be nil after removal, but got: %v", fs.data[int(ino)])
	}
}

// TestDataCleanupOnRemoveAll verifies that fs.data is cleaned up for directories and all children
func TestDataCleanupOnRemoveAll(t *testing.T) {
	fs, err := NewFS()
	if err != nil {
		t.Fatalf("Failed to create filesystem: %v", err)
	}

	// Create a directory structure
	err = fs.MkdirAll("/dir1/dir2", 0755)
	if err != nil {
		t.Fatalf("Failed to create directories: %v", err)
	}

	// Create files in the directories
	files := []string{
		"/dir1/file1.txt",
		"/dir1/dir2/file2.txt",
		"/dir1/dir2/file3.txt",
	}

	inodes := make([]uint64, 0)

	for _, path := range files {
		f, err := fs.Create(path)
		if err != nil {
			t.Fatalf("Failed to create file %s: %v", path, err)
		}
		f.Write([]byte("test data"))
		f.Close()

		stat, err := fs.Stat(path)
		if err != nil {
			t.Fatalf("Failed to stat file %s: %v", path, err)
		}
		inodes = append(inodes, stat.Sys().(*inode.Inode).Ino)
	}

	// Also get the directory inodes
	for _, path := range []string{"/dir1", "/dir1/dir2"} {
		stat, err := fs.Stat(path)
		if err != nil {
			t.Fatalf("Failed to stat directory %s: %v", path, err)
		}
		inodes = append(inodes, stat.Sys().(*inode.Inode).Ino)
	}

	// Verify all data exists
	for _, ino := range inodes {
		if int(ino) >= len(fs.data) {
			t.Fatalf("Inode %d is out of bounds for fs.data (len=%d)", ino, len(fs.data))
		}
		if fs.data[int(ino)] == nil {
			t.Fatalf("Expected data to exist for inode %d before removal", ino)
		}
	}

	// Remove the entire directory tree
	err = fs.RemoveAll("/dir1")
	if err != nil {
		t.Fatalf("Failed to remove directory: %v", err)
	}

	// Verify all data is cleaned up
	for _, ino := range inodes {
		if fs.data[int(ino)] != nil {
			t.Errorf("Expected data to be nil for inode %d after RemoveAll, but got: %v", ino, fs.data[int(ino)])
		}
	}
}

// TestMemoryLeakPrevention simulates a workload with many create/delete cycles
func TestMemoryLeakPrevention(t *testing.T) {
	fs, err := NewFS()
	if err != nil {
		t.Fatalf("Failed to create filesystem: %v", err)
	}

	// Create and delete many files
	iterations := 100
	for i := 0; i < iterations; i++ {
		path := "/tempfile.txt"
		f, err := fs.Create(path)
		if err != nil {
			t.Fatalf("Failed to create file on iteration %d: %v", i, err)
		}

		// Write some data
		data := make([]byte, 1024) // 1KB of data
		f.Write(data)
		f.Close()

		// Remove the file
		err = fs.Remove(path)
		if err != nil {
			t.Fatalf("Failed to remove file on iteration %d: %v", i, err)
		}
	}

	// Count non-nil entries in fs.data (excluding root and cwd)
	nonNilCount := 0
	for i, d := range fs.data {
		if d != nil && i > 1 { // Skip first two entries (reserved)
			nonNilCount++
		}
	}

	// After all the create/delete cycles, there should be minimal non-nil entries
	// (only the root and working directory)
	if nonNilCount > 0 {
		t.Errorf("Expected 0 non-nil data entries after cleanup, but found %d", nonNilCount)
	}
}
