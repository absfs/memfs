package memfs

import (
	"bytes"
	"io"
	"sync"
	"testing"
)

func TestMemByteStore_BasicReadWrite(t *testing.T) {
	store := NewMemByteStore()

	// Write some data
	data := []byte("hello world")
	n, err := store.WriteAt(1, data, 0)
	if err != nil {
		t.Fatalf("WriteAt failed: %v", err)
	}
	if n != len(data) {
		t.Errorf("WriteAt returned %d, want %d", n, len(data))
	}

	// Read it back
	buf := make([]byte, len(data))
	n, err = store.ReadAt(1, buf, 0)
	if err != nil {
		t.Fatalf("ReadAt failed: %v", err)
	}
	if n != len(data) {
		t.Errorf("ReadAt returned %d, want %d", n, len(data))
	}
	if !bytes.Equal(buf, data) {
		t.Errorf("ReadAt returned %q, want %q", buf, data)
	}
}

func TestMemByteStore_ReadNonExistent(t *testing.T) {
	store := NewMemByteStore()

	buf := make([]byte, 10)
	n, err := store.ReadAt(999, buf, 0)
	if err != io.EOF {
		t.Errorf("ReadAt on non-existent file returned error %v, want io.EOF", err)
	}
	if n != 0 {
		t.Errorf("ReadAt returned %d bytes, want 0", n)
	}
}

func TestMemByteStore_ReadAtOffset(t *testing.T) {
	store := NewMemByteStore()

	// Write some data
	data := []byte("hello world")
	store.WriteAt(1, data, 0)

	// Read from offset
	buf := make([]byte, 5)
	n, err := store.ReadAt(1, buf, 6)
	if err != nil {
		t.Fatalf("ReadAt failed: %v", err)
	}
	if n != 5 {
		t.Errorf("ReadAt returned %d, want 5", n)
	}
	if !bytes.Equal(buf, []byte("world")) {
		t.Errorf("ReadAt returned %q, want %q", buf, "world")
	}
}

func TestMemByteStore_ReadBeyondEOF(t *testing.T) {
	store := NewMemByteStore()

	// Write some data
	data := []byte("hello")
	store.WriteAt(1, data, 0)

	// Read beyond EOF
	buf := make([]byte, 10)
	n, err := store.ReadAt(1, buf, 0)
	if err != io.EOF {
		t.Errorf("ReadAt beyond EOF returned error %v, want io.EOF", err)
	}
	if n != 5 {
		t.Errorf("ReadAt returned %d bytes, want 5", n)
	}
	if !bytes.Equal(buf[:n], data) {
		t.Errorf("ReadAt returned %q, want %q", buf[:n], data)
	}
}

func TestMemByteStore_ReadAtEOF(t *testing.T) {
	store := NewMemByteStore()

	// Write some data
	data := []byte("hello")
	store.WriteAt(1, data, 0)

	// Read at EOF
	buf := make([]byte, 10)
	n, err := store.ReadAt(1, buf, 5)
	if err != io.EOF {
		t.Errorf("ReadAt at EOF returned error %v, want io.EOF", err)
	}
	if n != 0 {
		t.Errorf("ReadAt returned %d bytes, want 0", n)
	}
}

func TestMemByteStore_SparseFile(t *testing.T) {
	store := NewMemByteStore()

	// Write beyond current size (sparse file)
	data := []byte("world")
	n, err := store.WriteAt(1, data, 10)
	if err != nil {
		t.Fatalf("WriteAt failed: %v", err)
	}
	if n != len(data) {
		t.Errorf("WriteAt returned %d, want %d", n, len(data))
	}

	// Check size
	size, err := store.Stat(1)
	if err != nil {
		t.Fatalf("Stat failed: %v", err)
	}
	if size != 15 {
		t.Errorf("Stat returned size %d, want 15", size)
	}

	// Read the zeros
	buf := make([]byte, 10)
	n, err = store.ReadAt(1, buf, 0)
	if err != nil {
		t.Fatalf("ReadAt failed: %v", err)
	}
	if n != 10 {
		t.Errorf("ReadAt returned %d, want 10", n)
	}
	expected := make([]byte, 10)
	if !bytes.Equal(buf, expected) {
		t.Errorf("ReadAt returned %v, want %v (zeros)", buf, expected)
	}

	// Read the written data
	buf = make([]byte, 5)
	n, err = store.ReadAt(1, buf, 10)
	if err != nil {
		t.Fatalf("ReadAt failed: %v", err)
	}
	if n != 5 {
		t.Errorf("ReadAt returned %d, want 5", n)
	}
	if !bytes.Equal(buf, data) {
		t.Errorf("ReadAt returned %q, want %q", buf, data)
	}
}

