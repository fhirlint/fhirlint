//go:build unix

package resultcache

import (
	"os"
	"syscall"
)

func ownedByCurrentUser(info os.FileInfo) bool {
	st, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return true // no owner to compare; do not refuse on a guess
	}
	return int(st.Uid) == os.Getuid()
}
