package memfs

import (
	"errors"
	"fmt"
	"os"
	"sort"
	"strings"
	"syscall"
	"time"
)

type dirent struct {
	Name  string
	Inode *inode
}

func (e *dirent) IsDir() bool {
	if e.Inode == nil {
		return false
	}
	return e.Inode.IsDir()
}

func (e *dirent) String() string {
	nodeStr := "(nil)"
	if e.Inode != nil {
		nodeStr = fmt.Sprintf("{Ino:%d ...}", e.Inode.Ino)
	}
	return fmt.Sprintf("entry{%q, inode%s", e.Name, nodeStr)
}

type dirs []*dirent

func (d dirs) Len() int           { return len(d) }
func (d dirs) Swap(i, j int)      { d[i], d[j] = d[j], d[i] }
func (d dirs) Less(i, j int) bool { return d[i].Name < d[j].Name }

type inode struct {
	Ino   uint64
	Mode  os.FileMode
	Nlink uint64

	Data  []byte
	Ctime time.Time // creation time
	Atime time.Time // access time
	Mtime time.Time // modification time
	Uid   uint32
	Gid   uint32

	Dir dirs
}

func (n *inode) String() string {
	if n == nil {
		return "<nil>"
	}

	list := make([]string, len(n.Dir))
	for i, e := range n.Dir {
		list[i] = e.String()
	}
	return fmt.Sprintf("{Ino:%d,Mode:%s,Nlink:%d,len(Data):%d}\n\t%s", n.Ino, n.Mode, n.Nlink, len(n.Data), strings.Join(list, ",\n"))
}

type iNo uint64

func (n *iNo) newFile(mode os.FileMode) *inode {
	*n++
	now := time.Now()
	return &inode{
		Ino:   uint64(*n),
		Atime: now,
		Mtime: now,
		Ctime: now,
		Mode:  mode &^ os.ModeType,
	}
}

func (n *iNo) newDir(mode os.FileMode) *inode {
	dir := n.newFile(mode)
	var err error
	dir.Mode = os.ModeDir | mode&^os.ModeType
	err = dir.Link(".", dir)
	if err != nil {
		panic(err)
	}
	err = dir.Link("..", dir)
	if err != nil {
		panic(err)
	}

	return dir
}

func (n *inode) Link(name string, child *inode) error {
	if !n.IsDir() {
		return errors.New("not a directory")
	}

	x := n.find(name)

	entry := &dirent{name, child}

	if x < len(n.Dir) && n.Dir[x].Name == name {
		n.linkswapi(x, entry)
		return nil
	}
	n.linki(x, entry)
	return nil
}

func (n *inode) Unlink(name string) error {

	if !n.IsDir() {
		return errors.New("not a directory")
	}

	x := n.find(name)

	if x == n.Dir.Len() || n.Dir[x].Name != name {
		return syscall.ENOENT // os.ErrNotExist
	}

	n.unlinki(x)
	return nil
}

func (n *inode) UnlinkAll() {
	for _, e := range n.Dir {
		if e.Name == ".." {
			continue
		}
		if e.Inode.Ino == n.Ino {
			e.Inode.countDown()
			continue
		}
		e.Inode.UnlinkAll()
		e.Inode.countDown()
	}
	n.Dir = n.Dir[:0]
}

func (n *inode) IsDir() bool {
	return os.ModeDir&n.Mode != 0
}

func (n *inode) resolve(path string) (*inode, error) {
	name, trim := popPath(path)
	if name == "/" {
		if trim == "" {
			return n, nil
		}
		nn, err := n.resolve(trim)
		if err != nil {
			return nil, err
		}
		if nn == nil {
			return n, nil
		}
		return nn, err
	}
	x := n.find(name)
	if x < len(n.Dir) && n.Dir[x].Name == name {
		nn := n.Dir[x].Inode
		if len(trim) == 0 {
			return nn, nil
		}
		return nn.resolve(trim)
	}
	return nil, syscall.ENOENT // os.ErrNotExist
}

func (n *inode) accessed() {
	n.Atime = time.Now()
}

func (n *inode) modified() {
	now := time.Now()
	n.Atime = now
	n.Mtime = now
}

func (n *inode) countUp() {
	n.Nlink++
	n.accessed() // (I don't think link count mod counts as node mod )
}

func (n *inode) countDown() {
	if n.Nlink == 0 {
		panic(fmt.Sprintf("inode %d negative link count", n.Ino))
	}
	n.Nlink--
	n.accessed() // (I don't think link count mod counts as node mod )
}

func (n *inode) unlinki(i int) {
	n.Dir[i].Inode.countDown()
	copy(n.Dir[i:], n.Dir[i+1:])
	n.Dir = n.Dir[:len(n.Dir)-1]
	n.modified()
}

func (n *inode) linkswapi(i int, entry *dirent) {
	n.Dir[i].Inode.countDown()
	n.Dir[i] = entry
	n.Dir[i].Inode.countUp()
	n.modified()
}

func (n *inode) linki(i int, entry *dirent) {
	n.Dir = append(n.Dir, nil)
	copy(n.Dir[i+1:], n.Dir[i:])

	n.Dir[i] = entry
	n.Dir[i].Inode.countUp()
	n.modified()
}

func (n *inode) find(name string) int {
	return sort.Search(len(n.Dir), func(i int) bool {
		return n.Dir[i].Name >= name
	})
}
