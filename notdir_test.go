package memfs

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"syscall"
	"testing"
	"time"
)

// The tests in this file compare memfs with the os package on a temporary
// directory for every path-taking method, focusing on paths that run through
// something that is not a directory ("inside.txt/x"). On Unix such a path
// fails with ENOTDIR; on Windows the os package mostly reports
// ERROR_PATH_NOT_FOUND (which is syscall.ENOTDIR there, and which errors.Is
// treats as fs.ErrNotExist), with exceptions for Chdir and Rename. memfs must
// agree on whichever platform the tests run on.

// fixture builds the same tree in memfs and on disk:
//
//	inside.txt          regular file
//	d/                  directory
//	d/f.txt             regular file
//	flink -> inside.txt symlink to a file  (when the OS allows symlinks)
//	dlink -> d          symlink to a directory
type fixture struct {
	mem      *FileSystem
	root     string // os temp dir
	symlinks bool   // whether os symlinks could be created
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	mem, err := NewFS()
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	fx := &fixture{mem: mem, root: root}
	// Chmod("d/") may clear d's search bit; restore it so that the temp
	// dir can be removed. Cleanups run last first, so this precedes it.
	t.Cleanup(func() { os.Chmod(fx.osPath("d"), 0755) })

	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	writeMem := func(name string) {
		t.Helper()
		f, err := mem.Create(name)
		must(err)
		_, err = f.Write([]byte("data"))
		must(err)
		must(f.Close())
	}

	must(os.WriteFile(fx.osPath("inside.txt"), []byte("data"), 0644))
	must(os.Mkdir(fx.osPath("d"), 0755))
	must(os.WriteFile(fx.osPath("d/f.txt"), []byte("data"), 0644))
	writeMem("/inside.txt")
	must(mem.Mkdir("/d", 0755))
	writeMem("/d/f.txt")

	if os.Symlink("inside.txt", fx.osPath("flink")) == nil &&
		os.Symlink("d", fx.osPath("dlink")) == nil {
		fx.symlinks = true
		must(mem.Symlink("inside.txt", "/flink"))
		must(mem.Symlink("d", "/dlink"))
	}
	return fx
}

// osPath maps a slash-separated path relative to the fixture root onto the
// temp dir without cleaning it, so "inside.txt/.." reaches the OS intact.
func (fx *fixture) osPath(p string) string {
	return fx.root + string(os.PathSeparator) + filepath.FromSlash(p)
}

// pathOp runs one method against memfs and against os. Paths are already
// mapped for the target; q is the second path for two-path methods.
type pathOp struct {
	name string
	mem  func(m *FileSystem, p, q string) error
	os   func(p, q string) error
	q    string // second path argument, relative to the fixture root
	skip string // GOOS on which the os side cannot be compared
	// creates is set for methods that create the final element. For them
	// a trailing slash on a non-directory gives a platform-specific error
	// (EEXIST, EISDIR or ENOENT on Linux), which only the os comparison
	// checks.
	creates bool
}

func closeOpen(f interface{ Close() error }, err error) error {
	if err != nil {
		return err
	}
	return f.Close()
}

var epoch = time.Unix(1_000_000_000, 0)

