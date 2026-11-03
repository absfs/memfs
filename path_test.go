package memfs

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"syscall"
	"testing"
)

func newTestFS(t *testing.T) *FileSystem {
	t.Helper()
	fs, err := NewFS()
	if err != nil {
		t.Fatal(err)
	}
	return fs
}

func mustDo(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

func touch(t *testing.T, fs *FileSystem, name string) {
	t.Helper()
	f, err := fs.Create(name)
	mustDo(t, err)
	mustDo(t, f.Close())
}

func wantErrno(t *testing.T, what string, err error, want error) {
	t.Helper()
	if !errors.Is(err, want) {
		t.Errorf("%s: got %v, want %v", what, err, want)
	}
}

func TestWalkSymlinks(t *testing.T) {
	fs := newTestFS(t)
	mustDo(t, fs.MkdirAll("/a/b", 0755))
	touch(t, fs, "/a/b/file")
	touch(t, fs, "/a/marker")
	mustDo(t, fs.Symlink("/a/b", "/abs"))  // absolute target
	mustDo(t, fs.Symlink("a/b", "/rel"))   // relative target
	mustDo(t, fs.Symlink("rel", "/chain")) // link to a link
	mustDo(t, fs.Symlink("loop2", "/loop1"))
	mustDo(t, fs.Symlink("loop1", "/loop2"))
	mustDo(t, fs.Symlink("", "/empty"))

	for _, p := range []string{"/abs/file", "/rel/file", "/chain/file", "chain/file", "/a/./b/file", "/a/b/", "//a//b//file"} {
		if _, err := fs.Stat(p); err != nil {
			t.Errorf("Stat(%q): %v", p, err)
		}
	}

	// ".." after a symlink is the parent of the link's target, as on Unix.
	if _, err := fs.Stat("/chain/../marker"); err != nil {
		t.Errorf("Stat through symlink then ..: %v", err)
	}

	// Lstat does not follow the final link, but does follow earlier ones.
	fi, err := fs.Lstat("/chain")
	mustDo(t, err)
	if fi.Mode()&os.ModeSymlink == 0 {
		t.Errorf("Lstat(/chain) mode %v, want symlink", fi.Mode())
	}

	_, err = fs.Stat("/loop1")
	wantErrno(t, "Stat(/loop1)", err, syscall.ELOOP)
	_, err = fs.Stat("/loop1/x")
	wantErrno(t, "Stat(/loop1/x)", err, syscall.ELOOP)
	_, err = fs.Stat("/empty")
	wantErrno(t, "Stat(/empty)", err, syscall.ENOENT)
	_, err = fs.Stat("")
	wantErrno(t, "Stat(\"\")", err, syscall.ENOENT)
	_, err = fs.Stat("/a/b/file/../file")
	wantErrno(t, "Stat(file/..)", err, errNotDir)
}

func TestRootAndDotNames(t *testing.T) {
	fs := newTestFS(t)
	mustDo(t, fs.Mkdir("/d", 0755))

	wantErrno(t, "Mkdir(\"\")", fs.Mkdir("", 0755), syscall.ENOENT)
	wantErrno(t, "Mkdir(/)", fs.Mkdir("/", 0755), syscall.EEXIST)
	wantErrno(t, "Mkdir(/d/.)", fs.Mkdir("/d/.", 0755), syscall.EEXIST)
	wantErrno(t, "Remove(/)", fs.Remove("/"), syscall.EINVAL)
	wantErrno(t, "Remove(/d/.)", fs.Remove("/d/."), syscall.EINVAL)
	wantErrno(t, "RemoveAll(/)", fs.RemoveAll("/"), syscall.EINVAL)
	wantErrno(t, "RemoveAll(/d/..)", fs.RemoveAll("/d/.."), syscall.EINVAL)
	wantErrno(t, "Symlink(x, /)", fs.Symlink("x", "/"), syscall.EEXIST)
	wantErrno(t, "Rename(/d/., /x)", fs.Rename("/d/.", "/x"), syscall.EINVAL)
	wantErrno(t, "Rename(/d, /x/..)", fs.Rename("/d", "/d/.."), syscall.EINVAL)
	wantErrno(t, "Rename(/, /x)", fs.Rename("/", "/x"), syscall.EINVAL)

	if fi, err := fs.Stat("/d"); err != nil || !fi.IsDir() {
		t.Fatalf("/d damaged: %v %v", fi, err)
	}
	mustDo(t, fs.MkdirAll("/", 0755))
	mustDo(t, fs.MkdirAll("", 0755))
}

func TestRenameDirectories(t *testing.T) {
	fs := newTestFS(t)
	mustDo(t, fs.MkdirAll("/a/sub/deep", 0755))
	mustDo(t, fs.Mkdir("/b", 0755))
	touch(t, fs, "/b/marker")
	touch(t, fs, "/file")

	// A directory cannot be moved beneath itself.
	wantErrno(t, "Rename into self", fs.Rename("/a", "/a/sub/deep/a"), syscall.EINVAL)
	wantErrno(t, "Rename onto child", fs.Rename("/a/sub", "/a/sub/x"), syscall.EINVAL)
	wantErrno(t, "Rename missing", fs.Rename("/nope", "/x"), syscall.ENOENT)
	wantErrno(t, "Rename onto existing", fs.Rename("/file", "/b/marker"), syscall.EEXIST)
	wantErrno(t, "Rename from through file", fs.Rename("/file/x", "/y"), errNotDir)
	wantErrno(t, "Rename to under file", fs.Rename("/b/marker", "/file/x"), errRenameParentNotDir)
	wantErrno(t, "Rename to through file", fs.Rename("/b/marker", "/file/x/y"), errNotDir)

	// Moving a directory to a new parent updates its "..".
	mustDo(t, fs.Rename("/a/sub", "/b/sub"))
	if _, err := fs.Stat("/b/sub/deep/../../marker"); err != nil {
		t.Errorf("'..' of moved directory not updated: %v", err)
	}
	if _, err := fs.Stat("/a/sub"); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("source still present: %v", err)
	}

	// Renaming within the same directory, and through a symlinked parent.
	mustDo(t, fs.Rename("/b/sub", "/b/sub2"))
	mustDo(t, fs.Symlink("/b", "/blink"))
	mustDo(t, fs.Rename("/blink/sub2", "/blink/sub3"))
	if _, err := fs.Stat("/b/sub3/deep"); err != nil {
		t.Errorf("rename through symlinked parent: %v", err)
	}

	// A detached directory's ".." chain ends without reaching the root.
	mustDo(t, fs.Chdir("/b/sub3/deep"))
	mustDo(t, fs.RemoveAll("/b/sub3"))
	if fs.isWithin(fs.dir, fs.root) {
		t.Error("detached directory reported within root")
	}
}

