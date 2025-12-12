package memfs_test

import (
	"bytes"
	"fmt"
	iofs "io/fs"
	"os"
	"path"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/absfs/absfs"
	"github.com/absfs/fstools"
	"github.com/absfs/memfs"
)

func TestInterface(t *testing.T) {
	testfs, err := memfs.NewFS()
	if err != nil {
		t.Fatal(err)
	}

	var fs absfs.SymlinkFileSystem
	fs = testfs
	_ = fs
}

func TestWalk(t *testing.T) {

	fs, err := memfs.NewFS()
	if err != nil {
		t.Fatal(err)
	}

	// Create a test directory structure in memfs instead of walking OS files
	// This avoids Windows file locking issues
	testDirs := []string{
		"/testdir",
		"/testdir/subdir1",
		"/testdir/subdir2",
		"/testdir/subdir1/nested",
	}
	testFiles := []string{
		"/testdir/file1.txt",
		"/testdir/file2.txt",
		"/testdir/subdir1/file3.txt",
		"/testdir/subdir1/nested/file4.txt",
		"/testdir/subdir2/file5.txt",
	}

	for _, dir := range testDirs {
		if err := fs.MkdirAll(dir, 0755); err != nil {
			t.Fatalf("MkdirAll(%s) failed: %v", dir, err)
		}
	}
	for _, file := range testFiles {
		f, err := fs.Create(file)
		if err != nil {
			t.Fatalf("Create(%s) failed: %v", file, err)
		}
		f.Write([]byte("test content for " + file))
		f.Close()
	}

	testpath := "/testdir"

	t.Run("Walk", func(t *testing.T) {
		// Expected: 4 directories + 5 files = 9 entries, plus root = 10
		expectedPaths := map[string]bool{
			"/testdir":                    true,
			"/testdir/subdir1":            true,
			"/testdir/subdir2":            true,
			"/testdir/subdir1/nested":     true,
			"/testdir/file1.txt":          true,
			"/testdir/file2.txt":          true,
			"/testdir/subdir1/file3.txt":  true,
			"/testdir/subdir1/nested/file4.txt": true,
			"/testdir/subdir2/file5.txt":  true,
		}

		visited := make(map[string]bool)
		err = fstools.Walk(fs, testpath, func(walkPath string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			visited[walkPath] = true
			return nil
		})
		if err != nil {
			t.Errorf("Walk failed: %v", err)
		}

		// Check all expected paths were visited
		for p := range expectedPaths {
			if !visited[p] {
				t.Errorf("expected path not visited: %s", p)
			}
		}

		// Check we visited the expected number of paths (4 dirs + 5 files = 9)
		if len(visited) < 9 {
			t.Errorf("visited %d paths, expected at least 9", len(visited))
		}
	})

	t.Run("FastWalk", func(t *testing.T) {
		// Expected: 4 directories + 5 files = 9 entries
		expectedPaths := map[string]bool{
			"/testdir":                    true,
			"/testdir/subdir1":            true,
			"/testdir/subdir2":            true,
			"/testdir/subdir1/nested":     true,
			"/testdir/file1.txt":          true,
			"/testdir/file2.txt":          true,
			"/testdir/subdir1/file3.txt":  true,
			"/testdir/subdir1/nested/file4.txt": true,
			"/testdir/subdir2/file5.txt":  true,
		}

		visited := make(map[string]bool)
		x := sync.Mutex{}
		err = fstools.FastWalk(fs, testpath, func(walkPath string, d iofs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			x.Lock()
			defer x.Unlock()
			visited[walkPath] = true
			return nil
		})
		if err != nil {
			t.Errorf("FastWalk failed: %v", err)
		}

		// Check all expected paths were visited
		for p := range expectedPaths {
			if !visited[p] {
				t.Errorf("expected path not visited: %s", p)
			}
		}

		// Check we visited the expected number of paths (4 dirs + 5 files = 9)
		if len(visited) < 9 {
			t.Errorf("visited %d paths, expected at least 9", len(visited))
		}
	})
}

