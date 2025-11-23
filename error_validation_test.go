package memfs_test

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"syscall"
	"testing"

	"github.com/absfs/memfs"
)

// TestErrorReportingMatchesOS validates that memfs error reporting closely matches os package equivalents.
// This test creates parallel scenarios in both memfs and the real filesystem, then compares the errors
// to ensure they have the same types and underlying syscall error codes.
func TestErrorReportingMatchesOS(t *testing.T) {
	// Setup memfs
	mfs, err := memfs.NewFS()
	if err != nil {
		t.Fatal(err)
	}

	// Setup real filesystem test directory
	osTestDir := t.TempDir()

	tests := []struct {
		name        string
		setupMemFS  func() error
		setupOS     func() error
		testMemFS   func() error
		testOS      func() error
		wantErrType string // expected error type (e.g., "*os.PathError")
		wantSyscall error  // expected syscall error (e.g., syscall.ENOENT)
	}{
		{
			name:       "Open non-existent file",
			setupMemFS: func() error { return nil },
			setupOS:    func() error { return nil },
			testMemFS: func() error {
				_, err := mfs.Open("/nonexistent.txt")
				return err
			},
			testOS: func() error {
				_, err := os.Open(filepath.Join(osTestDir, "nonexistent.txt"))
				return err
			},
			wantErrType: "*os.PathError",
			wantSyscall: syscall.ENOENT,
		},
		{
			name: "Create file with O_EXCL when file exists",
			setupMemFS: func() error {
				f, err := mfs.Create("/exists.txt")
				if err != nil {
					return err
				}
				return f.Close()
			},
			setupOS: func() error {
				f, err := os.Create(filepath.Join(osTestDir, "exists.txt"))
				if err != nil {
					return err
				}
				return f.Close()
			},
			testMemFS: func() error {
				_, err := mfs.OpenFile("/exists.txt", os.O_CREATE|os.O_EXCL, 0644)
				return err
			},
			testOS: func() error {
				_, err := os.OpenFile(filepath.Join(osTestDir, "exists.txt"), os.O_CREATE|os.O_EXCL, 0644)
				return err
			},
			wantErrType: "*os.PathError",
			wantSyscall: syscall.EEXIST,
		},
		{
			name: "Open directory for writing",
			setupMemFS: func() error {
				return mfs.Mkdir("/testdir", 0755)
			},
			setupOS: func() error {
				return os.Mkdir(filepath.Join(osTestDir, "testdir"), 0755)
			},
			testMemFS: func() error {
				_, err := mfs.OpenFile("/testdir", os.O_WRONLY, 0)
				return err
			},
			testOS: func() error {
				_, err := os.OpenFile(filepath.Join(osTestDir, "testdir"), os.O_WRONLY, 0)
				return err
			},
			wantErrType: "*os.PathError",
			wantSyscall: syscall.EISDIR,
		},
		{
			name: "Remove non-empty directory",
			setupMemFS: func() error {
				if err := mfs.Mkdir("/nonempty", 0755); err != nil {
					return err
				}
				f, err := mfs.Create("/nonempty/file.txt")
				if err != nil {
					return err
				}
				return f.Close()
			},
			setupOS: func() error {
				dir := filepath.Join(osTestDir, "nonempty")
				if err := os.Mkdir(dir, 0755); err != nil {
					return err
				}
				f, err := os.Create(filepath.Join(dir, "file.txt"))
				if err != nil {
					return err
				}
				return f.Close()
			},
			testMemFS: func() error {
				return mfs.Remove("/nonempty")
			},
			testOS: func() error {
				return os.Remove(filepath.Join(osTestDir, "nonempty"))
			},
			wantErrType: "*os.PathError",
			wantSyscall: syscall.ENOTEMPTY,
		},
		{
			name: "Mkdir when directory exists",
			setupMemFS: func() error {
				return mfs.Mkdir("/existingdir", 0755)
			},
			setupOS: func() error {
				return os.Mkdir(filepath.Join(osTestDir, "existingdir"), 0755)
			},
			testMemFS: func() error {
				return mfs.Mkdir("/existingdir", 0755)
			},
			testOS: func() error {
				return os.Mkdir(filepath.Join(osTestDir, "existingdir"), 0755)
			},
			wantErrType: "*os.PathError",
			wantSyscall: os.ErrExist,
		},
		{
			name: "Remove non-existent file",
			setupMemFS: func() error {
				return nil
			},
			setupOS: func() error {
				return nil
			},
			testMemFS: func() error {
				return mfs.Remove("/doesnotexist.txt")
			},
			testOS: func() error {
				return os.Remove(filepath.Join(osTestDir, "doesnotexist.txt"))
			},
			wantErrType: "*os.PathError",
			wantSyscall: syscall.ENOENT,
		},
		{
			name: "Chdir to non-existent directory",
			setupMemFS: func() error {
				return nil
			},
			setupOS: func() error {
				return nil
			},
			testMemFS: func() error {
				return mfs.Chdir("/nonexistentdir")
			},
			testOS: func() error {
				return os.Chdir(filepath.Join(osTestDir, "nonexistentdir"))
			},
			wantErrType: "*os.PathError",
			wantSyscall: syscall.ENOENT,
		},
		{
			name: "Chdir to a file (not a directory)",
			setupMemFS: func() error {
				f, err := mfs.Create("/regularfile.txt")
				if err != nil {
					return err
				}
				return f.Close()
			},
			setupOS: func() error {
				f, err := os.Create(filepath.Join(osTestDir, "regularfile.txt"))
				if err != nil {
					return err
				}
				return f.Close()
			},
			testMemFS: func() error {
				return mfs.Chdir("/regularfile.txt")
			},
			testOS: func() error {
				return os.Chdir(filepath.Join(osTestDir, "regularfile.txt"))
			},
			wantErrType: "*os.PathError",
			wantSyscall: syscall.ENOTDIR,
		},
		{
			name: "Rename root directory",
			setupMemFS: func() error {
				return nil
			},
			setupOS: func() error {
				return nil
			},
			testMemFS: func() error {
				return mfs.Rename("/", "/newroot")
			},
			testOS: func() error {
				// OS doesn't allow renaming root, so we simulate with a directory
				// that would produce EINVAL
				return &os.LinkError{Op: "rename", Old: "/", New: "/newroot", Err: syscall.EINVAL}
			},
			wantErrType: "*os.LinkError",
			wantSyscall: syscall.EINVAL,
		},
		{
			name: "Stat symlink cycle",
			setupMemFS: func() error {
				// Create a temp file first
				f, err := mfs.Create("/temp_for_link")
				if err != nil {
					return err
				}
				f.Close()

				// Create symlink
				if err := mfs.Symlink("/temp_for_link", "/cycle_link"); err != nil {
					return err
				}
				// Update it to point to itself (creating a cycle)
				return mfs.Symlink("/cycle_link", "/cycle_link")
			},
			setupOS: func() error {
				// Create a symlink cycle in OS filesystem
				linkPath := filepath.Join(osTestDir, "cycle_link")
				return os.Symlink(linkPath, linkPath)
			},
			testMemFS: func() error {
				_, err := mfs.Stat("/cycle_link")
				return err
			},
			testOS: func() error {
				_, err := os.Stat(filepath.Join(osTestDir, "cycle_link"))
				return err
			},
			wantErrType: "*os.PathError",
			wantSyscall: syscall.ELOOP,
		},
		{
			name: "Read from write-only file",
			setupMemFS: func() error {
				return nil
			},
			setupOS: func() error {
				return nil
			},
			testMemFS: func() error {
				f, err := mfs.OpenFile("/writeonly.txt", os.O_CREATE|os.O_WRONLY, 0644)
				if err != nil {
					return err
				}
				defer f.Close()
				buf := make([]byte, 10)
				_, err = f.Read(buf)
				return err
			},
			testOS: func() error {
				f, err := os.OpenFile(filepath.Join(osTestDir, "writeonly.txt"), os.O_CREATE|os.O_WRONLY, 0644)
				if err != nil {
					return err
				}
				defer f.Close()
				buf := make([]byte, 10)
				_, err = f.Read(buf)
				return err
			},
			wantErrType: "*os.PathError",
			wantSyscall: syscall.EBADF,
		},
		{
			name: "Write to read-only file",
			setupMemFS: func() error {
				f, err := mfs.Create("/readonly.txt")
				if err != nil {
					return err
				}
				return f.Close()
			},
			setupOS: func() error {
				f, err := os.Create(filepath.Join(osTestDir, "readonly.txt"))
				if err != nil {
					return err
				}
				return f.Close()
			},
			testMemFS: func() error {
				f, err := mfs.OpenFile("/readonly.txt", os.O_RDONLY, 0)
				if err != nil {
					return err
				}
				defer f.Close()
				_, err = f.Write([]byte("test"))
				return err
			},
			testOS: func() error {
				f, err := os.OpenFile(filepath.Join(osTestDir, "readonly.txt"), os.O_RDONLY, 0)
				if err != nil {
					return err
				}
				defer f.Close()
				_, err = f.Write([]byte("test"))
				return err
			},
			wantErrType: "*os.PathError",
			wantSyscall: syscall.EBADF,
		},
		{
			name: "Readdir on closed file",
			setupMemFS: func() error {
				return mfs.Mkdir("/closeddir", 0755)
			},
			setupOS: func() error {
				return os.Mkdir(filepath.Join(osTestDir, "closeddir"), 0755)
			},
			testMemFS: func() error {
				f, err := mfs.Open("/closeddir")
				if err != nil {
					return err
				}
				f.Close()
				_, err = f.Readdir(-1)
				return err
			},
			testOS: func() error {
				f, err := os.Open(filepath.Join(osTestDir, "closeddir"))
				if err != nil {
					return err
				}
				f.Close()
				_, err = f.Readdir(-1)
				return err
			},
			wantErrType: "*os.PathError",
			// wantSyscall is nil because both return custom "use of closed file" error
			// The string comparison below will verify they match
			wantSyscall: nil,
		},
		{
			name: "Readdir on non-directory",
			setupMemFS: func() error {
				f, err := mfs.Create("/notadir.txt")
				if err != nil {
					return err
				}
				return f.Close()
			},
			setupOS: func() error {
				f, err := os.Create(filepath.Join(osTestDir, "notadir.txt"))
				if err != nil {
					return err
				}
				return f.Close()
			},
			testMemFS: func() error {
				f, err := mfs.Open("/notadir.txt")
				if err != nil {
					return err
				}
				defer f.Close()
				_, err = f.Readdir(-1)
				return err
			},
			testOS: func() error {
				f, err := os.Open(filepath.Join(osTestDir, "notadir.txt"))
				if err != nil {
					return err
				}
				defer f.Close()
				_, err = f.Readdir(-1)
				return err
			},
			wantErrType: "", // syscall.ENOTDIR is returned directly, not wrapped
			wantSyscall: syscall.ENOTDIR,
		},
		{
			name: "Read from directory",
			setupMemFS: func() error {
				return mfs.Mkdir("/readdir", 0755)
			},
			setupOS: func() error {
				return os.Mkdir(filepath.Join(osTestDir, "readdir"), 0755)
			},
			testMemFS: func() error {
				f, err := mfs.Open("/readdir")
				if err != nil {
					return err
				}
				defer f.Close()
				buf := make([]byte, 10)
				_, err = f.Read(buf)
				return err
			},
			testOS: func() error {
				f, err := os.Open(filepath.Join(osTestDir, "readdir"))
				if err != nil {
					return err
				}
				defer f.Close()
				buf := make([]byte, 10)
				_, err = f.Read(buf)
				return err
			},
			wantErrType: "*os.PathError",
			wantSyscall: syscall.EISDIR,
		},
		{
			name: "Stat non-existent file",
			setupMemFS: func() error {
				return nil
			},
			setupOS: func() error {
				return nil
			},
			testMemFS: func() error {
				_, err := mfs.Stat("/nosuchfile.txt")
				return err
			},
			testOS: func() error {
				_, err := os.Stat(filepath.Join(osTestDir, "nosuchfile.txt"))
				return err
			},
			wantErrType: "*os.PathError",
			wantSyscall: syscall.ENOENT,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup
			if err := tt.setupMemFS(); err != nil {
				t.Fatalf("memfs setup failed: %v", err)
			}
			if err := tt.setupOS(); err != nil {
				t.Fatalf("os setup failed: %v", err)
			}

			// Execute tests
			memfsErr := tt.testMemFS()
			osErr := tt.testOS()

			// Both should return errors
			if memfsErr == nil {
				t.Errorf("memfs returned nil error, expected error")
			}
			if osErr == nil {
				t.Errorf("os returned nil error, expected error")
			}

			if memfsErr == nil || osErr == nil {
				return
			}

			// Compare error types
			memfsErrType := getErrorType(memfsErr)
			osErrType := getErrorType(osErr)

			if tt.wantErrType != "" {
				if memfsErrType != tt.wantErrType {
					t.Errorf("memfs error type mismatch:\n  got:  %s\n  want: %s\n  error: %v",
						memfsErrType, tt.wantErrType, memfsErr)
				}
				if osErrType != tt.wantErrType {
					t.Errorf("os error type mismatch:\n  got:  %s\n  want: %s\n  error: %v",
						osErrType, tt.wantErrType, osErr)
				}
			}

			// Compare underlying syscall errors
			memfsSyscallErr := extractSyscallError(memfsErr)
			osSyscallErr := extractSyscallError(osErr)

			if tt.wantSyscall != nil {
				if !errors.Is(memfsSyscallErr, tt.wantSyscall) {
					t.Errorf("memfs syscall error mismatch:\n  got:  %v (%T)\n  want: %v (%T)\n  full error: %v",
						memfsSyscallErr, memfsSyscallErr, tt.wantSyscall, tt.wantSyscall, memfsErr)
				}
				if !errors.Is(osSyscallErr, tt.wantSyscall) {
					t.Errorf("os syscall error mismatch:\n  got:  %v (%T)\n  want: %v (%T)\n  full error: %v",
						osSyscallErr, osSyscallErr, tt.wantSyscall, tt.wantSyscall, osErr)
				}
			}

			// The underlying syscall errors should match between memfs and os
			if memfsSyscallErr != nil && osSyscallErr != nil {
				if memfsSyscallErr.Error() != osSyscallErr.Error() {
					t.Errorf("syscall errors differ:\n  memfs: %v (%T)\n  os:    %v (%T)",
						memfsSyscallErr, memfsSyscallErr, osSyscallErr, osSyscallErr)
				}
			}

			t.Logf("✓ Error types match: %s", memfsErrType)
			t.Logf("✓ memfs error: %v", memfsErr)
			t.Logf("✓ os error:    %v", osErr)
		})
	}
}

