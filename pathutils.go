package memfs

import (
	filepath "path" // force forward slash separators on all OSs.
	"strings"
)

// Abs returns name if name is an absolute path. If name is a relative
// path then an absolute path is constructed by using cwd as the current
// working directory.
func abs(cwd, name string) string {
	if filepath.IsAbs(name) {
		return name
	}
	return filepath.Join(cwd, name)
}

func popPath(path string) (string, string) {
	if path == "" {
		return "", "" // 1
	}
	if path == "/" {
		return "/", "" // 2
	}

	x := strings.Index(path, "/")
	if x == -1 {
		return path, "" // 6
	} else if x == 0 {
		return "/", strings.TrimLeft(path, "/") // 3
	}
	return path[:x], path[x+1:] // 4, 5
}
