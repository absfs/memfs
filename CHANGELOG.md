# Changelog

## Unreleased

### Fixed

- `MkdirAll` no longer succeeds when a path component is a regular file
  (`MkdirAll("/inside.txt/x")`); it returns `syscall.ENOTDIR` naming the
  offending path, as `os.MkdirAll` does. `Mkdir` had the same bug.
- Every path-taking method now fails for a path that runs through a
  non-directory, with the error os gives on the running platform:
  `syscall.ENOTDIR`, which on Windows is `ERROR_PATH_NOT_FOUND` and so
  `fs.ErrNotExist`. As with os on Windows, `Chdir` to a file returns
  `ERROR_DIRECTORY` and `Rename` under a file returns
  `ERROR_INVALID_PARAMETER`. Previously such paths returned ENOENT, an
  untyped "not a directory" error (Create, Symlink), or nil (Mkdir,
  MkdirAll, Rename to a path under a file).
- A trailing slash on a name that is not a directory fails with ENOTDIR, as
  on Unix, instead of being ignored: `Remove("file/")` and
  `Rename("file/", x)` no longer delete or move the file, and `Create("new/")`
  no longer creates a regular file. On Windows the error is
  `ERROR_DIRECTORY`, as with os there. Where Linux, macOS and Windows give
  different errors, memfs gives the running platform's.
- `OpenFile` with `O_CREATE` (without `O_EXCL`) on a dangling symbolic link
  creates the link's target, as os does. It used to fail with ENOENT.
- `MkdirAll` follows `os.MkdirAll` step for step, so a dangling or looping
  link in the path fails with the error `Mkdir` gives for it (EEXIST), as in
  os.
- Symbolic links are now followed in intermediate path components, and `..`
  is resolved through the directory tree rather than lexically. `Chdir`
  stores the directory it reached, so `Getwd`, `.` and relative paths agree
  after `Chdir` through a link followed by `..`; `Getwd` reports the physical
  path, as `os.Getwd` does.
- `Rename` of a directory to a new parent updates its `..` entry, and moving a
  directory beneath itself fails with EINVAL.
- `Chmod` keeps the file type bits (it used to turn a directory into a regular
  file when given plain permission bits) and, like `Chown`, `Chtimes` and
  `Truncate`, follows symbolic links.
- `Truncate`, `Chmod`, `Chtimes`, `Chown`, `Lchown`, `Readlink` and `Lstat`
  return `*os.PathError` with the right operation name; `Symlink` returns
  `*os.LinkError`, as os does. `Readlink` resolves relative names against the
  working directory.
- `ReadDir` on a regular file fails at open with ENOTDIR, like `os.ReadDir`.
- `Stat` returns a nil `FileInfo` on error.

### Changed

- The `FileSystem` godoc no longer claims it is safe for concurrent use; as
  the README says, it is not. Wrap it with `lockfs` to share it.

- `RemoveAll` returns nil when the path does not exist, matching
  `os.RemoveAll`. It used to return an error.
- Depend on `github.com/absfs/inode` v1.1.0 and drop the local `replace`
  directive that broke builds outside the author's workspace.
