package memfs

import (
	"errors"
	"io"
	"io/fs"
	"os"
	"syscall"
	"testing"
	"testing/fstest"
)

// TestReadDir tests the FileSystem.ReadDir method (io/fs.ReadDirFS interface)
func TestReadDir(t *testing.T) {
	t.Run("ReadRootDirectory", func(t *testing.T) {
		filesystem, err := NewFS()
		if err != nil {
			t.Fatal(err)
		}

		// Create some files and directories in root
		filesystem.Create("/file1.txt")
		filesystem.Create("/file2.txt")
		filesystem.Mkdir("/dir1", 0755)
		filesystem.Mkdir("/dir2", 0755)

		entries, err := filesystem.ReadDir("/")
		if err != nil {
			t.Fatalf("ReadDir failed: %v", err)
		}

		if len(entries) != 4 {
			t.Errorf("Expected 4 entries, got %d", len(entries))
		}

		// Verify entries are sorted by name
		expectedOrder := []string{"dir1", "dir2", "file1.txt", "file2.txt"}
		for i, entry := range entries {
			if entry.Name() != expectedOrder[i] {
				t.Errorf("Entry %d: expected %s, got %s", i, expectedOrder[i], entry.Name())
			}
		}
	})

	t.Run("ReadSubdirectory", func(t *testing.T) {
		filesystem, err := NewFS()
		if err != nil {
			t.Fatal(err)
		}

		// Create a subdirectory with files
		filesystem.Mkdir("/subdir", 0755)
		filesystem.Create("/subdir/a.txt")
		filesystem.Create("/subdir/b.txt")
		filesystem.Create("/subdir/c.txt")

		entries, err := filesystem.ReadDir("/subdir")
		if err != nil {
			t.Fatalf("ReadDir failed: %v", err)
		}

		if len(entries) != 3 {
			t.Errorf("Expected 3 entries, got %d", len(entries))
		}

		// Verify sorted order
		expectedNames := []string{"a.txt", "b.txt", "c.txt"}
		for i, entry := range entries {
			if entry.Name() != expectedNames[i] {
				t.Errorf("Expected %s, got %s", expectedNames[i], entry.Name())
			}
		}
	})

	t.Run("ReadNonExistentDirectory", func(t *testing.T) {
		filesystem, err := NewFS()
		if err != nil {
			t.Fatal(err)
		}

		_, err = filesystem.ReadDir("/nonexistent")
		if err == nil {
			t.Error("Expected error for non-existent directory")
		}

		pathErr, ok := err.(*os.PathError)
		if !ok {
			t.Errorf("Expected *os.PathError, got %T", err)
		} else if pathErr.Op != "open" {
			t.Errorf("Expected op 'open', got %s", pathErr.Op)
		}
	})

	t.Run("ReadFileNotDirectory", func(t *testing.T) {
		filesystem, err := NewFS()
		if err != nil {
			t.Fatal(err)
		}

		filesystem.Create("/file.txt")

		_, err = filesystem.ReadDir("/file.txt")
		if err == nil {
			t.Error("Expected error when reading file as directory")
		}

		// The error comes from File.ReadDir which returns syscall.ENOTDIR directly
		if !errors.Is(err, syscall.ENOTDIR) {
			t.Errorf("Expected ENOTDIR error, got %v", err)
		}
	})

	t.Run("EntriesAreSorted", func(t *testing.T) {
		filesystem, err := NewFS()
		if err != nil {
			t.Fatal(err)
		}

		filesystem.Mkdir("/testdir", 0755)
		// Create files in non-alphabetical order
		filesystem.Create("/testdir/zebra.txt")
		filesystem.Create("/testdir/apple.txt")
		filesystem.Create("/testdir/banana.txt")
		filesystem.Mkdir("/testdir/aardvark", 0755)

		entries, err := filesystem.ReadDir("/testdir")
		if err != nil {
			t.Fatal(err)
		}

		expectedOrder := []string{"aardvark", "apple.txt", "banana.txt", "zebra.txt"}
		for i, entry := range entries {
			if entry.Name() != expectedOrder[i] {
				t.Errorf("Entry %d: expected %s, got %s", i, expectedOrder[i], entry.Name())
			}
		}
	})
}

