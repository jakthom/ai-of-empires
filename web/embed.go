package web

import (
	"embed"
	"io/fs"
)

//go:embed all:dist
var bundle embed.FS

func Assets() fs.FS {
	assets, err := fs.Sub(bundle, "dist")
	if err != nil {
		panic(err)
	}
	return assets
}
