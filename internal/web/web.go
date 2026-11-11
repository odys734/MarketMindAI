package web

import (
	"embed"
	"io/fs"
)

//go:embed static/*
var assets embed.FS

// Filesystem returns the embedded dashboard assets.
func Filesystem() fs.FS {
	sub, err := fs.Sub(assets, "static")
	if err != nil {
		panic(err)
	}
	return sub
}