// TestReadFile tests the FileSystem.ReadFile method (io/fs.ReadFileFS interface)
func TestReadFile(t *testing.T) {
	t.Run("ReadExistingFile", func(t *testing.T) {
		filesystem, err := NewFS()
		if err != nil {
			t.Fatal(err)
		}

		content := []byte("Hello, World! This is test content.")
		f, _ := filesystem.Create("/test.txt")
		f.Write(content)
		f.Close()

		data, err := filesystem.ReadFile("/test.txt")
		if err != nil {
			t.Fatalf("ReadFile failed: %v", err)
		}

		if string(data) != string(content) {
			t.Errorf("Expected %q, got %q", string(content), string(data))
		}
	})

	t.Run("ReadNonExistentFile", func(t *testing.T) {
		filesystem, err := NewFS()
		if err != nil {
			t.Fatal(err)
		}

		_, err = filesystem.ReadFile("/nonexistent.txt")
		if err == nil {
			t.Error("Expected error for non-existent file")
		}

		pathErr, ok := err.(*os.PathError)
		if !ok {
			t.Errorf("Expected *os.PathError, got %T", err)
		} else if pathErr.Op != "open" {
			t.Errorf("Expected op 'open', got %s", pathErr.Op)
		}
	})

	t.Run("ReadDirectory", func(t *testing.T) {
		filesystem, err := NewFS()
		if err != nil {
			t.Fatal(err)
		}

		filesystem.Mkdir("/dir", 0755)

		data, err := filesystem.ReadFile("/dir")
		// ReadFile on directory may succeed but return empty data, or may fail
		// Let's just verify it doesn't panic and handles it reasonably
		if err == nil && len(data) != 0 {
			t.Error("Expected empty data or error when reading directory")
		}
	})

	t.Run("ReadEmptyFile", func(t *testing.T) {
		filesystem, err := NewFS()
		if err != nil {
			t.Fatal(err)
		}

		f, _ := filesystem.Create("/empty.txt")
		f.Close()

		data, err := filesystem.ReadFile("/empty.txt")
		if err != nil {
			t.Fatalf("ReadFile failed: %v", err)
		}

		if len(data) != 0 {
			t.Errorf("Expected empty data, got %d bytes", len(data))
		}
	})

	t.Run("ReadLargeFile", func(t *testing.T) {
		filesystem, err := NewFS()
		if err != nil {
			t.Fatal(err)
		}

		// Create a large file (10KB)
		largeContent := make([]byte, 10240)
		for i := range largeContent {
			largeContent[i] = byte(i % 256)
		}

		f, _ := filesystem.Create("/large.bin")
		f.Write(largeContent)
		f.Close()

		data, err := filesystem.ReadFile("/large.bin")
		if err != nil {
			t.Fatalf("ReadFile failed: %v", err)
		}

		if len(data) != len(largeContent) {
			t.Errorf("Expected %d bytes, got %d", len(largeContent), len(data))
		}

		for i := range data {
			if data[i] != largeContent[i] {
				t.Errorf("Byte %d: expected %d, got %d", i, largeContent[i], data[i])
				break
			}
		}
	})

	t.Run("ReadFileRelativePath", func(t *testing.T) {
		filesystem, err := NewFS()
		if err != nil {
			t.Fatal(err)
		}

		// Change to a subdirectory
		filesystem.Mkdir("/subdir", 0755)
		filesystem.Chdir("/subdir")

		// Create a file in the current directory
		f, _ := filesystem.Create("file.txt")
		f.Write([]byte("relative path content"))
		f.Close()

		// ReadFile with relative path
		data, err := filesystem.ReadFile("file.txt")
		if err != nil {
			t.Fatalf("ReadFile with relative path failed: %v", err)
		}

		if string(data) != "relative path content" {
			t.Errorf("Expected 'relative path content', got %q", string(data))
		}
	})
}

