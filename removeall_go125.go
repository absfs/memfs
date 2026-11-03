//go:build go1.25

package memfs

// removeAllKeepsSlash reports whether os.RemoveAll on Windows refuses
// "file\" rather than dropping the trailing separator. From Go 1.25 it shares
// the Unix implementation, which drops it. It is a variable so that the tests
// can exercise both.
var removeAllKeepsSlash = false