func TestMemByteStore_TruncateGrow(t *testing.T) {
	store := NewMemByteStore()

	// Write some data
	data := []byte("hello")
	store.WriteAt(1, data, 0)

	// Truncate to larger size
	err := store.Truncate(1, 10)
	if err != nil {
		t.Fatalf("Truncate failed: %v", err)
	}

	// Check size
	size, err := store.Stat(1)
	if err != nil {
		t.Fatalf("Stat failed: %v", err)
	}
	if size != 10 {
		t.Errorf("Stat returned size %d, want 10", size)
	}

	// Read the original data
	buf := make([]byte, 5)
	n, err := store.ReadAt(1, buf, 0)
	if err != nil {
		t.Fatalf("ReadAt failed: %v", err)
	}
	if !bytes.Equal(buf[:n], data) {
		t.Errorf("ReadAt returned %q, want %q", buf[:n], data)
	}

	// Read the extended zeros
	buf = make([]byte, 5)
	n, err = store.ReadAt(1, buf, 5)
	if err != nil {
		t.Fatalf("ReadAt failed: %v", err)
	}
	expected := make([]byte, 5)
	if !bytes.Equal(buf[:n], expected) {
		t.Errorf("ReadAt returned %v, want %v (zeros)", buf[:n], expected)
	}
}

func TestMemByteStore_TruncateShrink(t *testing.T) {
	store := NewMemByteStore()

	// Write some data
	data := []byte("hello world")
	store.WriteAt(1, data, 0)

	// Truncate to smaller size
	err := store.Truncate(1, 5)
	if err != nil {
		t.Fatalf("Truncate failed: %v", err)
	}

	// Check size
	size, err := store.Stat(1)
	if err != nil {
		t.Fatalf("Stat failed: %v", err)
	}
	if size != 5 {
		t.Errorf("Stat returned size %d, want 5", size)
	}

	// Read the truncated data
	buf := make([]byte, 10)
	n, err := store.ReadAt(1, buf, 0)
	if err != io.EOF {
		t.Errorf("ReadAt returned error %v, want io.EOF", err)
	}
	if n != 5 {
		t.Errorf("ReadAt returned %d bytes, want 5", n)
	}
	if !bytes.Equal(buf[:n], []byte("hello")) {
		t.Errorf("ReadAt returned %q, want %q", buf[:n], "hello")
	}
}

func TestMemByteStore_TruncateToZero(t *testing.T) {
	store := NewMemByteStore()

	// Write some data
	data := []byte("hello world")
	store.WriteAt(1, data, 0)

	// Truncate to zero
	err := store.Truncate(1, 0)
	if err != nil {
		t.Fatalf("Truncate failed: %v", err)
	}

	// Check size
	size, err := store.Stat(1)
	if err != nil {
		t.Fatalf("Stat failed: %v", err)
	}
	if size != 0 {
		t.Errorf("Stat returned size %d, want 0", size)
	}

	// Read should return EOF
	buf := make([]byte, 10)
	n, err := store.ReadAt(1, buf, 0)
	if err != io.EOF {
		t.Errorf("ReadAt returned error %v, want io.EOF", err)
	}
	if n != 0 {
		t.Errorf("ReadAt returned %d bytes, want 0", n)
	}
}

func TestMemByteStore_TruncateNonExistent(t *testing.T) {
	store := NewMemByteStore()

	// Truncate non-existent file to zero should succeed
	err := store.Truncate(999, 0)
	if err != nil {
		t.Errorf("Truncate non-existent to zero failed: %v", err)
	}

	// Truncate non-existent file to non-zero should create it
	err = store.Truncate(999, 10)
	if err != nil {
		t.Fatalf("Truncate non-existent to non-zero failed: %v", err)
	}

	size, err := store.Stat(999)
	if err != nil {
		t.Fatalf("Stat failed: %v", err)
	}
	if size != 10 {
		t.Errorf("Stat returned size %d, want 10", size)
	}
}

func TestMemByteStore_Remove(t *testing.T) {
	store := NewMemByteStore()

	// Write some data
	data := []byte("hello world")
	store.WriteAt(1, data, 0)

	// Remove the file
	err := store.Remove(1)
	if err != nil {
		t.Fatalf("Remove failed: %v", err)
	}

	// Check size
	size, err := store.Stat(1)
	if err != nil {
		t.Fatalf("Stat failed: %v", err)
	}
	if size != 0 {
		t.Errorf("Stat after Remove returned size %d, want 0", size)
	}

	// Read should return EOF
	buf := make([]byte, 10)
	n, err := store.ReadAt(1, buf, 0)
	if err != io.EOF {
		t.Errorf("ReadAt after Remove returned error %v, want io.EOF", err)
	}
	if n != 0 {
		t.Errorf("ReadAt after Remove returned %d bytes, want 0", n)
	}
}