var pathOps = []pathOp{
	{name: "Mkdir", creates: true,
		mem: func(m *FileSystem, p, _ string) error { return m.Mkdir(p, 0755) },
		os:  func(p, _ string) error { return os.Mkdir(p, 0755) }},
	{name: "MkdirAll", creates: true,
		mem: func(m *FileSystem, p, _ string) error { return m.MkdirAll(p, 0755) },
		os:  func(p, _ string) error { return os.MkdirAll(p, 0755) }},
	{name: "Create", creates: true,
		mem: func(m *FileSystem, p, _ string) error { return closeOpen(m.Create(p)) },
		os:  func(p, _ string) error { return closeOpen(os.Create(p)) }},
	{name: "OpenFile(O_WRONLY|O_CREATE)", creates: true,
		mem: func(m *FileSystem, p, _ string) error { return closeOpen(m.OpenFile(p, os.O_WRONLY|os.O_CREATE, 0644)) },
		os:  func(p, _ string) error { return closeOpen(os.OpenFile(p, os.O_WRONLY|os.O_CREATE, 0644)) }},
	{name: "OpenFile(O_RDWR|O_CREATE|O_EXCL)", creates: true,
		mem: func(m *FileSystem, p, _ string) error {
			return closeOpen(m.OpenFile(p, os.O_RDWR|os.O_CREATE|os.O_EXCL, 0644))
		},
		os: func(p, _ string) error { return closeOpen(os.OpenFile(p, os.O_RDWR|os.O_CREATE|os.O_EXCL, 0644)) }},
	{name: "Open",
		mem: func(m *FileSystem, p, _ string) error { return closeOpen(m.Open(p)) },
		os:  func(p, _ string) error { return closeOpen(os.Open(p)) }},
	{name: "Stat",
		mem: func(m *FileSystem, p, _ string) error { _, err := m.Stat(p); return err },
		os:  func(p, _ string) error { _, err := os.Stat(p); return err }},
	{name: "Lstat",
		mem: func(m *FileSystem, p, _ string) error { _, err := m.Lstat(p); return err },
		os:  func(p, _ string) error { _, err := os.Lstat(p); return err }},
	{name: "Remove",
		mem: func(m *FileSystem, p, _ string) error { return m.Remove(p) },
		os:  func(p, _ string) error { return os.Remove(p) }},
	{name: "RemoveAll",
		mem: func(m *FileSystem, p, _ string) error { return m.RemoveAll(p) },
		os:  func(p, _ string) error { return os.RemoveAll(p) }},
	{name: "Rename(old)", q: "d/renamed",
		mem: func(m *FileSystem, p, q string) error { return m.Rename(p, q) },
		os:  func(p, q string) error { return os.Rename(p, q) }},
	{name: "Rename(new)", q: "d/f.txt", creates: true,
		mem: func(m *FileSystem, p, q string) error { return m.Rename(q, p) },
		os:  func(p, q string) error { return os.Rename(q, p) }},
	{name: "Symlink", creates: true, q: "d",
		mem: func(m *FileSystem, p, _ string) error { return m.Symlink("d", p) },
		os:  func(p, _ string) error { return os.Symlink("d", p) }},
	{name: "Readlink",
		mem: func(m *FileSystem, p, _ string) error { _, err := m.Readlink(p); return err },
		os:  func(p, _ string) error { _, err := os.Readlink(p); return err }},
	{name: "Chmod",
		mem: func(m *FileSystem, p, _ string) error { return m.Chmod(p, 0644) },
		os:  func(p, _ string) error { return os.Chmod(p, 0644) }},
	{name: "Chtimes",
		mem: func(m *FileSystem, p, _ string) error { return m.Chtimes(p, epoch, epoch) },
		os:  func(p, _ string) error { return os.Chtimes(p, epoch, epoch) }},
	// os.Chown and os.Lchown are not supported on Windows (EWINDOWS).
	{name: "Chown", skip: "windows",
		mem: func(m *FileSystem, p, _ string) error { return m.Chown(p, os.Getuid(), os.Getgid()) },
		os:  func(p, _ string) error { return os.Chown(p, os.Getuid(), os.Getgid()) }},
	{name: "Lchown", skip: "windows",
		mem: func(m *FileSystem, p, _ string) error { return m.Lchown(p, os.Getuid(), os.Getgid()) },
		os:  func(p, _ string) error { return os.Lchown(p, os.Getuid(), os.Getgid()) }},
	{name: "Truncate",
		mem: func(m *FileSystem, p, _ string) error { return m.Truncate(p, 0) },
		os:  func(p, _ string) error { return os.Truncate(p, 0) }},
	{name: "ReadDir",
		mem: func(m *FileSystem, p, _ string) error { _, err := m.ReadDir(p); return err },
		os:  func(p, _ string) error { _, err := os.ReadDir(p); return err }},
	{name: "Chdir",
		mem: func(m *FileSystem, p, _ string) error { return m.Chdir(p) },
		os: func(p, _ string) error {
			wd, err := os.Getwd()
			if err != nil {
				return err
			}
			defer os.Chdir(wd)
			return os.Chdir(p)
		}},
}

// pathCase is a path to try with every method.
type pathCase struct {
	path     string
	notDir   bool // runs through a non-directory: must fail with errNotDir
	symlink  bool // needs the fixture symlinks
	unixOnly bool // Windows resolves "." and ".." lexically, so os differs
}

