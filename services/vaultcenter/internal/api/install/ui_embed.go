package install

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