func TestMemFS(t *testing.T) {
	fs, err := memfs.NewFS()
	if err != nil {
		t.Fatal(err)
	}

	if fs.TempDir() != "/tmp" {
		t.Fatalf("wrong TempDir output: %q != %q", fs.TempDir(), "/tmp")
	}
	fs.Tempdir = os.TempDir()
	if fs.TempDir() != os.TempDir() {
		t.Fatalf("wrong TempDir output: %q != %q", fs.TempDir(), os.TempDir())
	}

	testdir := fs.TempDir()
	timestr := time.Now().Format(time.RFC3339)
	testdir = path.Join(testdir, fmt.Sprintf("fstesting%s", timestr))

	err = fs.MkdirAll(testdir, 0777)
	if err != nil {
		t.Fatal(err)
	}
	defer fs.RemoveAll(fs.TempDir())

	cwd, err := fs.Getwd()
	if cwd != "/" {
		t.Fatalf("incorrect cwd %q", cwd)
	}
	err = fs.Chdir(testdir)
	if err != nil {
		t.Fatal(err)
	}

	// Old fstesting.AutoTest API has been removed.
	// Use TestMemFSSuite for comprehensive testing with new fstesting.Suite API.
}

func TestMkdir(t *testing.T) {

	fs, err := memfs.NewFS()
	if err != nil {
		t.Fatal(err)
	}

	if fs.TempDir() != "/tmp" {
		t.Fatalf("wrong TempDir output: %q != %q", fs.TempDir(), "/tmp")
	}

	fs.Tempdir = os.TempDir()
	if fs.TempDir() != os.TempDir() {
		t.Fatalf("wrong TempDir output: %q != %q", fs.TempDir(), os.TempDir())
	}

	testdir := fs.TempDir()

	t.Logf("Test path: %q", testdir)
	err = fs.MkdirAll(testdir, 0777)
	if err != nil {
		t.Fatal(err)
	}

	var list []string
	currentPath := "/"
outer:
	for _, name := range strings.Split(testdir, "/")[1:] {
		if name == "" {
			continue
		}
		f, err := fs.Open(currentPath)
		if err != nil {
			t.Fatal(err)
		}
		list, err = f.Readdirnames(-1)
		f.Close()
		if err != nil {
			t.Fatal(err)
		}
		for _, n := range list {
			if n == name {
				currentPath = path.Join(currentPath, name)
				continue outer
			}
		}
		t.Errorf("path error: %q + %q:  %s", currentPath, name, list)
	}

}

func TestOpenWrite(t *testing.T) {
	fs, err := memfs.NewFS()
	if err != nil {
		t.Fatal(err)
	}

	f, err := fs.Create("/test_file.txt")
	if err != nil {
		t.Fatal(err)
	}

	data := []byte("The quick brown fox jumped over the lazy dog.\n")
	n, err := f.Write(data)
	f.Close()
	if n != len(data) {
		t.Errorf("write error: wrong byte count %d, expected %d", n, len(data))
	}
	if err != nil {
		t.Fatal(err)
	}

	f, err = fs.Open("/test_file.txt")
	if err != nil {
		t.Fatal(err)
	}
	buff := make([]byte, 512)
	n, err = f.Read(buff)
	f.Close()
	if n != len(data) {
		t.Errorf("write error: wrong byte count %d, expected %d", n, len(data))
	}
	if err != nil {
		t.Fatal(err)
	}
	buff = buff[:n]
	if bytes.Compare(data, buff) != 0 {
		t.Log(string(data))
		t.Log(string(buff))

		t.Fatal("bytes written do not compare to bytes read")
	}

}

