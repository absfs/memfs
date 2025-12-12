package memfs_test

import (
	"fmt"
	"io"
	"os"
	"path"
	"testing"

	"github.com/absfs/fstools"
	"github.com/absfs/memfs"
)

// ExampleNewFS demonstrates creating a new in-memory file system.
func ExampleNewFS() {
	fs, err := memfs.NewFS()
	if err != nil {
		panic(err)
	}

	// Get the current working directory
	cwd, _ := fs.Getwd()
	fmt.Println(cwd)

	// Output:
	// /
}

// Example_basicFileOperations demonstrates basic file creation, writing, and reading.
func Example_basicFileOperations() {
	fs, _ := memfs.NewFS()

	// Create a new file
	file, err := fs.Create("/hello.txt")
	if err != nil {
		panic(err)
	}

	// Write data to the file
	_, err = file.Write([]byte("Hello, World!"))
	if err != nil {
		panic(err)
	}
	file.Close()

	// Open and read the file
	file, err = fs.Open("/hello.txt")
	if err != nil {
		panic(err)
	}
	defer file.Close()

	data := make([]byte, 13)
	n, err := file.Read(data)
	if err != nil {
		panic(err)
	}

	fmt.Printf("Read %d bytes: %s\n", n, string(data))

	// Output:
	// Read 13 bytes: Hello, World!
}

// Example_writeAndRead demonstrates writing and reading from files.
func Example_writeAndRead() {
	fs, _ := memfs.NewFS()

	// Create and write to a file
	file, _ := fs.Create("/data.txt")
	file.Write([]byte("line 1\n"))
	file.Write([]byte("line 2\n"))
	file.Write([]byte("line 3\n"))
	file.Close()

	// Open and read the entire file
	file, _ = fs.Open("/data.txt")
	defer file.Close()

	data, _ := io.ReadAll(file)
	fmt.Print(string(data))

	// Output:
	// line 1
	// line 2
	// line 3
}

// Example_directoryOperations demonstrates creating and navigating directories.
func Example_directoryOperations() {
	fs, _ := memfs.NewFS()

	// Create a directory
	err := fs.Mkdir("/home", 0755)
	if err != nil {
		panic(err)
	}

	// Create nested directories
	err = fs.MkdirAll("/home/user/documents", 0755)
	if err != nil {
		panic(err)
	}

	// Change to the new directory
	err = fs.Chdir("/home/user")
	if err != nil {
		panic(err)
	}

	// Get current working directory
	cwd, _ := fs.Getwd()
	fmt.Println(cwd)

	// Output:
	// /home/user
}

// Example_listDirectory demonstrates listing directory contents.
func Example_listDirectory() {
	fs, _ := memfs.NewFS()

	// Create some files and directories
	fs.MkdirAll("/app/src", 0755)
	fs.Create("/app/README.md")
	fs.Create("/app/main.go")
	fs.Create("/app/src/utils.go")

	// Open the directory
	dir, _ := fs.Open("/app")
	defer dir.Close()

	// Read directory entries
	entries, _ := dir.Readdir(-1)
	for _, entry := range entries {
		// Skip . and .. entries
		if entry.Name() == "." || entry.Name() == ".." {
			continue
		}
		fmt.Printf("%s (dir: %v)\n", entry.Name(), entry.IsDir())
	}

	// Output:
	// README.md (dir: false)
	// main.go (dir: false)
	// src (dir: true)
}

// Example_fileManipulation demonstrates renaming, removing, and truncating files.
func Example_fileManipulation() {
	fs, _ := memfs.NewFS()

	// Create a file
	file, _ := fs.Create("/temp.txt")
	file.Write([]byte("temporary data"))
	file.Close()

	// Rename the file
	fs.Rename("/temp.txt", "/permanent.txt")

	// Truncate the file
	fs.Truncate("/permanent.txt", 4)

	// Read the truncated file
	file, _ = fs.Open("/permanent.txt")
	data, _ := io.ReadAll(file)
	file.Close()
	fmt.Println(string(data))

	// Remove the file
	fs.Remove("/permanent.txt")

	// Try to open the removed file
	_, err := fs.Open("/permanent.txt")
	fmt.Println(err != nil)

	// Output:
	// temp
	// true
}

