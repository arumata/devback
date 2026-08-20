//go:build unix

package filesystem

import (
	"syscall"

	"github.com/arumata/devback/internal/usecase"
)

// fileIDFromSys extracts the device/inode pair from os.FileInfo.Sys().
func fileIDFromSys(sys interface{}) (usecase.FileID, bool) {
	st, ok := sys.(*syscall.Stat_t)
	if !ok || st == nil {
		return usecase.FileID{}, false
	}
	// Dev/Ino types differ across unix platforms (e.g. int32 Dev on darwin),
	// so the conversions are required even where they look redundant.
	// #nosec G115 -- dev/ino are opaque identifiers, sign is irrelevant.
	return usecase.FileID{Dev: uint64(st.Dev), Ino: uint64(st.Ino)}, true //nolint:unconvert
}
