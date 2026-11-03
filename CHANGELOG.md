# Changelog

## Unreleased

### Fixed

- `MkdirAll` no longer succeeds when a path component is a regular file
  (`MkdirAll("/inside.txt/x")`); it returns `syscall.ENOTDIR` naming the
  offending path, as `os.MkdirAll` does. `Mkdir` had the same bug.
- Every path-taking method now fails for a path that runs through a
  non-directory, with the error os gives on the running platform:
  `syscall.ENOTDIR` on Unix, `ERROR_PATH_NOT_FOUND` (which `errors.Is` treats
  as `fs.ErrNotExist`) on Windows. Previously such paths returned ENOENT, an
  untyped "not a directory" error (Create, Symlink), or nil (Mkdir,
  MkdirAll).
- Symbolic links are now followed in intermediate path components, and `..`
  is resolved through the directory tree rather than lexically.
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

- `RemoveAll` returns nil when the path does not exist, matching
  `os.RemoveAll`. It used to return an error.
- Depend on `github.com/absfs/inode` v1.1.0 and drop the local `replace`
  directive that broke builds outside the author's workspace.
