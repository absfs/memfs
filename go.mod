module github.com/absfs/memfs

go 1.25.4

require (
	github.com/absfs/absfs v0.0.0-20251109181304-77e2f9ac4448
	github.com/absfs/fstesting v0.0.0-20251207001735-c9d62652ff82
	github.com/absfs/inode v0.0.1
	github.com/absfs/lockfs v0.0.0-20251124210544-241704814c03
)

replace github.com/absfs/inode => ../inode