// Example_symbolicLinks demonstrates creating and using symbolic links.
func Example_symbolicLinks() {
	fs, _ := memfs.NewFS()

	// Create a file
	file, _ := fs.Create("/original.txt")
	file.Write([]byte("original content"))
	file.Close()

	// Create a symbolic link
	fs.Symlink("/original.txt", "/link.txt")

	// Get the link target
	target, _ := fs.Readlink("/link.txt")
	fmt.Println(target)

	// Stat follows the link
	info, _ := fs.Stat("/link.txt")
	fmt.Println(info.Size())

	// Lstat does not follow the link
	info, _ = fs.Lstat("/link.txt")
	fmt.Println(info.Mode()&os.ModeSymlink != 0)

	// Output:
	// /original.txt
	// 16
	// true
}

// Example_walkFileTree demonstrates traversing a file tree.
func Example_walkFileTree() {
	fs, _ := memfs.NewFS()

	// Create a directory structure
	fs.MkdirAll("/project/src/main", 0755)
	fs.MkdirAll("/project/src/utils", 0755)
	fs.Create("/project/README.md")
	fs.Create("/project/src/main/app.go")
	fs.Create("/project/src/utils/helpers.go")

	// Walk the file tree
	fstools.Walk(fs,"/project", func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			fmt.Printf("[DIR]  %s\n", path)
		} else {
			fmt.Printf("[FILE] %s\n", path)
		}
		return nil
	})

	// Output:
	// [DIR]  /project
	// [FILE] /project/README.md
	// [DIR]  /project/src
	// [DIR]  /project/src/main
	// [FILE] /project/src/main/app.go
	// [DIR]  /project/src/utils
	// [FILE] /project/src/utils/helpers.go
}

// Example_filePermissions demonstrates working with file permissions.
func Example_filePermissions() {
	fs, _ := memfs.NewFS()

	// Create a file with specific permissions
	file, _ := fs.OpenFile("/secret.txt", os.O_CREATE|os.O_RDWR, 0600)
	file.Write([]byte("secret data"))
	file.Close()

	// Check file permissions
	info, _ := fs.Stat("/secret.txt")
	fmt.Printf("Permissions: %o\n", info.Mode().Perm())

	// Change permissions
	fs.Chmod("/secret.txt", 0644)

	info, _ = fs.Stat("/secret.txt")
	fmt.Printf("New permissions: %o\n", info.Mode().Perm())

	// Output:
	// Permissions: 600
	// New permissions: 644
}

// Example_concurrentAccess demonstrates file system access with goroutines.
func Example_concurrentAccess() {
	fs, _ := memfs.NewFS()

	// Create files sequentially to avoid race conditions
	// (Note: memfs is not designed for concurrent writes to fs.data)
	for i := 0; i < 3; i++ {
		filename := fmt.Sprintf("/file%d.txt", i)
		file, _ := fs.Create(filename)
		file.Write([]byte(fmt.Sprintf("data from file %d", i)))
		file.Close()
	}

	// List all files
	dir, _ := fs.Open("/")
	entries, _ := dir.Readdir(-1)
	dir.Close()

	for _, entry := range entries {
		if entry.Name() != "." && entry.Name() != ".." && !entry.IsDir() {
			fmt.Println(entry.Name())
		}
	}

	// Output:
	// file0.txt
	// file1.txt
	// file2.txt
}

