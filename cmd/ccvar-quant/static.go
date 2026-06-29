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
			setWebCacheHeaders(w, cleanPath)
			fileServer.ServeHTTP(w, r)
			return
		}
		if _, err := fs.Stat(webFS, cleanPath); err != nil {
			if isStaticFilePath(cleanPath) {
				setWebCacheHeaders(w, cleanPath)
				http.NotFound(w, r)
				return
			}
			setWebCacheHeaders(w, "index.html")
			r.URL.Path = "/"
			fileServer.ServeHTTP(w, r)
			return
		}
		setWebCacheHeaders(w, cleanPath)
		fileServer.ServeHTTP(w, r)
	})
	return nil
}

func isStaticFilePath(cleanPath string) bool {
	return strings.HasPrefix(cleanPath, "assets/") || path.Ext(cleanPath) != ""
}

func setWebCacheHeaders(w http.ResponseWriter, cleanPath string) {
	if cleanPath == "" || cleanPath == "." || cleanPath == "index.html" {
		w.Header().Set("Cache-Control", "no-store, no-cache, must-revalidate, max-age=0")
		w.Header().Set("Pragma", "no-cache")
		w.Header().Set("Expires", "0")
		return
	}
	w.Header().Set("Cache-Control", "no-cache")
}