func TestMkdirAllPaths(t *testing.T) {
	fs := newTestFS(t)
	mustDo(t, fs.MkdirAll("/a/b", 0755))
	mustDo(t, fs.MkdirAll("/a/b", 0755)) // already there
	mustDo(t, fs.Chdir("/a"))
	mustDo(t, fs.MkdirAll("c/d/", 0755)) // relative, trailing slash
	mustDo(t, fs.MkdirAll("../a/./c//e", 0755))
	for _, p := range []string{"/a/c/d", "/a/c/e"} {
		if fi, err := fs.Stat(p); err != nil || !fi.IsDir() {
			t.Errorf("Stat(%q) = %v, %v", p, fi, err)
		}
	}

	// Through a symlink to a directory.
	mustDo(t, fs.Symlink("/a", "/alink"))
	mustDo(t, fs.MkdirAll("/alink/x/y", 0755))
	if _, err := fs.Stat("/a/x/y"); err != nil {
		t.Errorf("MkdirAll through symlink: %v", err)
	}

	// A dangling symlink component cannot be made a directory.
	mustDo(t, fs.Symlink("/nowhere", "/dangling"))
	wantErrno(t, "MkdirAll(dangling)", fs.MkdirAll("/dangling/x", 0755), syscall.EEXIST)

	// A symlink loop component: as with os.MkdirAll, the error is the one
	// Mkdir gives for the unresolvable component.
	mustDo(t, fs.Symlink("/l2", "/l1"))
	mustDo(t, fs.Symlink("/l1", "/l2"))
	wantErrno(t, "MkdirAll(loop)", fs.MkdirAll("/l1/x", 0755), syscall.EEXIST)
	if err := fs.Mkdir("/l1/", 0755); err == nil {
		t.Error("Mkdir(loop/) succeeded")
	}

	// "dir/." exists, although Mkdir cannot create it.
	mustDo(t, fs.MkdirAll("/a/.", 0755))
}

