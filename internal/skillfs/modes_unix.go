//go:build !windows

// The constant is skillfs.go's, split out only because a build tag cannot
// apply to part of a file.

//declscope:namespace skillfs
package skillfs

// modesMatter reports whether the file system carries an executable bit that
// Write can set and ExecutableBitsMatch can read back.
const modesMatter = true