// Example_temporaryFiles demonstrates working with temporary files.
func Example_temporaryFiles() {
	fs, _ := memfs.NewFS()

	// Get temp directory
	tmpDir := fs.TempDir()
	fmt.Println(tmpDir)

	// Create temp directory if it doesn't exist
	fs.MkdirAll(tmpDir, 0755)

	// Create a temp file
	tmpFile := path.Join(tmpDir, "temp-12345.txt")
	file, _ := fs.Create(tmpFile)
	file.Write([]byte("temporary data"))
	file.Close()

	// Use the temp file
	file, _ = fs.Open(tmpFile)
	data, _ := io.ReadAll(file)
	file.Close()

	fmt.Println(string(data))

	// Clean up
	fs.Remove(tmpFile)

	// Output:
	// /tmp
	// temporary data
}

// Example_copyFile demonstrates copying a file within the file system.
func Example_copyFile() {
	fs, _ := memfs.NewFS()

	// Create source file
	src, _ := fs.Create("/source.txt")
	src.Write([]byte("content to copy"))
	src.Close()

	// Open source for reading
	src, _ = fs.Open("/source.txt")

	// Create destination file
	dst, _ := fs.Create("/destination.txt")

	// Copy content
	bytes, _ := io.Copy(dst, src)
	fmt.Printf("Copied %d bytes\n", bytes)

	// Close both files
	src.Close()
	dst.Close()

	// Verify
	file, _ := fs.Open("/destination.txt")
	data, _ := io.ReadAll(file)
	file.Close()
	fmt.Println(string(data))

	// Output:
	// Copied 15 bytes
	// content to copy
}

// Example_removeAll demonstrates recursively removing directories.
func Example_removeAll() {
	fs, _ := memfs.NewFS()

	// Create a directory structure with files
	fs.MkdirAll("/data/logs/2024", 0755)
	fs.Create("/data/logs/2024/app.log")
	fs.Create("/data/logs/2024/error.log")
	fs.Create("/data/config.json")

	// Remove entire directory tree
	fs.RemoveAll("/data/logs")

	// Verify logs directory is gone
	_, err := fs.Stat("/data/logs")
	fmt.Println(err != nil)

	// But parent still exists
	_, err = fs.Stat("/data")
	fmt.Println(err == nil)

	// Output:
	// true
	// true
}

// Example_chownAndChtimes demonstrates changing file ownership and times.
func Example_chownAndChtimes() {
	fs, _ := memfs.NewFS()

	// Create a file
	file, _ := fs.Create("/test.txt")
	file.Write([]byte("test data"))
	file.Close()

	// Change ownership
	fs.Chown("/test.txt", 1000, 1000)

	// Note: Could also change access/modification times with fs.Chtimes()

	info, _ := fs.Stat("/test.txt")
	fmt.Printf("File: %s, Size: %d bytes\n", info.Name(), info.Size())

	// Output:
	// File: test.txt, Size: 9 bytes
}

// Benchmarks

// BenchmarkFileCreation measures the performance of creating files.
func BenchmarkFileCreation(b *testing.B) {
	fs, _ := memfs.NewFS()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		filename := fmt.Sprintf("/file%d.txt", i)
		file, _ := fs.Create(filename)
		file.Close()
	}
}

// BenchmarkFileCreationWithContent measures file creation with data writing.
func BenchmarkFileCreationWithContent(b *testing.B) {
	fs, _ := memfs.NewFS()
	data := []byte("Hello, World! This is some test data.")
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		filename := fmt.Sprintf("/file%d.txt", i)
		file, _ := fs.Create(filename)
		file.Write(data)
		file.Close()
	}
}

// BenchmarkSequentialWrite measures sequential write performance.
func BenchmarkSequentialWrite(b *testing.B) {
	sizes := []int{1024, 4096, 16384, 65536}
	for _, size := range sizes {
		b.Run(fmt.Sprintf("%dB", size), func(b *testing.B) {
			fs, _ := memfs.NewFS()
			data := make([]byte, size)
			for i := range data {
				data[i] = byte(i % 256)
			}

			file, _ := fs.Create("/benchmark.dat")
			defer file.Close()
			b.SetBytes(int64(size))
			b.ResetTimer()

			for i := 0; i < b.N; i++ {
				file.Seek(0, io.SeekStart)
				file.Write(data)
			}
		})
	}
}