var pathCases = []pathCase{
	{path: "inside.txt/x", notDir: true},
	{path: "inside.txt/x/y", notDir: true},
	{path: "d/f.txt/x", notDir: true},
	{path: "d/f.txt/x/y/z", notDir: true},
	{path: "flink/x", notDir: true, symlink: true},
	{path: "inside.txt/..", notDir: true, unixOnly: true},
	{path: "inside.txt/./x", notDir: true, unixOnly: true},
	{path: "inside.txt/", notDir: true},
	{path: "d/f.txt/", notDir: true},
	{path: "flink/", notDir: true, symlink: true},
	// Controls: these must keep agreeing with os after the fix.
	{path: "new/"},
	{path: "d/"},
	{path: "dlink/", symlink: true},
	{path: "missing/x"},
	{path: "d/new"},
	{path: "dlink/new", symlink: true},
	{path: "dlink/f.txt", symlink: true},
}

// knownDivergence lists method/path pairs where memfs deliberately differs
// from os for reasons unrelated to non-directory path components.
var knownDivergence = map[string]string{
	// memfs Rename refuses to replace an existing target; os replaces it
	// (here it is the same file, so os succeeds).
	"Rename(new) dlink/f.txt": "memfs Rename does not overwrite",
	// memfs Readlink returns ("", nil) for a file that is not a link; os
	// returns EINVAL.
	"Readlink dlink/f.txt": "memfs Readlink on a regular file",
	"Readlink d/":          "memfs Readlink on a directory",
	"Readlink dlink/":      "memfs Readlink on a directory",
}

// osDivergence lists further pairs that differ on one platform only.
var osDivergence = map[string]map[string]string{
	// With a trailing slash on a symbolic link to a directory, macOS removes
	// or renames the directory the link points to; memfs, like Linux, acts
	// on the link itself and so fails with ENOTDIR.
	"darwin": {
		"Remove dlink/":      "macOS follows the link",
		"Rename(old) dlink/": "macOS follows the link",
	},
	"windows": {
		// os on Windows replaces an existing file, ignoring the trailing
		// separator; memfs Rename never replaces.
		"Rename(new) inside.txt/": "memfs Rename does not overwrite",
		"Rename(new) d/f.txt/":    "memfs Rename does not overwrite",
		"Rename(new) flink/":      "memfs Rename does not overwrite",
		// Windows reports ERROR_ACCESS_DENIED for an existing directory as
		// the new name; memfs reports EEXIST.
		"Rename(new) d/":     "Windows denies access to a directory",
		"Rename(new) dlink/": "Windows denies access to a directory",
		"Symlink d/":         "Windows denies access to a directory",
		"Symlink dlink/":     "Windows denies access to a directory",
	},
}

// errorTargets are compared with errors.Is between memfs and os.
func errorTargets() []error {
	targets := []error{fs.ErrNotExist, fs.ErrExist, fs.ErrPermission}
	if runtime.GOOS != "windows" {
		// On Windows os returns Windows errnos, which never match these
		// portable syscall values, while memfs uses them throughout.
		targets = append(targets, syscall.ENOTDIR, syscall.ENOENT, syscall.EEXIST, syscall.EISDIR)
	}
	return targets
}

func compareErrors(t *testing.T, memErr, osErr error) {
	t.Helper()
	if (memErr == nil) != (osErr == nil) {
		t.Fatalf("memfs err = %v, os err = %v", memErr, osErr)
	}
	if memErr == nil {
		return
	}
	if mt, ot := reflect.TypeOf(memErr), reflect.TypeOf(osErr); mt != ot {
		t.Errorf("error type: memfs %v, os %v (memfs %v, os %v)", mt, ot, memErr, osErr)
	}
	for _, target := range errorTargets() {
		if m, o := errors.Is(memErr, target), errors.Is(osErr, target); m != o {
			t.Errorf("errors.Is(err, %v): memfs %v, os %v (memfs %v, os %v)", target, m, o, memErr, osErr)
		}
	}
	if m, o := os.IsNotExist(memErr), os.IsNotExist(osErr); m != o {
		t.Errorf("os.IsNotExist: memfs %v, os %v (memfs %v, os %v)", m, o, memErr, osErr)
	}
	if m, o := os.IsExist(memErr), os.IsExist(osErr); m != o {
		t.Errorf("os.IsExist: memfs %v, os %v (memfs %v, os %v)", m, o, memErr, osErr)
	}
}

