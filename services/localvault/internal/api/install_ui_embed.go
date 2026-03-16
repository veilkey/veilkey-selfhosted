package api

import (
	"embed"
	"io/fs"
)

//go:embed install_ui_dist/*
var installUIDist embed.FS

func embeddedInstallIndex() ([]byte, bool) {
	body, err := fs.ReadFile(installUIDist, "install_ui_dist/install.html")
	if err != nil {
		return nil, false
	}
	return body, true
}

// InstallUIAssets returns the embedded install UI filesystem for serving static assets.
func InstallUIAssets() fs.FS {
	sub, err := fs.Sub(installUIDist, "install_ui_dist")
	if err != nil {
		return installUIDist
	}
	return sub
}