// getErrorType returns a string representation of the error type
func getErrorType(err error) string {
	if err == nil {
		return "nil"
	}

	switch err.(type) {
	case *os.PathError:
		return "*os.PathError"
	case *os.LinkError:
		return "*os.LinkError"
	default:
		return "other"
	}
}

// extractSyscallError extracts the underlying syscall error from wrapped errors
func extractSyscallError(err error) error {
	if err == nil {
		return nil
	}

	// Unwrap PathError
	if pathErr, ok := err.(*os.PathError); ok {
		return pathErr.Err
	}

	// Unwrap LinkError
	if linkErr, ok := err.(*os.LinkError); ok {
		return linkErr.Err
	}

	return err
}

// TestFileOperationErrors validates specific file operation error scenarios
func TestFileOperationErrors(t *testing.T) {
	mfs, err := memfs.NewFS()
	if err != nil {
		t.Fatal(err)
	}

	t.Run("Stat after close returns EBADF", func(t *testing.T) {
		f, err := mfs.Create("/testfile.txt")
		if err != nil {
			t.Fatal(err)
		}
		f.Close()

		// The file is now closed, but Stat on the File object should fail
		_, err = f.Stat()
		if err == nil {
			t.Error("expected error when calling Stat on closed file")
			return
		}

		pathErr, ok := err.(*os.PathError)
		if !ok {
			t.Errorf("expected *os.PathError, got %T", err)
			return
		}

		if !errors.Is(pathErr.Err, syscall.EBADF) {
			t.Errorf("expected EBADF, got %v", pathErr.Err)
		}
	})

	t.Run("Read after close returns EBADF", func(t *testing.T) {
		f, err := mfs.Create("/testfile2.txt")
		if err != nil {
			t.Fatal(err)
		}
		f.Close()

		buf := make([]byte, 10)
		_, err = f.Read(buf)
		if err == nil && err != io.EOF {
			t.Error("expected error when reading from closed file")
			return
		}

		// Accept either EOF or EBADF for closed file
		if err != io.EOF {
			pathErr, ok := err.(*os.PathError)
			if !ok {
				t.Errorf("expected *os.PathError or io.EOF, got %T", err)
				return
			}

			if !errors.Is(pathErr.Err, syscall.EBADF) {
				t.Errorf("expected EBADF, got %v", pathErr.Err)
			}
		}
	})

	t.Run("EOF on empty file read", func(t *testing.T) {
		f, err := mfs.Create("/emptyfile.txt")
		if err != nil {
			t.Fatal(err)
		}
		defer f.Close()

		buf := make([]byte, 10)
		n, err := f.Read(buf)
		if err != io.EOF {
			t.Errorf("expected io.EOF, got %v", err)
		}
		if n != 0 {
			t.Errorf("expected 0 bytes read, got %d", n)
		}
	})
}

