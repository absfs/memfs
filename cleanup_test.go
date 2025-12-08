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

	// Verify data exists in store
	size, err := fs.store.Stat(ino)
	if err != nil {
		t.Fatalf("Failed to stat data in store: %v", err)
	}
	if size == 0 {
		t.Fatalf("Expected data to exist before removal")
	}

	// Remove the file
	err = fs.Remove("/testfile.txt")
	if err != nil {
		t.Fatalf("Failed to remove file: %v", err)
	}

	// Verify data is cleaned up (size should be 0 for removed files)
	size, err = fs.store.Stat(ino)
	if err != nil {
		t.Fatalf("Failed to stat data in store after removal: %v", err)
	}
	if size != 0 {
		t.Errorf("Expected size to be 0 after removal, but got: %d", size)
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

	// Verify all data exists (files should have non-zero size)
	for i, ino := range inodes {
		size, err := fs.store.Stat(ino)
		if err != nil {
			t.Fatalf("Failed to stat inode %d in store: %v", ino, err)
		}
		// Files have data, directories may have size 0
		if i < len(files) && size == 0 {
			t.Fatalf("Expected file inode %d to have data before removal", ino)
		}
	}

	// Remove the entire directory tree
	err = fs.RemoveAll("/dir1")
	if err != nil {
		t.Fatalf("Failed to remove directory: %v", err)
	}

	// Verify all data is cleaned up
	for _, ino := range inodes {
		size, err := fs.store.Stat(ino)
		if err != nil {
			t.Fatalf("Failed to stat inode %d in store after removal: %v", ino, err)
		}
		if size != 0 {
			t.Errorf("Expected size to be 0 for inode %d after RemoveAll, but got: %d", ino, size)
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

	// The store should have minimal entries after cleanup
	// We can't directly count entries in the map, but we can verify that
	// repeatedly creating and deleting files doesn't cause unbounded growth.
	// This is more of a memory profiling concern, but we can at least verify
	// the test completes without error.

	// Try to create a file to verify the filesystem is still functional
	f, err := fs.Create("/verify.txt")
	if err != nil {
		t.Fatalf("Failed to create verification file after many cycles: %v", err)
	}
	f.Close()

	stat, err := fs.Stat("/verify.txt")
	if err != nil {
		t.Fatalf("Failed to stat verification file: %v", err)
	}
	if stat.Size() != 0 {
		t.Errorf("Expected verification file to have size 0, got %d", stat.Size())
	}
}
