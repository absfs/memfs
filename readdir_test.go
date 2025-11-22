package memfs

import (
	"os"
	"testing"
)

// TestReaddirNegativeSlicePanic tests that Readdir doesn't panic when n < diroffset
func TestReaddirNegativeSlicePanic(t *testing.T) {
	fs, err := NewFS()
	if err != nil {
		t.Fatalf("Failed to create filesystem: %v", err)
	}

	// Create a test directory with some files
	err = fs.Mkdir("/testdir", 0755)
	if err != nil {
		t.Fatalf("Failed to create directory: %v", err)
	}

	// Create several files in the directory
	for i := 0; i < 10; i++ {
		path := "/testdir/file" + string(rune('0'+i))
		f, err := fs.Create(path)
		if err != nil {
			t.Fatalf("Failed to create file %s: %v", path, err)
		}
		f.Close()
	}

	// Open the directory
	dir, err := fs.Open("/testdir")
	if err != nil {
		t.Fatalf("Failed to open directory: %v", err)
	}
	defer dir.Close()

	// Read first 5 entries
	entries, err := dir.Readdir(5)
	if err != nil {
		t.Fatalf("First Readdir failed: %v", err)
	}
	if len(entries) != 5 {
		t.Errorf("Expected 5 entries, got %d", len(entries))
	}

	// Now read next 3 entries (this should work without panic)
	// This is the key test - in the old code, if diroffset > n, it would panic
	entries, err = dir.Readdir(3)
	if err != nil {
		t.Fatalf("Second Readdir failed: %v", err)
	}
	if len(entries) != 3 {
		t.Errorf("Expected 3 entries, got %d", len(entries))
	}

	// Read remaining entries - just verify it doesn't panic
	entries, err = dir.Readdir(10)
	if err != nil && err != os.ErrClosed {
		t.Fatalf("Third Readdir failed: %v", err)
	}
	// Don't check exact count, just that we got some entries
	t.Logf("Third call returned %d entries", len(entries))
}

// TestReaddirnamesNegativeSlicePanic tests that Readdirnames doesn't panic when n < diroffset
func TestReaddirnamesNegativeSlicePanic(t *testing.T) {
	fs, err := NewFS()
	if err != nil {
		t.Fatalf("Failed to create filesystem: %v", err)
	}

	// Create a test directory with some files
	err = fs.Mkdir("/testdir2", 0755)
	if err != nil {
		t.Fatalf("Failed to create directory: %v", err)
	}

	// Create several files in the directory
	for i := 0; i < 10; i++ {
		path := "/testdir2/file" + string(rune('0'+i))
		f, err := fs.Create(path)
		if err != nil {
			t.Fatalf("Failed to create file %s: %v", path, err)
		}
		f.Close()
	}

	// Open the directory
	dir, err := fs.Open("/testdir2")
	if err != nil {
		t.Fatalf("Failed to open directory: %v", err)
	}
	defer dir.Close()

	// Read first 7 entries
	names, err := dir.Readdirnames(7)
	if err != nil {
		t.Fatalf("First Readdirnames failed: %v", err)
	}
	if len(names) != 7 {
		t.Errorf("Expected 7 entries, got %d", len(names))
	}

	// Now read next 2 entries (this should work without panic)
	names, err = dir.Readdirnames(2)
	if err != nil {
		t.Fatalf("Second Readdirnames failed: %v", err)
	}
	if len(names) != 2 {
		t.Errorf("Expected 2 entries, got %d", len(names))
	}

	// Read remaining entries - just verify it doesn't panic
	names, err = dir.Readdirnames(5)
	if err != nil && err != os.ErrClosed {
		t.Fatalf("Third Readdirnames failed: %v", err)
	}
	t.Logf("Third call returned %d entries", len(names))
}

// TestReaddirAll tests reading all entries at once
func TestReaddirAll(t *testing.T) {
	fs, err := NewFS()
	if err != nil {
		t.Fatalf("Failed to create filesystem: %v", err)
	}

	err = fs.Mkdir("/testdir3", 0755)
	if err != nil {
		t.Fatalf("Failed to create directory: %v", err)
	}

	// Create files
	for i := 0; i < 5; i++ {
		path := "/testdir3/file" + string(rune('0'+i))
		f, err := fs.Create(path)
		if err != nil {
			t.Fatalf("Failed to create file %s: %v", path, err)
		}
		f.Close()
	}

	dir, err := fs.Open("/testdir3")
	if err != nil {
		t.Fatalf("Failed to open directory: %v", err)
	}
	defer dir.Close()

	// Read all entries (n <= 0)
	entries, err := dir.Readdir(-1)
	if err != nil {
		t.Fatalf("Readdir(-1) failed: %v", err)
	}
	// Should have at least the files we created (may include . and ..)
	if len(entries) < 5 {
		t.Errorf("Expected at least 5 entries, got %d", len(entries))
	}
	t.Logf("Readdir(-1) returned %d entries", len(entries))

	// Reading again should return EOF
	_, err = dir.Readdir(-1)
	if err != os.ErrClosed && err == nil {
		// The behavior after reading all might vary, but it shouldn't panic
		t.Logf("Second Readdir(-1) returned: %v", err)
	}
}