func TestMemByteStore_RemoveNonExistent(t *testing.T) {
	store := NewMemByteStore()

	// Remove non-existent file should succeed
	err := store.Remove(999)
	if err != nil {
		t.Errorf("Remove non-existent failed: %v", err)
	}
}

func TestMemByteStore_Stat(t *testing.T) {
	store := NewMemByteStore()

	// Stat non-existent file
	size, err := store.Stat(1)
	if err != nil {
		t.Fatalf("Stat failed: %v", err)
	}
	if size != 0 {
		t.Errorf("Stat returned size %d, want 0", size)
	}

	// Write some data
	data := []byte("hello world")
	store.WriteAt(1, data, 0)

	// Stat existing file
	size, err = store.Stat(1)
	if err != nil {
		t.Fatalf("Stat failed: %v", err)
	}
	if size != int64(len(data)) {
		t.Errorf("Stat returned size %d, want %d", size, len(data))
	}
}

func TestMemByteStore_WriteEmpty(t *testing.T) {
	store := NewMemByteStore()

	// Write empty data should be a no-op
	n, err := store.WriteAt(1, []byte{}, 0)
	if err != nil {
		t.Errorf("WriteAt empty failed: %v", err)
	}
	if n != 0 {
		t.Errorf("WriteAt empty returned %d, want 0", n)
	}

	// Size should still be 0
	size, err := store.Stat(1)
	if err != nil {
		t.Fatalf("Stat failed: %v", err)
	}
	if size != 0 {
		t.Errorf("Stat returned size %d, want 0", size)
	}
}

func TestMemByteStore_ConcurrentReads(t *testing.T) {
	store := NewMemByteStore()

	// Write some data
	data := []byte("hello world from concurrent test")
	store.WriteAt(1, data, 0)

	// Spawn multiple concurrent readers
	const numReaders = 100
	var wg sync.WaitGroup
	wg.Add(numReaders)

	errors := make(chan error, numReaders)

	for i := 0; i < numReaders; i++ {
		go func() {
			defer wg.Done()

			buf := make([]byte, len(data))
			n, err := store.ReadAt(1, buf, 0)
			if err != nil {
				errors <- err
				return
			}
			if n != len(data) {
				errors <- io.ErrShortBuffer
				return
			}
			if !bytes.Equal(buf, data) {
				errors <- io.ErrUnexpectedEOF
				return
			}
		}()
	}

	wg.Wait()
	close(errors)

	for err := range errors {
		t.Errorf("Concurrent read error: %v", err)
	}
}

func TestMemByteStore_ConcurrentWrites(t *testing.T) {
	store := NewMemByteStore()

	// Spawn multiple concurrent writers to different files
	const numWriters = 100
	var wg sync.WaitGroup
	wg.Add(numWriters)

	errors := make(chan error, numWriters)

	for i := 0; i < numWriters; i++ {
		go func(ino uint64) {
			defer wg.Done()

			data := []byte{byte(ino), byte(ino >> 8), byte(ino >> 16), byte(ino >> 24)}
			n, err := store.WriteAt(ino, data, 0)
			if err != nil {
				errors <- err
				return
			}
			if n != len(data) {
				errors <- io.ErrShortWrite
				return
			}
		}(uint64(i))
	}

	wg.Wait()
	close(errors)

	for err := range errors {
		t.Errorf("Concurrent write error: %v", err)
	}

	// Verify all writes succeeded
	for i := 0; i < numWriters; i++ {
		ino := uint64(i)
		expected := []byte{byte(ino), byte(ino >> 8), byte(ino >> 16), byte(ino >> 24)}
		buf := make([]byte, len(expected))
		n, err := store.ReadAt(ino, buf, 0)
		if err != nil {
			t.Errorf("Read after concurrent writes failed for ino %d: %v", ino, err)
			continue
		}
		if n != len(expected) {
			t.Errorf("Read after concurrent writes returned %d bytes for ino %d, want %d", n, ino, len(expected))
			continue
		}
		if !bytes.Equal(buf, expected) {
			t.Errorf("Read after concurrent writes returned %v for ino %d, want %v", buf, ino, expected)
		}
	}
}

