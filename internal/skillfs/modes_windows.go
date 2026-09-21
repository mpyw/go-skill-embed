//go:build windows

// The constant is skillfs.go's, split out only because a build tag cannot
// apply to part of a file.

//declscope:namespace skillfs
package skillfs

// modesMatter reports whether the file system carries an executable bit that
// Write can set and ExecutableBitsMatch can read back.
//
// Windows has none. os.Stat builds a regular file's mode from the read-only
// attribute alone, so Mode()&0o111 is zero for every file and a rule that
// answers true can never agree with it.
const modesMatter = false