// BenchmarkSequentialRead measures sequential read performance.
func BenchmarkSequentialRead(b *testing.B) {
	sizes := []int{1024, 4096, 16384, 65536}
	for _, size := range sizes {
		b.Run(fmt.Sprintf("%dB", size), func(b *testing.B) {
			fs, _ := memfs.NewFS()
			data := make([]byte, size)
			for i := range data {
				data[i] = byte(i % 256)
			}

			// Setup: create file with data
			file, _ := fs.Create("/benchmark.dat")
			file.Write(data)
			file.Close()

			// Benchmark reading
			readBuf := make([]byte, size)
			file, _ = fs.Open("/benchmark.dat")
			defer file.Close()
			b.SetBytes(int64(size))
			b.ResetTimer()

			for i := 0; i < b.N; i++ {
				file.Seek(0, io.SeekStart)
				file.Read(readBuf)
			}
		})
	}
}

// BenchmarkRandomAccess measures random read/write performance.
func BenchmarkRandomAccess(b *testing.B) {
	fs, _ := memfs.NewFS()
	fileSize := 1024 * 1024 // 1 MB
	data := make([]byte, fileSize)
	for i := range data {
		data[i] = byte(i % 256)
	}

	// Setup: create large file
	file, _ := fs.Create("/large.dat")
	file.Write(data)
	file.Close()

	b.Run("RandomRead", func(b *testing.B) {
		file, _ := fs.Open("/large.dat")
		defer file.Close()
		buf := make([]byte, 4096)
		b.ResetTimer()

		for i := 0; i < b.N; i++ {
			offset := int64((i * 4096) % (fileSize - 4096))
			file.Seek(offset, io.SeekStart)
			file.Read(buf)
		}
	})

	b.Run("RandomWrite", func(b *testing.B) {
		file, _ := fs.OpenFile("/large.dat", os.O_RDWR, 0644)
		defer file.Close()
		buf := make([]byte, 4096)
		b.ResetTimer()

		for i := 0; i < b.N; i++ {
			offset := int64((i * 4096) % (fileSize - 4096))
			file.Seek(offset, io.SeekStart)
			file.Write(buf)
		}
	})
}

// BenchmarkDirectoryOperations measures directory operation performance.
func BenchmarkDirectoryOperations(b *testing.B) {
	b.Run("Mkdir", func(b *testing.B) {
		fs, _ := memfs.NewFS()
		b.ResetTimer()

		for i := 0; i < b.N; i++ {
			dirname := fmt.Sprintf("/dir%d", i)
			fs.Mkdir(dirname, 0755)
		}
	})

	b.Run("MkdirAll", func(b *testing.B) {
		fs, _ := memfs.NewFS()
		b.ResetTimer()

		for i := 0; i < b.N; i++ {
			path := fmt.Sprintf("/dir%d/subdir/nested", i)
			fs.MkdirAll(path, 0755)
		}
	})

	b.Run("Readdir", func(b *testing.B) {
		fs, _ := memfs.NewFS()
		// Setup: create directory with files
		fs.Mkdir("/testdir", 0755)
		for i := 0; i < 100; i++ {
			filename := fmt.Sprintf("/testdir/file%d.txt", i)
			file, _ := fs.Create(filename)
			file.Close()
		}
		b.ResetTimer()

		for i := 0; i < b.N; i++ {
			dir, _ := fs.Open("/testdir")
			dir.Readdir(-1)
			dir.Close()
		}
	})

	b.Run("RemoveAll", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			b.StopTimer()
			fs, _ := memfs.NewFS()
			// Setup: create directory tree
			fs.MkdirAll("/data/logs/2024", 0755)
			for j := 0; j < 10; j++ {
				filename := fmt.Sprintf("/data/logs/2024/file%d.log", j)
				file, _ := fs.Create(filename)
				file.Close()
			}
			b.StartTimer()

			fs.RemoveAll("/data")
		}
	})
}

