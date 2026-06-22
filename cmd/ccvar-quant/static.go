package main

import (
	"embed"
	"errors"
	"io/fs"
	"net/http"
	"path"
	"strings"
)

//go:embed all:web
var embeddedWeb embed.FS

func mountWeb(mux *http.ServeMux) error {
	webFS, err := fs.Sub(embeddedWeb, "web")
	if err != nil {
		return err
	}
	fileServer := http.FileServer(http.FS(webFS))
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api/") {
			writeErrorJSON(w, http.StatusNotFound, errors.New("api route not found"))
			return
		}
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			writeErrorJSON(w, http.StatusMethodNotAllowed, errors.New("method not allowed"))
			return
		}
		cleanPath := strings.TrimPrefix(path.Clean(r.URL.Path), "/")
		if cleanPath == "." || cleanPath == "" {
			fileServer.ServeHTTP(w, r)
			return
		}
		if _, err := fs.Stat(webFS, cleanPath); err != nil {
			r.URL.Path = "/"
		}
		fileServer.ServeHTTP(w, r)
	})
	return nil
}
