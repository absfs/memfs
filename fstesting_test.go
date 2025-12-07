package memfs_test

import (
	"testing"

	"github.com/absfs/fstesting"
	"github.com/absfs/memfs"
)

// TestMemFSSuite runs the standard fstesting suite against memfs.
func TestMemFSSuite(t *testing.T) {
	fs, err := memfs.NewFS()
	if err != nil {
		t.Fatalf("failed to create memfs: %v", err)
	}

	suite := &fstesting.Suite{
		FS: fs,
		Features: fstesting.Features{
			Symlinks:      true,
			HardLinks:     false,
			Permissions:   true,
			Timestamps:    true,
			CaseSensitive: true,
			AtomicRename:  true,
			SparseFiles:   false,
			LargeFiles:    true,
		},
	}

	suite.Run(t)
}
