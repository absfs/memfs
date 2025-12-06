package memfs

import (
	"testing"
)

// FuzzCreateFile tests file creation with random names
func FuzzCreateFile(f *testing.F) {
	// Add seed corpus
	f.Add("/file.txt")
	f.Add("/a/b/c.txt")
	f.Add("/test")
	f.Add("/.hidden")
	f.Add("/path/with spaces/file.txt")

	f.Fuzz(func(t *testing.T, path string) {
		if len(path) == 0 || path[0] != '/' {
			return // Skip invalid paths
		}

		fs, err := NewFS()
		if err != nil {
			t.Fatal(err)
		}

		// Try to create parent directories
		fs.MkdirAll(path[:len(path)-len(baseName(path))-1], 0755)

		// Create and write to file
		file, err := fs.Create(path)
		if err != nil {
			return // Some paths are invalid, that's expected
		}
		defer file.Close()

		// Write some data
		_, err = file.Write([]byte("test data"))
		if err != nil {
			t.Errorf("Failed to write: %v", err)
		}
	})
}

// FuzzReadWrite tests read/write operations with random data
func FuzzReadWrite(f *testing.F) {
	// Add seed corpus
	f.Add([]byte("hello world"))
	f.Add([]byte{0, 1, 2, 3, 4, 5})
	f.Add([]byte(""))
	f.Add([]byte("a very long string that might cause issues"))

	f.Fuzz(func(t *testing.T, data []byte) {
		fs, err := NewFS()
		if err != nil {
			t.Fatal(err)
		}

		// Create file
		file, err := fs.Create("/test.bin")
		if err != nil {
			t.Fatal(err)
		}

		// Write data
		n, err := file.Write(data)
		if err != nil {
			t.Fatalf("Write failed: %v", err)
		}
		if n != len(data) {
			t.Fatalf("Expected to write %d bytes, wrote %d", len(data), n)
		}
		file.Close()

		// Read it back
		file, err = fs.Open("/test.bin")
		if err != nil {
			t.Fatal(err)
		}

		readBuf := make([]byte, len(data)+10)
		n, _ = file.Read(readBuf)
		file.Close()

		if n != len(data) {
			t.Fatalf("Expected to read %d bytes, read %d", len(data), n)
		}

		// Verify data matches
		for i := 0; i < n; i++ {
			if readBuf[i] != data[i] {
				t.Fatalf("Data mismatch at position %d", i)
			}
		}
	})
}

// FuzzSymlink tests symlink creation with random targets
func FuzzSymlink(f *testing.F) {
	f.Add("/target.txt", "/link")
	f.Add("/a/b/c", "/x/y/z")
	f.Add("../relative", "/link2")

	f.Fuzz(func(t *testing.T, target, linkPath string) {
		if len(linkPath) == 0 || linkPath[0] != '/' {
			return
		}

		fs, err := NewFS()
		if err != nil {
			t.Fatal(err)
		}

		// Create parent directory for link
		if idx := lastIndex(linkPath, '/'); idx > 0 {
			fs.MkdirAll(linkPath[:idx], 0755)
		}

		// Create symlink - errors are expected for many inputs
		err = fs.Symlink(target, linkPath)
		if err != nil {
			return
		}

		// Verify we can read it back
		readTarget, err := fs.Readlink(linkPath)
		if err != nil {
			t.Errorf("Failed to readlink: %v", err)
			return
		}

		if readTarget != target {
			t.Errorf("Symlink target mismatch: got %q, want %q", readTarget, target)
		}
	})
}

// Helper functions
func baseName(path string) string {
	for i := len(path) - 1; i >= 0; i-- {
		if path[i] == '/' {
			return path[i+1:]
		}
	}
	return path
}

func lastIndex(s string, c byte) int {
	for i := len(s) - 1; i >= 0; i-- {
		if s[i] == c {
			return i
		}
	}
	return -1
}