func TestOpenFileDanglingSymlink(t *testing.T) {
	fs := newTestFS(t)
	mustDo(t, fs.Symlink("/nowhere", "/dangling"))
	_, err := fs.OpenFile("/dangling", os.O_RDWR|os.O_CREATE|os.O_EXCL, 0644)
	wantErrno(t, "O_CREATE|O_EXCL on dangling", err, syscall.EEXIST)

	// Without O_EXCL the file is created where the link points, as os does,
	// through a chain of links, relative ones resolved from the link's
	// directory.
	mustDo(t, fs.Mkdir("/d", 0755))
	mustDo(t, fs.Symlink("../dangling", "/d/rel"))
	f, err := fs.OpenFile("d/rel", os.O_RDWR|os.O_CREATE, 0644)
	mustDo(t, err)
	if _, err := f.Write([]byte("hi")); err != nil {
		t.Fatal(err)
	}
	mustDo(t, f.Close())
	if b, err := fs.ReadFile("/nowhere"); err != nil || string(b) != "hi" {
		t.Errorf("/nowhere = %q, %v; want \"hi\"", b, err)
	}
	if target, err := fs.Readlink("/dangling"); err != nil || target != "/nowhere" {
		t.Errorf("Readlink(/dangling) = %q, %v", target, err)
	}

	// A dangling link whose target lies under a missing directory, or ends
	// in a slash, or that is empty, still fails.
	mustDo(t, fs.Symlink("/missing/x", "/deep"))
	_, err = fs.OpenFile("/deep", os.O_RDWR|os.O_CREATE, 0644)
	wantErrno(t, "O_CREATE on link into missing dir", err, syscall.ENOENT)
	mustDo(t, fs.Symlink("/slashed/", "/slash"))
	_, err = fs.OpenFile("/slash", os.O_RDWR|os.O_CREATE, 0644)
	wantErrno(t, "O_CREATE on link to name/", err, errSlashCreate())
	mustDo(t, fs.Symlink("", "/empty"))
	_, err = fs.OpenFile("/empty", os.O_RDWR|os.O_CREATE, 0644)
	wantErrno(t, "O_CREATE on empty link", err, syscall.ENOENT)

	_, err = fs.OpenFile("/missing/x", os.O_RDWR|os.O_CREATE, 0644)
	wantErrno(t, "O_CREATE with missing parent", err, syscall.ENOENT)
}

func TestTruncateAndChmodSemantics(t *testing.T) {
	fs := newTestFS(t)
	mustDo(t, fs.Mkdir("/d", 0755))
	touch(t, fs, "/d/f")
	mustDo(t, fs.Symlink("/d/f", "/flink"))

	wantErrno(t, "Truncate(dir)", fs.Truncate("/d", 0), syscall.EISDIR)
	wantErrno(t, "Truncate(missing)", fs.Truncate("/missing", 0), syscall.ENOENT)

	// Truncate follows symlinks.
	mustDo(t, fs.Truncate("/flink", 7))
	if fi, err := fs.Stat("/d/f"); err != nil || fi.Size() != 7 {
		t.Errorf("Truncate through symlink: %v, %v", fi, err)
	}

	// Chmod keeps the file type and follows symlinks.
	mustDo(t, fs.Chmod("/d", 0700))
	if fi, err := fs.Stat("/d"); err != nil || !fi.IsDir() || fi.Mode().Perm() != 0700 {
		t.Errorf("Chmod(/d): %v, %v", fi, err)
	}
	mustDo(t, fs.Chmod("/flink", 0600|os.ModeSetuid))
	if fi, _ := fs.Lstat("/flink"); fi.Mode()&os.ModeSymlink == 0 {
		t.Error("Chmod changed the symlink instead of its target")
	}
	if fi, _ := fs.Stat("/d/f"); fi.Mode() != 0600|os.ModeSetuid {
		t.Errorf("target mode = %v, want %v", fi.Mode(), 0600|os.ModeSetuid)
	}
}