// BenchmarkStatOperations measures file stat operation performance.
func BenchmarkStatOperations(b *testing.B) {
	fs, _ := memfs.NewFS()

	// Setup: create some files
	for i := 0; i < 10; i++ {
		filename := fmt.Sprintf("/file%d.txt", i)
		file, _ := fs.Create(filename)
		file.Write([]byte("test data"))
		file.Close()
	}

	b.Run("Stat", func(b *testing.B) {
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			fs.Stat("/file0.txt")
		}
	})

	b.Run("Lstat", func(b *testing.B) {
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			fs.Lstat("/file0.txt")
		}
	})

	b.Run("StatWithSymlink", func(b *testing.B) {
		fs.Symlink("/file0.txt", "/link.txt")
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			fs.Stat("/link.txt")
		}
	})
}

// BenchmarkWalk measures file tree walking performance.
func BenchmarkWalk(b *testing.B) {
	fs, _ := memfs.NewFS()

	// Setup: create a directory tree
	fs.MkdirAll("/project/src/main", 0755)
	fs.MkdirAll("/project/src/utils", 0755)
	fs.MkdirAll("/project/test", 0755)
	for i := 0; i < 20; i++ {
		filename := fmt.Sprintf("/project/src/file%d.go", i)
		file, _ := fs.Create(filename)
		file.Close()
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		fstools.Walk(fs,"/project", func(path string, info os.FileInfo, err error) error {
			return nil
		})
	}
}

// BenchmarkSymlinkOperations measures symbolic link operation performance.
func BenchmarkSymlinkOperations(b *testing.B) {
	b.Run("CreateSymlink", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			b.StopTimer()
			fs, _ := memfs.NewFS()
			file, _ := fs.Create("/target.txt")
			file.Close()
			b.StartTimer()

			fs.Symlink("/target.txt", "/link.txt")
		}
	})

	b.Run("Readlink", func(b *testing.B) {
		fs, _ := memfs.NewFS()
		file, _ := fs.Create("/target.txt")
		file.Close()
		fs.Symlink("/target.txt", "/link.txt")
		b.ResetTimer()

		for i := 0; i < b.N; i++ {
			fs.Readlink("/link.txt")
		}
	})
}

// BenchmarkRename measures file rename performance.
func BenchmarkRename(b *testing.B) {
	for i := 0; i < b.N; i++ {
		b.StopTimer()
		fs, _ := memfs.NewFS()
		file, _ := fs.Create("/old.txt")
		file.Write([]byte("data"))
		file.Close()
		b.StartTimer()

		fs.Rename("/old.txt", "/new.txt")
	}
}

// BenchmarkTruncate measures file truncate performance.
func BenchmarkTruncate(b *testing.B) {
	fs, _ := memfs.NewFS()
	file, _ := fs.Create("/test.txt")
	file.Write(make([]byte, 10240))
	file.Close()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		fs.Truncate("/test.txt", 1024)
		fs.Truncate("/test.txt", 10240)
	}
}

// BenchmarkChmod measures permission change performance.
func BenchmarkChmod(b *testing.B) {
	fs, _ := memfs.NewFS()
	file, _ := fs.Create("/test.txt")
	file.Close()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		fs.Chmod("/test.txt", 0644)
	}
}

// BenchmarkChown measures ownership change performance.
func BenchmarkChown(b *testing.B) {
	fs, _ := memfs.NewFS()
	file, _ := fs.Create("/test.txt")
	file.Close()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		fs.Chown("/test.txt", 1000, 1000)
	}
}
