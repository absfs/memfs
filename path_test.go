package memfs

import (
	"errors"
	"os"
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
	wantErrno(t, "Rename to through file", fs.Rename("/b/marker", "/file/x"), errNotDir)

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

	// A symlink loop component.
	mustDo(t, fs.Symlink("/l2", "/l1"))
	mustDo(t, fs.Symlink("/l1", "/l2"))
	wantErrno(t, "MkdirAll(loop)", fs.MkdirAll("/l1/x", 0755), syscall.ELOOP)
}

func TestOpenFileDanglingSymlink(t *testing.T) {
	fs := newTestFS(t)
	mustDo(t, fs.Symlink("/nowhere", "/dangling"))
	_, err := fs.OpenFile("/dangling", os.O_RDWR|os.O_CREATE, 0644)
	wantErrno(t, "O_CREATE on dangling", err, syscall.ENOENT)
	_, err = fs.OpenFile("/dangling", os.O_RDWR|os.O_CREATE|os.O_EXCL, 0644)
	wantErrno(t, "O_CREATE|O_EXCL on dangling", err, syscall.EEXIST)
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
	wantErrno(t, "Chdir(file)", fs.Chdir("/file"), syscall.ENOTDIR)
	_, err = fs.ReadDir("/file")
	var pe *os.PathError
	if !errors.As(err, &pe) || pe.Op != "open" || pe.Err != syscall.ENOTDIR {
		t.Errorf("ReadDir(file) = %v, want open ENOTDIR", err)
	}
}