// TestSub tests the FileSystem.Sub method (io/fs.SubFS interface)
func TestSub(t *testing.T) {
	t.Run("SubAtRoot", func(t *testing.T) {
		filesystem, err := NewFS()
		if err != nil {
			t.Fatal(err)
		}

		filesystem.Create("/file.txt")

		subFS, err := filesystem.Sub("/")
		if err != nil {
			t.Fatalf("Sub failed: %v", err)
		}

		// Should be able to open file relative to root
		f, err := subFS.Open("file.txt")
		if err != nil {
			t.Errorf("Failed to open file in sub-filesystem: %v", err)
		} else {
			f.Close()
		}
	})

	t.Run("SubAtSubdirectory", func(t *testing.T) {
		filesystem, err := NewFS()
		if err != nil {
			t.Fatal(err)
		}

		filesystem.Mkdir("/subdir", 0755)
		filesystem.Create("/subdir/file1.txt")
		filesystem.Create("/subdir/file2.txt")
		filesystem.Create("/other.txt")

		subFS, err := filesystem.Sub("/subdir")
		if err != nil {
			t.Fatalf("Sub failed: %v", err)
		}

		// Should be able to access files in subdirectory
		f, err := subFS.Open("file1.txt")
		if err != nil {
			t.Errorf("Failed to open file1.txt: %v", err)
		} else {
			f.Close()
		}

		// Should NOT be able to access files outside subdirectory
		_, err = subFS.Open("../other.txt")
		if err == nil {
			t.Error("Should not be able to escape sub-filesystem")
		}
	})

	t.Run("SubNonExistentDirectory", func(t *testing.T) {
		filesystem, err := NewFS()
		if err != nil {
			t.Fatal(err)
		}

		_, err = filesystem.Sub("/nonexistent")
		if err == nil {
			t.Error("Expected error for non-existent directory")
		}
	})

	t.Run("SubWithFilePath", func(t *testing.T) {
		filesystem, err := NewFS()
		if err != nil {
			t.Fatal(err)
		}

		filesystem.Create("/file.txt")

		_, err = filesystem.Sub("/file.txt")
		if err == nil {
			t.Error("Expected error when Sub is called with file path")
		}
	})

	t.Run("SubFSWorksWithStdlib", func(t *testing.T) {
		filesystem, err := NewFS()
		if err != nil {
			t.Fatal(err)
		}

		// Create test structure
		filesystem.Mkdir("/testdir", 0755)
		filesystem.Mkdir("/testdir/subdir", 0755)
		f, _ := filesystem.Create("/testdir/file.txt")
		f.Write([]byte("test content"))
		f.Close()
		f2, _ := filesystem.Create("/testdir/subdir/nested.txt")
		f2.Write([]byte("nested content"))
		f2.Close()

		subFS, err := filesystem.Sub("/testdir")
		if err != nil {
			t.Fatal(err)
		}

		// Test with fs.ReadFile
		data, err := fs.ReadFile(subFS, "file.txt")
		if err != nil {
			t.Errorf("fs.ReadFile failed: %v", err)
		} else if string(data) != "test content" {
			t.Errorf("Expected 'test content', got %q", string(data))
		}

		// Test with fs.ReadDir
		entries, err := fs.ReadDir(subFS, ".")
		if err != nil {
			t.Errorf("fs.ReadDir failed: %v", err)
		} else if len(entries) != 2 {
			t.Errorf("Expected 2 entries, got %d", len(entries))
		}

		// Test with fs.Stat
		info, err := fs.Stat(subFS, "file.txt")
		if err != nil {
			t.Errorf("fs.Stat failed: %v", err)
		} else if info.Size() != 12 {
			t.Errorf("Expected size 12, got %d", info.Size())
		}
	})

	t.Run("SubFSNestedSub", func(t *testing.T) {
		filesystem, err := NewFS()
		if err != nil {
			t.Fatal(err)
		}

		filesystem.MkdirAll("/a/b/c", 0755)
		f, _ := filesystem.Create("/a/b/c/file.txt")
		f.Write([]byte("nested"))
		f.Close()

		// Create sub at /a
		subA, err := filesystem.Sub("/a")
		if err != nil {
			t.Fatal(err)
		}

		// Create sub of sub at b
		subB, err := fs.Sub(subA, "b")
		if err != nil {
			t.Fatal(err)
		}

		// Should be able to access c/file.txt from subB
		data, err := fs.ReadFile(subB, "c/file.txt")
		if err != nil {
			t.Errorf("Failed to read nested file: %v", err)
		} else if string(data) != "nested" {
			t.Errorf("Expected 'nested', got %q", string(data))
		}
	})
}

