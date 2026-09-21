//go:build !windows

// A build tag cannot apply to part of a file, so the constant lives here.

//declscope:namespace skillfs
package skillfs

// modesMatter reports whether this platform carries an executable bit that
// Write can set and ExecutableBitsMatch can read back.
const modesMatter = true