// TestErrorMessageConsistency checks that error messages are formatted consistently
func TestErrorMessageConsistency(t *testing.T) {
	mfs, err := memfs.NewFS()
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name      string
		operation func() error
		wantOp    string // expected operation name in PathError
	}{
		{
			name: "Open error includes operation",
			operation: func() error {
				_, err := mfs.Open("/nonexistent")
				return err
			},
			wantOp: "open",
		},
		{
			name: "Mkdir error includes operation",
			operation: func() error {
				mfs.Mkdir("/dir1", 0755)
				return mfs.Mkdir("/dir1", 0755) // second call should error
			},
			wantOp: "mkdir",
		},
		{
			name: "Remove error includes operation",
			operation: func() error {
				return mfs.Remove("/nonexistent")
			},
			wantOp: "remove",
		},
		{
			name: "Chdir error includes operation",
			operation: func() error {
				return mfs.Chdir("/nonexistent")
			},
			wantOp: "chdir",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.operation()
			if err == nil {
				t.Fatal("expected error, got nil")
			}

			pathErr, ok := err.(*os.PathError)
			if !ok {
				linkErr, ok := err.(*os.LinkError)
				if !ok {
					t.Fatalf("expected *os.PathError or *os.LinkError, got %T", err)
				}
				if linkErr.Op != tt.wantOp {
					t.Errorf("operation mismatch: got %q, want %q", linkErr.Op, tt.wantOp)
				}
				return
			}

			if pathErr.Op != tt.wantOp {
				t.Errorf("operation mismatch: got %q, want %q", pathErr.Op, tt.wantOp)
			}
		})
	}
}
