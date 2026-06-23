package discodrive

import (
	"embed"
	"io/fs"
)

// webDist is the built Nuxt SPA (Docker node stage / npm run generate). The all:
// prefix is required so that `_nuxt` assets are included (embed skips them otherwise).
//
//go:embed all:web/dist
var webDist embed.FS

// WebUI returns an FS rooted at web/dist with the compiled UI static files.
func WebUI() fs.FS {
	sub, err := fs.Sub(webDist, "web/dist")
	if err != nil {
		return webDist
	}
	return sub
}
