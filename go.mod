module github.com/absfs/memfs

go 1.23

require (
	github.com/absfs/absfs v1.0.0
	github.com/absfs/fstesting v0.9.1
	github.com/absfs/fstools v0.9.1
	github.com/absfs/inode v1.0.0
)

replace github.com/absfs/inode => ../inode