// TestFileReadDir tests the File.ReadDir method
func TestFileReadDir(t *testing.T) {
	t.Run("ReadAllEntries", func(t *testing.T) {
		filesystem, err := NewFS()
		if err != nil {
			t.Fatal(err)
		}

		filesystem.Mkdir("/testdir", 0755)
		filesystem.Create("/testdir/file1.txt")
		filesystem.Create("/testdir/file2.txt")
		filesystem.Create("/testdir/file3.txt")
		filesystem.Mkdir("/testdir/subdir", 0755)

		dir, err := filesystem.Open("/testdir")
		if err != nil {
			t.Fatal(err)
		}
		defer dir.Close()

		// Read all entries (n <= 0)
		entries, err := dir.ReadDir(-1)
		if err != nil {
			t.Fatalf("ReadDir failed: %v", err)
		}

		if len(entries) != 4 {
			t.Errorf("Expected 4 entries, got %d", len(entries))
		}

		// Verify entries are sorted
		expectedNames := []string{"file1.txt", "file2.txt", "file3.txt", "subdir"}
		for i, entry := range entries {
			if entry.Name() != expectedNames[i] {
				t.Errorf("Entry %d: expected %s, got %s", i, expectedNames[i], entry.Name())
			}
		}
	})

	t.Run("ReadPartialEntries", func(t *testing.T) {
		filesystem, err := NewFS()
		if err != nil {
			t.Fatal(err)
		}

		filesystem.Mkdir("/testdir", 0755)
		for i := 0; i < 10; i++ {
			filesystem.Create("/testdir/file" + string(rune('0'+i)) + ".txt")
		}

		dir, err := filesystem.Open("/testdir")
		if err != nil {
			t.Fatal(err)
		}
		defer dir.Close()

		// Read 3 entries
		entries1, err := dir.ReadDir(3)
		if err != nil {
			t.Fatalf("First ReadDir failed: %v", err)
		}
		if len(entries1) != 3 {
			t.Errorf("Expected 3 entries, got %d", len(entries1))
		}

		// Read 3 more entries
		entries2, err := dir.ReadDir(3)
		if err != nil {
			t.Fatalf("Second ReadDir failed: %v", err)
		}
		if len(entries2) != 3 {
			t.Errorf("Expected 3 entries, got %d", len(entries2))
		}

		// Verify no overlap
		for _, e1 := range entries1 {
			for _, e2 := range entries2 {
				if e1.Name() == e2.Name() {
					t.Errorf("Duplicate entry: %s", e1.Name())
				}
			}
		}
	})

	t.Run("IterateWithMultipleCalls", func(t *testing.T) {
		filesystem, err := NewFS()
		if err != nil {
			t.Fatal(err)
		}

		filesystem.Mkdir("/testdir", 0755)
		expectedCount := 7
		for i := 0; i < expectedCount; i++ {
			filesystem.Create("/testdir/file" + string(rune('0'+i)) + ".txt")
		}

		dir, err := filesystem.Open("/testdir")
		if err != nil {
			t.Fatal(err)
		}
		defer dir.Close()

		// Read entries one by one until EOF
		var allEntries []fs.DirEntry
		for {
			entries, err := dir.ReadDir(1)
			if err == io.EOF {
				break
			}
			if err != nil {
				t.Fatalf("ReadDir failed: %v", err)
			}
			if len(entries) == 0 {
				break
			}
			allEntries = append(allEntries, entries...)
		}

		if len(allEntries) != expectedCount {
			t.Errorf("Expected %d total entries, got %d", expectedCount, len(allEntries))
		}

		// Verify next call returns EOF
		entries, err := dir.ReadDir(1)
		if err != io.EOF {
			t.Errorf("Expected EOF, got %v", err)
		}
		if len(entries) != 0 {
			t.Errorf("Expected 0 entries after EOF, got %d", len(entries))
		}
	})

	t.Run("ReadDirOnFile", func(t *testing.T) {
		filesystem, err := NewFS()
		if err != nil {
			t.Fatal(err)
		}

		filesystem.Create("/file.txt")

		file, err := filesystem.Open("/file.txt")
		if err != nil {
			t.Fatal(err)
		}
		defer file.Close()

		_, err = file.ReadDir(-1)
		if err == nil {
			t.Error("Expected error when calling ReadDir on file")
		}
		if !errors.Is(err, syscall.ENOTDIR) {
			t.Errorf("Expected ENOTDIR error, got %v", err)
		}
	})

	t.Run("ReadDirAfterClose", func(t *testing.T) {
		filesystem, err := NewFS()
		if err != nil {
			t.Fatal(err)
		}

		filesystem.Mkdir("/testdir", 0755)

		dir, err := filesystem.Open("/testdir")
		if err != nil {
			t.Fatal(err)
		}
		dir.Close()

		// ReadDir on closed file should return error because node is nil
		// The function checks if node is nil before calling IsDir()
		defer func() {
			if r := recover(); r != nil {
				t.Errorf("ReadDir on closed file should not panic: %v", r)
			}
		}()

		_, err = dir.ReadDir(-1)
		if err == nil {
			t.Error("Expected error when calling ReadDir on closed file")
		}
	})

	t.Run("DirEntryMethods", func(t *testing.T) {
		filesystem, err := NewFS()
		if err != nil {
			t.Fatal(err)
		}

		filesystem.Mkdir("/testdir", 0755)
		f, _ := filesystem.Create("/testdir/file.txt")
		f.Write([]byte("content"))
		f.Close()
		filesystem.Mkdir("/testdir/subdir", 0755)

		dir, err := filesystem.Open("/testdir")
		if err != nil {
			t.Fatal(err)
		}
		defer dir.Close()

		entries, err := dir.ReadDir(-1)
		if err != nil {
			t.Fatal(err)
		}

		// Find the file entry
		var fileEntry, dirEntry fs.DirEntry
		for _, entry := range entries {
			if entry.Name() == "file.txt" {
				fileEntry = entry
			}
			if entry.Name() == "subdir" {
				dirEntry = entry
			}
		}

		if fileEntry == nil {
			t.Fatal("File entry not found")
		}
		if dirEntry == nil {
			t.Fatal("Directory entry not found")
		}

		// Test DirEntry methods on file
		if fileEntry.IsDir() {
			t.Error("file.txt should not be a directory")
		}
		if fileEntry.Type()&fs.ModeDir != 0 {
			t.Error("file.txt type should not be directory")
		}
		info, err := fileEntry.Info()
		if err != nil {
			t.Errorf("Failed to get info: %v", err)
		} else if info.Size() != 7 {
			t.Errorf("Expected size 7, got %d", info.Size())
		}

		// Test DirEntry methods on directory
		if !dirEntry.IsDir() {
			t.Error("subdir should be a directory")
		}
		if dirEntry.Type()&fs.ModeDir == 0 {
			t.Error("subdir type should be directory")
		}
		info, err = dirEntry.Info()
		if err != nil {
			t.Errorf("Failed to get info: %v", err)
		} else if !info.IsDir() {
			t.Error("Info should indicate directory")
		}
	})
}

