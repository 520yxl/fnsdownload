package app

import (
	"os"
	"path/filepath"
)

type Config struct {
	SocketPath  string
	DataDir     string
	DownloadDir string
	GatewayPrefix string
}

func ConfigFromEnv() Config {
	appDest := env("TRIM_APPDEST", ".")
	data := env("DATA_DIR", filepath.Join(env("TRIM_PKGVAR", "."), "data"))
	downloads := env("DOWNLOAD_DIR", filepath.Join(env("TRIM_APPDEST_VOL", "."), "@appshare", "fnsdownload", "downloads"))
	return Config{
		SocketPath: filepath.Join(appDest, "app.sock"),
		DataDir: data,
		DownloadDir: downloads,
		GatewayPrefix: "/app/fnsdownload",
	}
}

func env(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}
