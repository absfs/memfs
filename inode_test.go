package memfs

import (
	"errors"
	"fmt"
	"os"
	filepath "path"
	"strings"
	"testing"
)

func TestPopPath(t *testing.T) {
	tests := []struct {
		Input string
		Name  string
		Trim  string
	}{
		{"", "", ""},
		{"/", "/", ""},
		{"/foo/bar/bat", "/", "foo/bar/bat"},
		{"foo/bar/bat", "foo", "bar/bat"},
		{"bar/bat", "bar", "bat"},
		{"bat", "bat", ""},
	}

	for i, test := range tests {
		name, trim := popPath(test.Input)
		t.Logf("%q, %q := popPath(%q)", name, trim, test.Input)
		if name != test.Name {
			t.Fatalf("%d: %s != %s", i, name, test.Name)
		}
		if trim != test.Trim {
			t.Fatalf("%d: %s != %s", i, trim, test.Trim)
		}
	}

}

func TestInode(t *testing.T) {
	var ino iNo
	root := ino.newDir(0777)
	children := make([]*inode, 100)
	for i := range children {
		ino++
		children[i] = ino.newFile(0666)
	}

	NlinkTest := func(location string, count int) {
		for _, n := range children {
			if n.Nlink != uint64(count) {
				t.Fatalf("%s: incorrect link count %d != %d", location, n.Nlink, count)
			}
		}
	}
	NlinkTest("NLT 1", 0)

	paths := make(map[string]*inode)
	paths["/"] = root

	for i, n := range children {
		name := fmt.Sprintf("file.%04d.txt", i+2)

		err := root.Link(name, n)
		name = filepath.Join("/", name)
		paths[name] = n
		if err != nil {
			t.Fatal(err)
		}
	}

	NlinkTest("NLT 2", 1)

	CWD := "/"
	cwd := &CWD
	Mkdir := func(path string, perm os.FileMode) error {

		if !filepath.IsAbs(path) {
			path = filepath.Join(*cwd, path)
		}

		// does this path already exist?
		_, ok := paths[path]
		if ok { // if so, error
			return os.ErrExist
		}

		// find the parent directory
		dir, name := filepath.Split(path)
		dir = filepath.Clean(dir)
		parent, ok := paths[dir]
		if !ok {
			return os.ErrNotExist
		}

		// build the node
		dirnode := ino.newDir(0777)
		dirnode.Link("..", parent)
		// add a link to the parent directory
		parent.Link(name, dirnode)

		paths[path] = dirnode

		if dirnode.Nlink != 2 {
			return fmt.Errorf("incorrect link count for %q", path)
		}
		return nil // done?
	}

	err := Mkdir("dir0001", 0777)
	if err != nil {
		t.Fatal(err)
	}

	CWD = "/dir0001"
	err = Mkdir("dir0002", 0777)
	if err != nil {
		t.Fatal(err)
	}

	dirnode, ok := paths["/dir0001/dir0002"]
	if !ok {
		t.Fatal("broken path")
	}

	// dirnode.link(name, child)
	for path, n := range paths {
		name := filepath.Base(path)
		if !strings.HasPrefix(name, "file") {
			continue
		}
		dirnode.Link(name, n)
		name = filepath.Join("/dir0001/dir0002", name)
		paths[name] = n
	}

	NlinkTest("NLT 3", 2)

	for path, _ := range paths {
		if !strings.HasPrefix(path, "/file") {
			continue
		}

		name := filepath.Base(path)
		err := root.Unlink(name)
		if err != nil {
			t.Fatalf("%s %s", name, err)
		}
		delete(paths, path)
	}

	NlinkTest("NLT 4", 1)

	type testcase struct {
		Path string
		Node *inode
	}
	testoutput := make(chan *testcase)
	var walk func(node *inode, path string) error
	walk = func(node *inode, path string) error {
		testoutput <- &testcase{path, node}

		if !node.IsDir() {
			if node.Dir.Len() != 0 {
				return errors.New("is directory")
			}
			return nil
		}
		for _, suffix := range []string{"/.", "/.."} {
			if strings.HasSuffix(path, suffix) {
				return nil
			}
		}

		if path == "/" {
			path = ""
		}
		for _, entry := range node.Dir {
			err := walk(entry.Inode, path+"/"+entry.Name)
			if err != nil {
				return err
			}
		}
		return nil
	}

	go func() {
		defer close(testoutput)
		err = walk(root, "/")
		if err != nil {
			t.Fatal(err)
		}
	}()
	for test := range testoutput {
		fmt.Printf("%s %d\n", test.Path, test.Node.Ino)
	}
}

