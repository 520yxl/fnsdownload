package app

import (
	"os"
	"path/filepath"
	"strings"
)

type Config struct {
	SocketPath    string
	DataDir       string
	DownloadDir   string
	UIPath        string
	GatewayPrefix string
}

// ConfigFromEnv follows fnOS paths: TRIM_APPDEST is already the installed
// target directory; TRIM_DATA_SHARE_PATHS contains system-created shares.
func ConfigFromEnv() Config {
	appDest := env("TRIM_APPDEST", ".")
	pkgVar := env("TRIM_PKGVAR", ".")
	sharePath := firstSharePath(os.Getenv("TRIM_DATA_SHARE_PATHS"))
	if sharePath == "" {
		sharePath = filepath.Join(pkgVar, "downloads")
	}
	return Config{
		SocketPath:    filepath.Join(appDest, "app.sock"),
		DataDir:       env("DATA_DIR", filepath.Join(pkgVar, "data")),
		DownloadDir:   env("DOWNLOAD_DIR", sharePath),
		UIPath:        env("UI_DIR", filepath.Join(appDest, "ui")),
		GatewayPrefix: "/app/fnsdownload",
	}
}

func firstSharePath(value string) string {
	for _, path := range strings.Split(value, ":") {
		path = strings.TrimSpace(path)
		if path != "" {
			return path
		}
	}
	return ""
}

func env(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}