func TestRelativeLinkOps(t *testing.T) {
	fs := newTestFS(t)
	mustDo(t, fs.Mkdir("/d", 0755))
	mustDo(t, fs.Chdir("/d"))
	mustDo(t, fs.Symlink("target", "link"))
	got, err := fs.Readlink("link")
	if err != nil || got != "target" {
		t.Errorf("Readlink(relative) = %q, %v", got, err)
	}
	mustDo(t, fs.Chdir(".."))
	if wd, _ := fs.Getwd(); wd != "/" {
		t.Errorf("Getwd after Chdir(..) = %q", wd)
	}
	touch(t, fs, "/file")
	wantErrno(t, "Chdir(file)", fs.Chdir("/file"), errChdirNotDir)
	_, err = fs.ReadDir("/file")
	var pe *os.PathError
	if !errors.As(err, &pe) || pe.Op != "open" || pe.Err != syscall.ENOTDIR {
		t.Errorf("ReadDir(file) = %v, want open ENOTDIR", err)
	}
}

// TestTrailingSlash checks the rules for "name/" under the Linux, BSD/macOS
// and Windows conventions, whichever platform the test runs on. The os
// comparison in notdir_test.go checks the running platform's own rules.
func TestTrailingSlash(t *testing.T) {
	saved := slashStyle
	defer func() { slashStyle = saved }()

	stat := func(name string) func(fs *FileSystem) error {
		return func(fs *FileSystem) error { _, err := fs.Stat(name); return err }
	}
	open := func(name string, flag int) func(fs *FileSystem) error {
		return func(fs *FileSystem) error {
			f, err := fs.OpenFile(name, flag, 0644)
			if err == nil {
				f.Close()
			}
			return err
		}
	}
	removeAll := func(name string, keepsSlash bool) func(fs *FileSystem) error {
		return func(fs *FileSystem) error {
			saved := removeAllKeepsSlash
			defer func() { removeAllKeepsSlash = saved }()
			removeAllKeepsSlash = keepsSlash
			return fs.RemoveAll(name)
		}
	}
	const (
		create = os.O_RDWR | os.O_CREATE
		excl   = os.O_RDWR | os.O_CREATE | os.O_EXCL
	)
	winDir := errDirectoryName
	cases := []struct {
		what          string
		run           func(fs *FileSystem) error
		bsd, lin, win error // nil means success
	}{
		// Resolving "file/" fails in every method.
		{"Stat(f/)", stat("/f/"), errNotDir, errNotDir, winDir},
		{"Stat(slashlink)", stat("/slashlink"), errNotDir, errNotDir, winDir},
		{"Stat(d/)", stat("/d/"), nil, nil, nil},
		{"Lstat(flink/)", func(fs *FileSystem) error { _, err := fs.Lstat("/flink/"); return err }, errNotDir, errNotDir, winDir},
		{"Lstat(dlink/)", func(fs *FileSystem) error { _, err := fs.Lstat("/dlink/"); return err }, nil, nil, nil},
		{"Readlink(flink/)", func(fs *FileSystem) error { _, err := fs.Readlink("/flink/"); return err }, errNotDir, errNotDir, winDir},
		{"Open(f/)", open("/f/", os.O_RDONLY), errNotDir, errNotDir, winDir},
		{"Open(d/)", open("/d/", os.O_RDONLY), nil, nil, nil},
		{"Chmod(f/)", func(fs *FileSystem) error { return fs.Chmod("/f/", 0600) }, errNotDir, errNotDir, winDir},
		{"Truncate(f/)", func(fs *FileSystem) error { return fs.Truncate("/f/", 0) }, errNotDir, errNotDir, winDir},
		{"Truncate(d/)", func(fs *FileSystem) error { return fs.Truncate("/d/", 0) }, syscall.EISDIR, syscall.EISDIR, errNotDir},

		{"Remove(f/)", func(fs *FileSystem) error { return fs.Remove("/f/") }, errNotDir, errNotDir, winDir},
		{"Remove(flink/)", func(fs *FileSystem) error { return fs.Remove("/flink/") }, errNotDir, errNotDir, winDir},
		{"Remove(dlink/)", func(fs *FileSystem) error { return fs.Remove("/dlink/") }, errNotDir, errNotDir, nil},
		{"Remove(e/)", func(fs *FileSystem) error { return fs.Remove("/e/") }, nil, nil, nil},
		{"RemoveAll(f/) since Go 1.25", removeAll("/f/", false), nil, nil, nil},
		{"RemoveAll(f/) before Go 1.25", removeAll("/f/", true), nil, nil, winDir},
		{"RemoveAll(new/)", func(fs *FileSystem) error { return fs.RemoveAll("/new/") }, nil, nil, nil},

		{"Rename(f/, g)", func(fs *FileSystem) error { return fs.Rename("/f/", "/g") }, errNotDir, errNotDir, winDir},
		{"Rename(dlink/, g)", func(fs *FileSystem) error { return fs.Rename("/dlink/", "/g") }, errNotDir, errNotDir, nil},
		{"Rename(f, flink/)", func(fs *FileSystem) error { return fs.Rename("/f", "/flink/") }, errNotDir, errNotDir, winDir},
		{"Rename(e, f/)", func(fs *FileSystem) error { return fs.Rename("/e", "/f/") }, errNotDir, errNotDir, winDir},
		{"Rename(f, dlink/)", func(fs *FileSystem) error { return fs.Rename("/f", "/dlink/") }, syscall.EEXIST, syscall.EEXIST, syscall.EEXIST},
		{"Rename(e/, new/)", func(fs *FileSystem) error { return fs.Rename("/e/", "/new/") }, nil, nil, nil},
		{"Rename(f, new/)", func(fs *FileSystem) error { return fs.Rename("/f", "/new/") }, syscall.ENOENT, errNotDir, nil},

		{"Mkdir(new/)", func(fs *FileSystem) error { return fs.Mkdir("/new/", 0755) }, nil, nil, nil},
		{"MkdirAll(new/x/)", func(fs *FileSystem) error { return fs.MkdirAll("/new/x/", 0755) }, nil, nil, nil},
		{"Mkdir(d/)", func(fs *FileSystem) error { return fs.Mkdir("/d/", 0755) }, syscall.EEXIST, syscall.EEXIST, syscall.EEXIST},
		{"Mkdir(f/)", func(fs *FileSystem) error { return fs.Mkdir("/f/", 0755) }, errNotDir, syscall.EEXIST, syscall.EEXIST},
		{"MkdirAll(f/)", func(fs *FileSystem) error { return fs.MkdirAll("/f/", 0755) }, errNotDir, syscall.EEXIST, syscall.EEXIST},

		{"Symlink(x, d/)", func(fs *FileSystem) error { return fs.Symlink("x", "/d/") }, syscall.EEXIST, syscall.EEXIST, syscall.EEXIST},
		{"Symlink(x, flink/)", func(fs *FileSystem) error { return fs.Symlink("x", "/flink/") }, errNotDir, syscall.EEXIST, winDir},
		{"Symlink(x, new/)", func(fs *FileSystem) error { return fs.Symlink("x", "/new/") }, syscall.ENOENT, syscall.ENOENT, winDir},

		{"Create(f/x/)", open("/f/x/", create), errNotDir, errNotDir, errNotDir},
		{"Create(f/)", open("/f/", create), errNotDir, syscall.EISDIR, winDir},
		{"Create(new/)", open("/new/", create), syscall.ENOENT, syscall.EISDIR, winDir},
		{"Create(d/)", open("/d/", create), syscall.EISDIR, syscall.EISDIR, errNotDir},
		{"OpenFile(d/, O_EXCL)", open("/d/", excl), syscall.EEXIST, syscall.EISDIR, errNotDir},
		{"OpenFile(new/, O_EXCL)", open("/new/", excl), syscall.ENOENT, syscall.EISDIR, winDir},
	}
	styles := []struct {
		name  string
		rules slashRules
	}{{"bsd", bsdSlash}, {"linux", linuxSlash}, {"windows", windowsSlash}}
	for _, st := range styles {
		slashStyle = st.rules
		for _, c := range cases {
			fs := newTestFS(t)
			touch(t, fs, "/f")
			mustDo(t, fs.Mkdir("/d", 0755))
			mustDo(t, fs.Mkdir("/e", 0755))
			mustDo(t, fs.Symlink("f", "/flink"))
			mustDo(t, fs.Symlink("d", "/dlink"))
			mustDo(t, fs.Symlink("f/", "/slashlink"))

			want := map[slashRules]error{bsdSlash: c.bsd, linuxSlash: c.lin, windowsSlash: c.win}[st.rules]
			what := st.name + " " + c.what
			err := c.run(fs)
			if want == nil {
				if err != nil {
					t.Errorf("%s: %v, want success", what, err)
				}
				continue
			}
			wantErrno(t, what, err, want)

			// Nothing that failed may have touched the file or the links,
			// and Readlink still returns a link's target verbatim.
			if fi, err := fs.Lstat("/f"); err != nil || fi.IsDir() {
				t.Errorf("%s: /f damaged: %v, %v", what, fi, err)
			}
			for link, target := range map[string]string{"/flink": "f", "/dlink": "d", "/slashlink": "f/"} {
				if got, err := fs.Readlink(link); err != nil || got != target {
					t.Errorf("%s: Readlink(%s) = %q, %v; want %q", what, link, got, err, target)
				}
			}
		}
	}
}

