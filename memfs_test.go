package memfs_test

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/absfs/absfs"
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
		err = fs.Walk(testpath, func(walkPath string, info os.FileInfo, err error) error {
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
		err = fs.FastWalk(testpath, func(walkPath string, mode os.FileMode) error {
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
	testdir = filepath.Join(testdir, fmt.Sprintf("fstesting%s", timestr))

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
	path := "/"
outer:
	for _, name := range strings.Split(testdir, "/")[1:] {
		if name == "" {
			continue
		}
		f, err := fs.Open(path)
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
				path = filepath.Join(path, name)
				continue outer
			}
		}
		t.Errorf("path error: %q + %q:  %s", path, name, list)
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

func TestSymlinkCycleDetection(t *testing.T) {
	// Test 1: Self-referencing symlink (A -> A)
	t.Run("SelfReferencingSymlink", func(t *testing.T) {
		fs, err := memfs.NewFS()
		if err != nil {
			t.Fatal(err)
		}

		// Create a temporary target file
		f, err := fs.Create("/temp_target")
		if err != nil {
			t.Fatal(err)
		}
		f.Close()

		// Create symlink pointing to temp target
		err = fs.Symlink("/temp_target", "/self_link")
		if err != nil {
			t.Fatal(err)
		}

		// Update symlink to point to itself (this uses the update path in Symlink)
		err = fs.Symlink("/self_link", "/self_link")
		if err != nil {
			t.Fatal(err)
		}

		// Attempting to stat should detect the cycle
		_, err = fs.Stat("/self_link")
		if err == nil {
			t.Fatal("expected error for self-referencing symlink, got nil")
		}
		// Check that the error is ELOOP (too many levels of symbolic links)
		pathErr, ok := err.(*os.PathError)
		if !ok {
			t.Fatalf("expected *os.PathError, got %T", err)
		}
		if pathErr.Err.Error() != "too many levels of symbolic links" {
			t.Errorf("expected ELOOP error, got: %v", pathErr.Err)
		}
	})

	// Test 2: Two-node circular symlink (A -> B -> A)
	t.Run("TwoNodeCircularSymlink", func(t *testing.T) {
		fs, err := memfs.NewFS()
		if err != nil {
			t.Fatal(err)
		}

		// Create temporary target files
		f1, err := fs.Create("/temp1")
		if err != nil {
			t.Fatal(err)
		}
		f1.Close()

		f2, err := fs.Create("/temp2")
		if err != nil {
			t.Fatal(err)
		}
		f2.Close()

		// Create two symlinks initially pointing to temp files
		err = fs.Symlink("/temp1", "/link_a")
		if err != nil {
			t.Fatal(err)
		}

		err = fs.Symlink("/temp2", "/link_b")
		if err != nil {
			t.Fatal(err)
		}

		// Update to create cycle: A -> B
		err = fs.Symlink("/link_b", "/link_a")
		if err != nil {
			t.Fatal(err)
		}

		// B -> A (completing the cycle)
		err = fs.Symlink("/link_a", "/link_b")
		if err != nil {
			t.Fatal(err)
		}

		// Attempting to stat either should detect the cycle
		_, err = fs.Stat("/link_a")
		if err == nil {
			t.Fatal("expected error for circular symlink A, got nil")
		}

		_, err = fs.Stat("/link_b")
		if err == nil {
			t.Fatal("expected error for circular symlink B, got nil")
		}
	})

	// Test 3: Three-node circular symlink (A -> B -> C -> A)
	t.Run("ThreeNodeCircularSymlink", func(t *testing.T) {
		fs, err := memfs.NewFS()
		if err != nil {
			t.Fatal(err)
		}

		// Create temporary target files
		temps := []string{"/t1", "/t2", "/t3"}
		for _, name := range temps {
			f, err := fs.Create(name)
			if err != nil {
				t.Fatal(err)
			}
			f.Close()
		}

		// Create three symlinks initially pointing to temp files
		err = fs.Symlink("/t1", "/chain_a")
		if err != nil {
			t.Fatal(err)
		}

		err = fs.Symlink("/t2", "/chain_b")
		if err != nil {
			t.Fatal(err)
		}

		err = fs.Symlink("/t3", "/chain_c")
		if err != nil {
			t.Fatal(err)
		}

		// Update to create cycle: A -> B -> C -> A
		err = fs.Symlink("/chain_b", "/chain_a")
		if err != nil {
			t.Fatal(err)
		}

		err = fs.Symlink("/chain_c", "/chain_b")
		if err != nil {
			t.Fatal(err)
		}

		err = fs.Symlink("/chain_a", "/chain_c")
		if err != nil {
			t.Fatal(err)
		}

		// Attempting to stat any of them should detect the cycle
		_, err = fs.Stat("/chain_a")
		if err == nil {
			t.Fatal("expected error for circular symlink chain, got nil")
		}
	})

	// Test 4: Valid symlink chain (no cycle)
	t.Run("ValidSymlinkChain", func(t *testing.T) {
		fs, err := memfs.NewFS()
		if err != nil {
			t.Fatal(err)
		}

		// Create a file
		f, err := fs.Create("/real_file.txt")
		if err != nil {
			t.Fatal(err)
		}
		_, err = f.Write([]byte("test content"))
		if err != nil {
			t.Fatal(err)
		}
		f.Close()

		// Create symlink chain: link1 -> link2 -> real_file
		err = fs.Symlink("/real_file.txt", "/link2")
		if err != nil {
			t.Fatal(err)
		}

		err = fs.Symlink("/link2", "/link1")
		if err != nil {
			t.Fatal(err)
		}

		// This should work fine (no cycle)
		info, err := fs.Stat("/link1")
		if err != nil {
			t.Fatalf("expected no error for valid symlink chain, got: %v", err)
		}

		if info.Name() != "link1" {
			t.Errorf("expected name 'link1', got '%s'", info.Name())
		}

		if info.Size() != 12 {
			t.Errorf("expected size 12, got %d", info.Size())
		}
	})
}
