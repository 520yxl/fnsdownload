package app

import (
	"embed"
	"io/fs"
	"net/http"
)

//go:embed web
var webAssets embed.FS

func webHandler() http.Handler {
	sub, err := fs.Sub(webAssets, "web")
	if err != nil { panic(err) }
	return http.FileServer(http.FS(sub))
}