// TestChdirPhysical checks that Chdir through a symbolic link records the
// directory it reached, so that Getwd, "." and relative paths agree, as they
// do with os.
func TestChdirPhysical(t *testing.T) {
	for _, steps := range [][]string{{"/l/.."}, {"l/.."}, {"/l", ".."}, {"/l", "../b/.."}} {
		fs := newTestFS(t)
		mustDo(t, fs.MkdirAll("/a/b", 0755))
		mustDo(t, fs.Symlink("/a/b", "/l"))
		for _, s := range steps {
			mustDo(t, fs.Chdir(s))
		}
		if wd, _ := fs.Getwd(); wd != "/a" {
			t.Errorf("%v: Getwd = %q, want /a", steps, wd)
		}
		touch(t, fs, "x")
		if _, err := fs.Stat("/a/x"); err != nil {
			t.Errorf("%v: Create(x) did not create /a/x: %v", steps, err)
		}
		if _, err := fs.Stat("b"); err != nil {
			t.Errorf("%v: Stat(b): %v", steps, err)
		}
		entries, err := fs.ReadDir(".")
		mustDo(t, err)
		var names []string
		for _, e := range entries {
			names = append(names, e.Name())
		}
		if fmt.Sprint(names) != "[b x]" {
			t.Errorf("%v: ReadDir(.) = %v, want [b x]", steps, names)
		}
	}

	// Chdir into the link itself reports the target, like a physical getwd.
	fs := newTestFS(t)
	mustDo(t, fs.MkdirAll("/a/b", 0755))
	mustDo(t, fs.Symlink("a/b", "/l"))
	mustDo(t, fs.Chdir("/l"))
	if wd, _ := fs.Getwd(); wd != "/a/b" {
		t.Errorf("Getwd after Chdir(/l) = %q, want /a/b", wd)
	}
	mustDo(t, fs.Chdir("/"))
	if wd, _ := fs.Getwd(); wd != "/" {
		t.Errorf("Getwd after Chdir(/) = %q", wd)
	}
}

