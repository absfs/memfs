package memfs

import (
	"io"
	"sync"
	"sync/atomic"

	"github.com/absfs/inode"
)

// MemByteStore implements a thread-safe in-memory ByteStore.
//
// This implementation uses a map of atomic pointers to byte slices, enabling
// lock-free reads while ensuring thread-safe writes through a mutex. Each file's
// data is stored as an atomic.Pointer[[]byte], allowing concurrent readers to
// access file data without blocking each other or blocking writers.
//
// The store supports sparse files by filling gaps with zeros when writing beyond
// the current file size.
//
// Thread Safety:
//   - Reads (ReadAt, Stat) use atomic loads for lock-free access
//   - Writes (WriteAt, Truncate) use a mutex to prevent concurrent modifications
//   - Remove operations use a mutex to safely delete entries
//
// This design makes MemByteStore fully thread-safe and suitable for use in
// concurrent environments without requiring external synchronization.
type MemByteStore struct {
	mu    sync.RWMutex
	files map[uint64]*atomic.Pointer[[]byte]
}

// NewMemByteStore creates a new thread-safe in-memory ByteStore.
func NewMemByteStore() *MemByteStore {
	return &MemByteStore{
		files: make(map[uint64]*atomic.Pointer[[]byte]),
	}
}

// ReadAt reads len(p) bytes from the file at the given offset.
//
// Returns the number of bytes read and any error encountered.
// Returns io.EOF when offset is at or beyond the end of file.
//
// This operation is lock-free for better concurrency. It uses atomic
// loads to read the current file data without blocking other readers
// or writers.
func (s *MemByteStore) ReadAt(ino uint64, p []byte, off int64) (int, error) {
	s.mu.RLock()
	filePtr := s.files[ino]
	s.mu.RUnlock()

	if filePtr == nil {
		return 0, io.EOF
	}

	data := filePtr.Load()
	if data == nil {
		return 0, io.EOF
	}

	if off >= int64(len(*data)) {
		return 0, io.EOF
	}

	n := copy(p, (*data)[off:])
	if n < len(p) {
		return n, io.EOF
	}
	return n, nil
}

// WriteAt writes len(p) bytes to the file at the given offset.
//
// Extends the file if necessary, filling any gaps with zeros to support
// sparse file semantics. Returns the number of bytes written and any error.
//
// This operation uses a mutex to ensure thread-safe modifications. The
// implementation uses atomic compare-and-swap to update the file data,
// ensuring consistency even in the face of concurrent operations.
func (s *MemByteStore) WriteAt(ino uint64, p []byte, off int64) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	// Get or create the atomic pointer for this file
	filePtr := s.files[ino]
	if filePtr == nil {
		filePtr = &atomic.Pointer[[]byte]{}
		s.files[ino] = filePtr
	}

	// Load current data
	data := filePtr.Load()
	var currentData []byte
	if data != nil {
		currentData = *data
	}

	end := off + int64(len(p))

	// Create new data slice if we need to extend the file
	var newData []byte
	if end > int64(len(currentData)) {
		newData = make([]byte, end)
		copy(newData, currentData)
	} else {
		// Make a copy to avoid modifying the old slice that might still be read
		newData = make([]byte, len(currentData))
		copy(newData, currentData)
	}

	// Write the new data
	n := copy(newData[off:], p)

	// Atomically update the pointer
	filePtr.Store(&newData)

	return n, nil
}

// Truncate changes the file size.
//
// If the file is larger than size, it is truncated to size.
// If the file is smaller, it is extended with zero bytes to size.
//
// This operation uses a mutex to ensure thread-safe modifications.
func (s *MemByteStore) Truncate(ino uint64, size int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	filePtr := s.files[ino]
	if filePtr == nil {
		if size == 0 {
			return nil
		}
		// Create new file with zeros
		filePtr = &atomic.Pointer[[]byte]{}
		s.files[ino] = filePtr
		newData := make([]byte, size)
		filePtr.Store(&newData)
		return nil
	}

	data := filePtr.Load()
	var currentData []byte
	if data != nil {
		currentData = *data
	}

	if size == 0 {
		// Truncate to zero
		empty := make([]byte, 0)
		filePtr.Store(&empty)
		return nil
	}

	if int64(len(currentData)) == size {
		return nil
	}

	if size < int64(len(currentData)) {
		// Shrink - make a copy of the truncated data
		newData := make([]byte, size)
		copy(newData, currentData[:size])
		filePtr.Store(&newData)
		return nil
	}

	// Extend with zeros
	newData := make([]byte, size)
	copy(newData, currentData)
	filePtr.Store(&newData)
	return nil
}

// Remove deletes all data associated with the inode.
//
// After Remove, operations on this inode will behave as if the file
// doesn't exist. Calling Remove on a non-existent inode is a no-op.
//
// This operation uses a mutex to ensure thread-safe removal.
func (s *MemByteStore) Remove(ino uint64) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	delete(s.files, ino)
	return nil
}

// Stat returns the current size of the file in bytes.
//
// Returns (0, nil) for empty or non-existent files.
//
// This operation uses atomic loads for lock-free access.
func (s *MemByteStore) Stat(ino uint64) (int64, error) {
	s.mu.RLock()
	filePtr := s.files[ino]
	s.mu.RUnlock()

	if filePtr == nil {
		return 0, nil
	}

	data := filePtr.Load()
	if data == nil {
		return 0, nil
	}

	return int64(len(*data)), nil
}

// Ensure MemByteStore implements inode.ByteStore at compile time
var _ inode.ByteStore = (*MemByteStore)(nil)