// TestFSTestCompatibility tests that memfs works with the stdlib fstest package via Sub
func TestFSTestCompatibility(t *testing.T) {
	t.Run("BasicFSTest", func(t *testing.T) {
		filesystem, err := NewFS()
		if err != nil {
			t.Fatal(err)
		}

		// Create a simple file structure
		filesystem.Mkdir("/dir", 0755)
		f1, _ := filesystem.Create("/file1.txt")
		f1.Write([]byte("content1"))
		f1.Close()
		f2, _ := filesystem.Create("/dir/file2.txt")
		f2.Write([]byte("content2"))
		f2.Close()

		// Get fs.FS via Sub method
		stdFS, err := filesystem.Sub("/")
		if err != nil {
			t.Fatal(err)
		}

		// Test with fstest.TestFS
		if err := fstest.TestFS(stdFS, "file1.txt", "dir/file2.txt"); err != nil {
			t.Errorf("fstest.TestFS failed: %v", err)
		}
	})

	t.Run("NestedStructure", func(t *testing.T) {
		filesystem, err := NewFS()
		if err != nil {
			t.Fatal(err)
		}

		// Create nested structure
		filesystem.MkdirAll("/a/b/c", 0755)
		f1, _ := filesystem.Create("/a/file.txt")
		f1.Write([]byte("a"))
		f1.Close()
		f2, _ := filesystem.Create("/a/b/file.txt")
		f2.Write([]byte("ab"))
		f2.Close()
		f3, _ := filesystem.Create("/a/b/c/file.txt")
		f3.Write([]byte("abc"))
		f3.Close()

		// Get fs.FS via Sub method
		stdFS, err := filesystem.Sub("/")
		if err != nil {
			t.Fatal(err)
		}

		if err := fstest.TestFS(stdFS, "a/file.txt", "a/b/file.txt", "a/b/c/file.txt"); err != nil {
			t.Errorf("fstest.TestFS failed: %v", err)
		}
	})
}