// TestChdirPhysicalMatchesOS repeats the reviewer's sequence on disk.
func TestChdirPhysicalMatchesOS(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows resolves .. lexically")
	}
	root, err := filepath.EvalSymlinks(t.TempDir())
	mustDo(t, err)
	mustDo(t, os.MkdirAll(filepath.Join(root, "a", "b"), 0755))
	if err := os.Symlink(filepath.Join(root, "a", "b"), filepath.Join(root, "l")); err != nil {
		t.Skipf("os symlinks unavailable: %v", err)
	}
	wd, err := os.Getwd()
	mustDo(t, err)
	defer os.Chdir(wd)
	mustDo(t, os.Chdir(filepath.Join(root, "l")))
	mustDo(t, os.Chdir(".."))
	osWd, err := os.Getwd()
	mustDo(t, err)
	osWd, err = filepath.EvalSymlinks(osWd)
	mustDo(t, err)

	fs := newTestFS(t)
	mustDo(t, fs.MkdirAll("/a/b", 0755))
	mustDo(t, fs.Symlink("/a/b", "/l"))
	mustDo(t, fs.Chdir("/l"))
	mustDo(t, fs.Chdir(".."))
	memWd, _ := fs.Getwd()

	rel, err := filepath.Rel(root, osWd)
	mustDo(t, err)
	if want := "/" + filepath.ToSlash(rel); memWd != want {
		t.Errorf("memfs Getwd = %q, os Getwd = %q (relative %q)", memWd, osWd, want)
	}
}

func TestPathOfUnlinked(t *testing.T) {
	fs := newTestFS(t)
	orphan := fs.ino.NewDir(0755)
	if _, ok := fs.pathOf(orphan); ok {
		t.Error("pathOf(orphan without ..) succeeded")
	}
	orphan.Link("..", fs.root)
	if _, ok := fs.pathOf(orphan); ok {
		t.Error("pathOf(orphan not in its parent) succeeded")
	}
}