func TestResolve(t *testing.T) {
	ino := new(iNo)

	var root, parent, dir *inode
	root = ino.newDir(0777)
	parent = root

	dir = ino.newDir(0777)
	err := parent.Link("tmp", dir)
	if err != nil {
		t.Fatal(err)
	}
	err = dir.Link("..", parent)
	if err != nil {
		t.Fatal(err)
	}

	parent = dir
	dir = ino.newDir(0777)
	parent.Link("foo", dir)
	err = dir.Link("..", parent)
	if err != nil {
		t.Fatal(err)
	}

	dir = ino.newDir(0777)
	parent.Link("bar", dir)
	err = dir.Link("..", parent)
	if err != nil {
		t.Fatal(err)
	}

	dir = ino.newDir(0777)
	parent.Link("bat", dir)
	err = dir.Link("..", parent)
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		Path string
		Ino  uint64
	}{
		{
			Path: "/",
			Ino:  1,
		},
		{
			Path: "/.",
			Ino:  1,
		},
		{
			Path: "/..",
			Ino:  1,
		},
		{
			Path: "/tmp",
			Ino:  2,
		},
		{
			Path: "/tmp/.",
			Ino:  2,
		},
		{
			Path: "/tmp/..",
			Ino:  1,
		},
		{
			Path: "/tmp/bar",
			Ino:  4,
		},
		{
			Path: "/tmp/bar/.",
			Ino:  4,
		},
		{
			Path: "/tmp/bar/..",
			Ino:  2,
		},
		{
			Path: "/tmp/bat",
			Ino:  5,
		},
		{
			Path: "/tmp/bat/.",
			Ino:  5,
		},
		{
			Path: "/tmp/bat/..",
			Ino:  2,
		},
		{
			Path: "/tmp/foo",
			Ino:  3,
		},
		{
			Path: "/tmp/foo/.",
			Ino:  3,
		},
		{
			Path: "/tmp/foo/..",
			Ino:  2,
		},
	}
	_ = tests
	count := 0

	type testcase struct {
		Path string
		Node *inode
	}

	testoutput := make(chan *testcase)
	var walk func(node *inode, path string) error
	walk = func(node *inode, path string) error {
		count++
		if count > 20 {
			return errors.New("counted to far")
		}

		// fmt.Printf("%d %d %s\n", node.Ino, node.Nlink, path)
		testoutput <- &testcase{path, node}

		if !node.IsDir() {
			if node.Dir.Len() != 0 {
				return errors.New("is directory")
			}
			return nil
		}
		for _, suffix := range []string{"/.", "/.."} {
			if strings.HasSuffix(path, suffix) {
				return nil
			}
		}

		if path == "/" {
			path = ""
		}
		for _, entry := range node.Dir {
			err := walk(entry.Inode, path+"/"+entry.Name)
			if err != nil {
				return err
			}
		}
		return nil
	}
	go func() {
		defer close(testoutput)
		err = walk(root, "/")
		if err != nil {
			t.Fatal(err)
		}
	}()

	i := 0
	for test := range testoutput {
		if tests[i].Path != test.Path {
			t.Errorf("Path: expected %q, got %q", tests[i].Path, test.Path)
		}

		if tests[i].Ino != test.Node.Ino {
			t.Errorf("Ino: expected %d, got %d -- %q, %q", tests[i].Ino, test.Node.Ino, tests[i].Path, test.Path)
		}
		i++
	}

	t.Run("resolve", func(t *testing.T) {
		tests := make(map[string]uint64)
		tests["/"] = 1
		tests["/tmp"] = 2
		tests["/tmp/bar"] = 4
		tests["/tmp/bat"] = 5
		tests["/tmp/foo"] = 3
		var dir *inode
		for Path, Ino := range tests {
			node, err := root.resolve(Path)
			if err != nil {
				t.Fatal(err)
			}
			if Path == "/tmp/foo" {
				dir = node
			}
			if node.Ino != Ino {
				t.Fatalf("Ino: %d, Expected: %d\n", node.Ino, Ino)
			}
		}

		// test relative paths
		tests = make(map[string]uint64)
		tests["../.."] = 1
		tests[".."] = 2
		tests["../bar"] = 4
		tests["../bat"] = 5
		tests["."] = 3
		for Path, Ino := range tests {
			node, err := dir.resolve(Path)
			if err != nil {
				t.Fatal(err)
			}

			t.Logf("%d %q \t%q", node.Ino, Path, filepath.Join("/tmp/foo", Path))
			if node.Ino != Ino {
				t.Fatalf("Ino: %d, Expected: %d\n", node.Ino, Ino)
			}
		}
	})

}
