//go:build !dev

package miyabi

import (
	"embed"
	"io/fs"
)

//go:embed web/dist
var embeddedFrontend embed.FS

func Frontend() fs.FS {
	frontend, err := fs.Sub(embeddedFrontend, "web/dist")
	if err != nil {
		panic(err)
	}
	return frontend
}
