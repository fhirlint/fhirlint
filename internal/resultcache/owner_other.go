//go:build !unix

package resultcache

import "os"

// ownedByCurrentUser has no owner to compare outside Unix, so it accepts the
// directory. Windows profile directories are per-user already.
func ownedByCurrentUser(os.FileInfo) bool { return true }
