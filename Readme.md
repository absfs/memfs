# memfs - In Memory File System
The `memfs` package implements the `absfs.FileSystem` interface as a RAM-backed filesystem.

Care has been taken to ensure that `memfs` returns identical errors both in text
and type as the os package. This makes `memfs` particularly suited for use in
testing.

## Features

- **POSIX-like File Operations**: Create, Open, Read, Write, Seek, Truncate, Close
- **Directory Operations**: Mkdir, MkdirAll, Remove, RemoveAll, Chdir, Getwd, Readdir
- **Symbolic Links**: Full symlink support with automatic cycle detection
- **File Permissions**: Chmod, Chown, Lchown with standard Unix-style permissions
- **File Metadata**: Stat, Lstat, modification times, ownership, and size tracking
- **Directory Traversal**: Walk and FastWalk for efficient tree traversal
- **Path Operations**: Rename, relative and absolute path support
- **Error Compatibility**: Returns errors identical to the os package for easy testing
- **absfs Interface**: Full implementation of absfs.FileSystem and absfs.SymlinkFileSystem

## Install

```bash
$ go get github.com/absfs/memfs
```

## Example Usage

```go
package main

import(
    "fmt"
    "os"

    "github.com/absfs/memfs"
)

func main() {
    fs, _ := memfs.NewFS() // remember kids don't ignore errors

    // Opens a file with read/write permissions in the current directory
    f, _ := fs.Create("/example.txt")

    f.Write([]byte("Hello, world!"))
    f.Close()

    fs.Remove("example.txt")
}
```

## Limitations

### Thread Safety
**WARNING**: `memfs` is **NOT thread-safe**. The implementation does not use mutexes or any synchronization primitives. Concurrent access to the same FileSystem instance from multiple goroutines will result in race conditions and undefined behavior.

If you need concurrent access:
- Create separate FileSystem instances for each goroutine
- Implement your own synchronization using mutexes around fs operations
- Use channels to serialize access to a shared FileSystem instance

### Persistence
`memfs` provides **no persistence**. All data is stored in memory and will be lost when:
- The FileSystem instance is garbage collected
- Your application terminates
- The system restarts

This is by design - `memfs` is intended for temporary storage, testing, and scenarios where persistence is not required.

### Memory Constraints
- **Memory grows linearly** with the number and size of files created
- **No automatic cleanup**: Deleted file data is set to nil but the underlying slice capacity remains
- **Large files**: Creating very large files (> 1GB) can cause memory pressure
- **No memory limits**: memfs does not enforce any quota or size limits

For applications with many files or large datasets, monitor memory usage carefully.

### Performance Considerations
- **Fast for small to medium datasets**: Operations are O(1) to O(n) depending on the operation
- **Path resolution**: Nested directory lookups are O(depth × entries)
- **Directory listings**: O(n) where n is the number of entries in the directory
- **No disk I/O overhead**: All operations are in-memory and very fast for typical use cases

### Special Files Not Supported
The following special file types are **not supported**:
- Device files (block or character devices)
- Named pipes (FIFOs)
- Unix domain sockets
- Hard links (only symbolic links are supported)

Attempting to create these will result in either an error or undefined behavior.

## Performance Characteristics

### Benchmarks
Based on the included benchmarks, typical performance characteristics are:

- **File Creation**: ~1-2 µs per file
- **Sequential Read/Write**:
  - 1KB: ~500 MB/s
  - 64KB: ~2 GB/s (memory bandwidth limited)
- **Random Access**: ~200-500 ns per operation
- **Directory Operations**:
  - Mkdir: ~500 ns
  - MkdirAll: ~2 µs for 3-level nesting
  - Readdir (100 files): ~5 µs
- **Metadata Operations**:
  - Stat: ~200 ns
  - Chmod/Chown: ~100 ns

These numbers are approximate and will vary based on hardware and system load.

### Best Practices
- Use `MkdirAll` for creating nested directory structures
- Batch operations when possible to reduce overhead
- For large directory listings, use `Readdirnames` instead of `Readdir` if you only need names
- Close files promptly to trigger Sync and update metadata
- Use `RemoveAll` for recursive deletion rather than manual traversal

## Thread Safety Warnings

**CRITICAL**: Do not access the same `memfs.FileSystem` instance from multiple goroutines without external synchronization.

### Race Condition Examples

```go
// UNSAFE - Data race on fs.data
fs, _ := memfs.NewFS()
go func() {
    f, _ := fs.Create("/file1.txt")
    f.Write([]byte("data"))
    f.Close()
}()
go func() {
    f, _ := fs.Create("/file2.txt")
    f.Write([]byte("data"))
    f.Close()
}()
```

