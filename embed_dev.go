//go:build dev

package miyabi

import "io/fs"

func Frontend() fs.FS {
	return nil
}
