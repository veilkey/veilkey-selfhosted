package api

import (
	"embed"
	"io/fs"
	"net/http"
	"path/filepath"
	"strings"

	"veilkey-vaultcenter/internal/api/admin"
)

//go:embed assets/*
var embeddedAssets embed.FS

func (s *Server) assetHandler() http.Handler {
	sub, err := fs.Sub(embeddedAssets, "assets")
	if err != nil {
		panic(err)
	}
	legacy := http.StripPrefix("/assets/", http.FileServer(http.FS(sub)))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if assetsDir := admin.DevUIAssetsDir(); assetsDir != "" {
			http.StripPrefix("/assets/", http.FileServer(http.Dir(assetsDir))).ServeHTTP(w, r)
			return
		}
		if uiAssets, ok := admin.EmbeddedUIAssets(); ok {
			name := filepath.Clean(strings.TrimPrefix(r.URL.Path, "/assets/"))
			name = strings.TrimPrefix(name, "/")
			if name != "" {
				if _, err := fs.Stat(uiAssets, name); err == nil {
					http.StripPrefix("/assets/", http.FileServer(http.FS(uiAssets))).ServeHTTP(w, r)
					return
				}
			}
		}
		legacy.ServeHTTP(w, r)
	})
}
