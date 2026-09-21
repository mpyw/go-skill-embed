//go:build windows

// A build tag cannot apply to part of a file, so the constant lives here.

//declscope:namespace skillfs
package skillfs

// modesMatter reports whether this platform carries an executable bit that
// Write can set and ExecutableBitsMatch can read back.
//
// Windows carries none. os.Stat builds a regular file's mode from the
// read-only attribute alone, so Mode()&0o111 is zero for a regular file and a
// rule that answers true can never agree with it.
const modesMatter = false
