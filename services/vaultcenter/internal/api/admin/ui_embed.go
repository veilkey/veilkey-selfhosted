package admin

import (
	"embed"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

//go:embed ui_dist/index.html ui_dist/install.html ui_dist/setup.html ui_dist/favicon.svg all:ui_dist/assets
var embeddedUIFS embed.FS

// EmbeddedUIIndex returns the content of ui_dist/index.html.
func EmbeddedUIIndex() ([]byte, bool) {
	body, err := fs.ReadFile(embeddedUIFS, "ui_dist/index.html")
	if err != nil {
		return nil, false
	}
	return body, true
}

// EmbeddedUIInstallFile returns the content of ui_dist/install.html.
func EmbeddedUIInstallFile() ([]byte, bool) {
	body, err := fs.ReadFile(embeddedUIFS, "ui_dist/install.html")
	if err != nil {
		return nil, false
	}
	return body, true
}

// EmbeddedUISetupFile returns the content of ui_dist/setup.html.
func EmbeddedUISetupFile() ([]byte, bool) {
	body, err := fs.ReadFile(embeddedUIFS, "ui_dist/setup.html")
	if err != nil {
		return nil, false
	}
	return body, true
}

// EmbeddedUIStaticFile returns the content of a named file from ui_dist/.
// It does not serve files under the assets/ subdirectory (those are served
// via EmbeddedUIAssets).
func EmbeddedUIStaticFile(name string) ([]byte, bool) {
	clean := strings.TrimPrefix(filepath.Clean(name), "/")
	if clean == "" || strings.HasPrefix(clean, "assets/") {
		return nil, false
	}
	body, err := fs.ReadFile(embeddedUIFS, filepath.Join("ui_dist", clean))
	if err != nil {
		return nil, false
	}
	return body, true
}

// EmbeddedUIAssets returns the fs.FS rooted at ui_dist/assets.
func EmbeddedUIAssets() (fs.FS, bool) {
	sub, err := fs.Sub(embeddedUIFS, "ui_dist/assets")
	if err != nil {
		return nil, false
	}
	return sub, true
}

// devUIDir returns the dev-mode override directory from the environment.
func devUIDir() string {
	return strings.TrimSpace(os.Getenv("VEILKEY_UI_DEV_DIR"))
}

// DevUIIndex reads index.html from the dev override directory (if set).
func DevUIIndex() ([]byte, bool) {
	devDir := devUIDir()
	if devDir == "" {
		return nil, false
	}
	path := filepath.Join(devDir, "index.html")
	if body, err := os.ReadFile(path); err == nil {
		return body, true
	}
	return nil, false
}

// DevUIStaticFile reads a named file from the dev override directory.
func DevUIStaticFile(name string) ([]byte, bool) {
	devDir := devUIDir()
	if devDir == "" {
		return nil, false
	}
	path := filepath.Join(devDir, filepath.Clean(name))
	body, err := os.ReadFile(path)
	if err != nil {
		return nil, false
	}
	return body, true
}

// DevUIAssetsDir returns the assets subdirectory of the dev override dir, if present.
func DevUIAssetsDir() string {
	devDir := devUIDir()
	if devDir == "" {
		return ""
	}
	assetsDir := filepath.Join(devDir, "assets")
	if info, err := os.Stat(assetsDir); err == nil && info.IsDir() {
		return assetsDir
	}
	return ""
}