### Safe Concurrent Usage

```go
// Option 1: Use separate filesystem instances
fs1, _ := memfs.NewFS()
fs2, _ := memfs.NewFS()
go func() { fs1.Create("/file1.txt") }()
go func() { fs2.Create("/file2.txt") }()

// Option 2: External synchronization
var mu sync.Mutex
fs, _ := memfs.NewFS()
go func() {
    mu.Lock()
    defer mu.Unlock()
    fs.Create("/file1.txt")
}()
```

### What's Safe
- Reading from different open file handles to different files
- Multiple goroutines reading (not writing) metadata with `Stat`/`Lstat`

### What's Unsafe
- Concurrent writes to `fs.data` (file creation, writing, deletion)
- Concurrent directory modifications (Mkdir, Remove)
- Concurrent access to the same file handle
- Any operation that modifies the filesystem structure

## Comparison with Alternatives

### vs os package
- **memfs**: No disk I/O, data lost on exit, much faster, ideal for testing
- **os**: Persistent storage, slower due to disk I/O, production-ready

### vs afero
- **memfs**: Focused implementation of absfs interface, simpler, smaller API surface
- **afero**: Larger ecosystem, more features, different abstraction layer

### vs testing.TB.TempDir()
- **memfs**: Pure in-memory, no filesystem pollution, faster cleanup
- **TempDir**: Uses real filesystem, automatically cleaned up by testing framework

### vs map[string][]byte
- **memfs**: Full filesystem semantics (directories, permissions, metadata)
- **map**: Simple key-value storage, no directory structure, no permissions

### Use memfs when you need:
- Fast, isolated filesystem for unit tests
- Temporary in-memory storage with filesystem semantics
- Filesystem abstraction that works identically across platforms
- No disk I/O overhead
- Easy cleanup (just discard the FileSystem instance)

## Troubleshooting

### Common Issues

#### "no such file or directory" errors
- Ensure parent directories exist before creating files
- Use `MkdirAll` to create nested directory structures
- Check that paths are absolute (start with `/`) or relative to the current working directory

#### "directory not empty" when removing
- Use `RemoveAll` instead of `Remove` for directories with contents
- Ensure you're not trying to remove the root directory `/`

#### Memory leaks or high memory usage
- Check for files that are created but never deleted
- Large files consume memory proportional to their size
- Deleted files still occupy slice capacity until garbage collection
- Consider creating a new FileSystem instance periodically to reset memory

#### Permission errors
- Check file permissions with `Stat()` and verify the mode
- Ensure files are opened with appropriate flags (`O_RDONLY`, `O_WRONLY`, `O_RDWR`)
- Remember that `fs.Umask` affects newly created file permissions

#### Symlink cycles
- `Stat()` will return an error with "too many levels of symbolic links"
- Use `Lstat()` to inspect symlinks without following them
- Use `Readlink()` to check where a symlink points

#### Race conditions and crashes
- See [Thread Safety Warnings](#thread-safety-warnings) above
- Run tests with `-race` flag: `go test -race`
- Ensure only one goroutine accesses the FileSystem at a time

#### Path separator issues
- memfs always uses `/` as the path separator, regardless of OS
- Use `filepath.Join()` or construct paths with `/` directly
- Avoid using `\` (backslash) in paths

### Debugging Tips

```go
// Check current working directory
cwd, _ := fs.Getwd()
fmt.Println("Current directory:", cwd)

// List directory contents
dir, _ := fs.Open("/")
entries, _ := dir.Readdir(-1)
for _, entry := range entries {
    fmt.Printf("%s (dir: %v)\n", entry.Name(), entry.IsDir())
}

// Inspect file metadata
info, _ := fs.Stat("/myfile.txt")
fmt.Printf("Size: %d, Mode: %v, ModTime: %v\n",
    info.Size(), info.Mode(), info.ModTime())

// Check if error is a specific type
if pathErr, ok := err.(*os.PathError); ok {
    fmt.Printf("Operation: %s, Path: %s, Err: %v\n",
        pathErr.Op, pathErr.Path, pathErr.Err)
}
```

## absfs
Check out the [`absfs`](https://github.com/absfs/absfs) repo for more information about the abstract filesystem interface and features like filesystem composition.

## LICENSE

This project is governed by the MIT License. See [LICENSE](https://github.com/absfs/osfs/blob/master/LICENSE)



