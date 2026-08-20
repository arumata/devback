//go:build !unix

package filesystem

import (
	"github.com/arumata/devback/internal/usecase"
)

// fileIDFromSys reports no file identity on platforms without unix inodes.
func fileIDFromSys(sys interface{}) (usecase.FileID, bool) {
	return usecase.FileID{}, false
}