// TestRemoveEmptyDirectory verifies that Remove works on empty directories.
// This tests that the internal "." and ".." entries don't prevent removal.
func TestRemoveEmptyDirectory(t *testing.T) {
	fs, err := memfs.NewFS()
	if err != nil {
		t.Fatal(err)
	}

	// Create an empty directory
	err = fs.Mkdir("/emptydir", 0755)
	if err != nil {
		t.Fatal(err)
	}

	// Verify Readdir returns 0 entries (matching os behavior - no . or ..)
	f, err := fs.Open("/emptydir")
	if err != nil {
		t.Fatal(err)
	}
	entries, err := f.Readdir(-1)
	f.Close()
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Errorf("Expected 0 entries in empty directory, got %d", len(entries))
	}

	// Remove should succeed on empty directory
	err = fs.Remove("/emptydir")
	if err != nil {
		t.Errorf("Remove on empty directory failed: %v", err)
	}

	// Verify directory is gone
	_, err = fs.Stat("/emptydir")
	if !os.IsNotExist(err) {
		t.Errorf("Directory should not exist after Remove, got err: %v", err)
	}
}

// TestRemoveNonEmptyDirectory verifies that Remove fails on non-empty directories.
func TestRemoveNonEmptyDirectory(t *testing.T) {
	fs, err := memfs.NewFS()
	if err != nil {
		t.Fatal(err)
	}

	// Create a directory with a file
	err = fs.Mkdir("/nonempty", 0755)
	if err != nil {
		t.Fatal(err)
	}
	f, err := fs.Create("/nonempty/file.txt")
	if err != nil {
		t.Fatal(err)
	}
	f.Close()

	// Remove should fail
	err = fs.Remove("/nonempty")
	if err == nil {
		t.Error("Remove on non-empty directory should fail")
	}

	// Directory should still exist
	_, err = fs.Stat("/nonempty")
	if err != nil {
		t.Errorf("Directory should still exist: %v", err)
	}
}

// TestOpenFileAppend verifies that O_APPEND works correctly.
// Writes to files opened with O_APPEND should always append to the end.
func TestOpenFileAppend(t *testing.T) {
	fs, err := memfs.NewFS()
	if err != nil {
		t.Fatal(err)
	}

	// Create initial file with content
	f, err := fs.Create("/append.txt")
	if err != nil {
		t.Fatal(err)
	}
	f.Write([]byte("original\n"))
	f.Close()

	// Open for append
	f, err = fs.OpenFile("/append.txt", os.O_WRONLY|os.O_APPEND, 0)
	if err != nil {
		t.Fatalf("failed to open for append: %v", err)
	}

	// Write should append, not overwrite
	_, err = f.Write([]byte("appended\n"))
	if err != nil {
		t.Fatalf("write failed: %v", err)
	}
	f.Close()

	// Read back and verify
	f, _ = fs.Open("/append.txt")
	data := make([]byte, 100)
	n, _ := f.Read(data)
	f.Close()

	content := string(data[:n])
	expected := "original\nappended\n"
	if content != expected {
		t.Errorf("O_APPEND failed: expected %q, got %q", expected, content)
	}
}

// TestOpenFileAppendMultipleWrites verifies multiple appends work correctly.
func TestOpenFileAppendMultipleWrites(t *testing.T) {
	fs, err := memfs.NewFS()
	if err != nil {
		t.Fatal(err)
	}

	// Create file and open for append
	f, err := fs.OpenFile("/multi.txt", os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		t.Fatal(err)
	}

	// Multiple writes
	f.Write([]byte("line1\n"))
	f.Write([]byte("line2\n"))
	f.Write([]byte("line3\n"))
	f.Close()

	// Verify
	f, _ = fs.Open("/multi.txt")
	data := make([]byte, 100)
	n, _ := f.Read(data)
	f.Close()

	expected := "line1\nline2\nline3\n"
	if string(data[:n]) != expected {
		t.Errorf("Multiple appends failed: expected %q, got %q", expected, string(data[:n]))
	}
}