// TestNotDirMatchesOS runs every path-taking method on every path case in a
// fresh fixture, both in memfs (absolute and relative path) and on disk, and
// requires the outcomes to agree.
func TestNotDirMatchesOS(t *testing.T) {
	for _, op := range pathOps {
		for _, pc := range pathCases {
			key := op.name + " " + pc.path
			t.Run(key, func(t *testing.T) {
				if op.skip == runtime.GOOS {
					t.Skipf("os.%s unsupported on %s", op.name, runtime.GOOS)
				}
				if pc.unixOnly && runtime.GOOS == "windows" {
					t.Skip("Windows resolves . and .. lexically")
				}
				if why, ok := knownDivergence[key]; ok {
					t.Skip(why)
				}
				if why, ok := osDivergence[runtime.GOOS][key]; ok {
					t.Skip(why)
				}

				for _, relative := range []bool{false, true} {
					fx := newFixture(t)
					if pc.symlink && !fx.symlinks {
						t.Skip("os symlinks unavailable")
					}
					osErr := op.os(fx.osPath(pc.path), fx.osPath(op.q))

					memPath, memQ := "/"+pc.path, "/"+op.q
					if relative {
						if err := fx.mem.Chdir("/d"); err != nil {
							t.Fatal(err)
						}
						memPath, memQ = "../"+pc.path, "../"+op.q
					}
					memErr := op.mem(fx.mem, memPath, memQ)
					compareErrors(t, memErr, osErr)

					slashCreate := op.creates && strings.HasSuffix(pc.path, "/")
					if pc.notDir && op.name != "MkdirAll" && !slashCreate && memErr != nil &&
						!errors.Is(memErr, errNotDir) && !errors.Is(memErr, errRenameParentNotDir) &&
						!errors.Is(memErr, errSlashNotDir()) {
						t.Errorf("relative=%v: memfs err %v is not errNotDir (%v)", relative, memErr, errNotDir)
					}
				}
			})
		}
	}
}

// TestMkdirAllThroughFile is the original report: MkdirAll succeeded when a
// path component was a regular file.
func TestMkdirAllThroughFile(t *testing.T) {
	fx := newFixture(t)
	for _, p := range []string{"/inside.txt/x", "/inside.txt", "/d/f.txt/x/y", "inside.txt/x"} {
		err := fx.mem.MkdirAll(p, 0755)
		var pe *os.PathError
		if !errors.As(err, &pe) || pe.Err != syscall.ENOTDIR {
			t.Errorf("MkdirAll(%q) = %v, want *PathError with syscall.ENOTDIR", p, err)
			continue
		}
		want := strings.TrimSuffix(strings.TrimSuffix(p, "/y"), "/x")
		if pe.Path != want {
			t.Errorf("MkdirAll(%q) error path = %q, want %q", p, pe.Path, want)
		}
	}
	if _, err := fx.mem.Stat("/inside.txt"); err != nil {
		t.Fatal(err)
	}
	if fi, _ := fx.mem.Stat("/inside.txt"); fi.IsDir() {
		t.Error("inside.txt turned into a directory")
	}
}

// TestCreateThroughDanglingLinkMatchesOS checks that O_CREATE without O_EXCL
// on a dangling symbolic link creates the link's target, as os does, and
// that O_EXCL fails with EEXIST.
func TestCreateThroughDanglingLinkMatchesOS(t *testing.T) {
	root := t.TempDir()
	if err := os.Symlink("gone.txt", filepath.Join(root, "dang")); err != nil {
		t.Skipf("os symlinks unavailable: %v", err)
	}
	mem := newTestFS(t)
	mustDo(t, mem.Symlink("gone.txt", "/dang"))

	osErr := closeOpen(os.OpenFile(filepath.Join(root, "dang"), os.O_RDWR|os.O_CREATE|os.O_EXCL, 0644))
	memErr := closeOpen(mem.OpenFile("/dang", os.O_RDWR|os.O_CREATE|os.O_EXCL, 0644))
	compareErrors(t, memErr, osErr)

	osErr = closeOpen(os.OpenFile(filepath.Join(root, "dang"), os.O_WRONLY|os.O_CREATE, 0644))
	memErr = closeOpen(mem.OpenFile("/dang", os.O_WRONLY|os.O_CREATE, 0644))
	compareErrors(t, memErr, osErr)
	_, osErr = os.Lstat(filepath.Join(root, "gone.txt"))
	_, memErr = mem.Lstat("/gone.txt")
	compareErrors(t, memErr, osErr)
}
