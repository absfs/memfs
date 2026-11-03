//go:build !go1.25

package memfs

// removeAllKeepsSlash reports whether os.RemoveAll on Windows refuses
// "file\" rather than dropping the trailing separator. Before Go 1.25 it
// does. It is a variable so that the tests can exercise both.
var removeAllKeepsSlash = true