func TestMemByteStore_ConcurrentReadWrite(t *testing.T) {
	store := NewMemByteStore()

	// Initial data
	store.WriteAt(1, []byte("initial data"), 0)

	// Spawn concurrent readers and writers
	const numReaders = 50
	const numWriters = 50
	var wg sync.WaitGroup
	wg.Add(numReaders + numWriters)

	for i := 0; i < numReaders; i++ {
		go func() {
			defer wg.Done()

			buf := make([]byte, 100)
			// Just ensure no crashes, data might be in transition
			store.ReadAt(1, buf, 0)
		}()
	}

	for i := 0; i < numWriters; i++ {
		go func(n int) {
			defer wg.Done()

			data := []byte("concurrent write ")
			data = append(data, byte(n))
			store.WriteAt(1, data, int64(n%10))
		}(i)
	}

	wg.Wait()

	// Final consistency check - file should exist and be readable
	size, err := store.Stat(1)
	if err != nil {
		t.Fatalf("Stat after concurrent operations failed: %v", err)
	}
	if size == 0 {
		t.Error("File size is 0 after concurrent operations")
	}
}

func TestMemByteStore_ConcurrentTruncate(t *testing.T) {
	store := NewMemByteStore()

	// Initial data
	store.WriteAt(1, []byte("hello world"), 0)

	// Spawn concurrent truncate operations
	const numOps = 100
	var wg sync.WaitGroup
	wg.Add(numOps)

	for i := 0; i < numOps; i++ {
		go func(size int64) {
			defer wg.Done()
			store.Truncate(1, size)
		}(int64(i % 20))
	}

	wg.Wait()

	// File should still be accessible
	size, err := store.Stat(1)
	if err != nil {
		t.Fatalf("Stat after concurrent truncates failed: %v", err)
	}
	if size < 0 {
		t.Errorf("Invalid size %d after concurrent truncates", size)
	}
}

func TestMemByteStore_ConcurrentRemove(t *testing.T) {
	store := NewMemByteStore()

	// Create multiple files
	const numFiles = 100
	for i := 0; i < numFiles; i++ {
		store.WriteAt(uint64(i), []byte("data"), 0)
	}

	// Spawn concurrent remove operations
	var wg sync.WaitGroup
	wg.Add(numFiles)

	for i := 0; i < numFiles; i++ {
		go func(ino uint64) {
			defer wg.Done()
			store.Remove(ino)
		}(uint64(i))
	}

	wg.Wait()

	// All files should be removed
	for i := 0; i < numFiles; i++ {
		size, err := store.Stat(uint64(i))
		if err != nil {
			t.Errorf("Stat after remove failed for ino %d: %v", i, err)
		}
		if size != 0 {
			t.Errorf("Size is %d for ino %d after remove, want 0", size, i)
		}
	}
}

func TestMemByteStore_MultipleFiles(t *testing.T) {
	store := NewMemByteStore()

	// Write to multiple files
	files := map[uint64][]byte{
		1:  []byte("file one"),
		2:  []byte("file two"),
		10: []byte("file ten"),
		42: []byte("the answer"),
	}

	for ino, data := range files {
		n, err := store.WriteAt(ino, data, 0)
		if err != nil {
			t.Fatalf("WriteAt for ino %d failed: %v", ino, err)
		}
		if n != len(data) {
			t.Errorf("WriteAt for ino %d returned %d, want %d", ino, n, len(data))
		}
	}

	// Read them back
	for ino, expected := range files {
		buf := make([]byte, len(expected))
		n, err := store.ReadAt(ino, buf, 0)
		if err != nil {
			t.Fatalf("ReadAt for ino %d failed: %v", ino, err)
		}
		if n != len(expected) {
			t.Errorf("ReadAt for ino %d returned %d, want %d", ino, n, len(expected))
		}
		if !bytes.Equal(buf, expected) {
			t.Errorf("ReadAt for ino %d returned %q, want %q", ino, buf, expected)
		}
	}
}

func TestMemByteStore_Overwrite(t *testing.T) {
	store := NewMemByteStore()

	// Write initial data
	store.WriteAt(1, []byte("hello world"), 0)

	// Overwrite part of it
	store.WriteAt(1, []byte("WORLD"), 6)

	// Read it back
	buf := make([]byte, 11)
	n, err := store.ReadAt(1, buf, 0)
	if err != nil {
		t.Fatalf("ReadAt failed: %v", err)
	}
	if n != 11 {
		t.Errorf("ReadAt returned %d, want 11", n)
	}
	expected := []byte("hello WORLD")
	if !bytes.Equal(buf, expected) {
		t.Errorf("ReadAt returned %q, want %q", buf, expected)
	}
}
