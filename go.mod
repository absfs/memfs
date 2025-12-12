module github.com/absfs/memfs

go 1.23

require (
	github.com/absfs/absfs v0.0.0-20251208232938-aa0ca30de832
	github.com/absfs/fstesting v0.0.0-20251207022242-d748a85c4a1e
	github.com/absfs/fstools v0.0.0-00010101000000-000000000000
	github.com/absfs/inode v0.0.0-20251208170702-9db24ab95ae4
)

replace github.com/absfs/absfs => ../absfs

replace github.com/absfs/lockfs => ../lockfs

replace github.com/absfs/fstesting => ../fstesting

replace github.com/absfs/fstools => ../fstools

replace github.com/absfs/inode => ../inode
