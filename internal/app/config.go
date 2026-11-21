package app

import (
	"os"
	"path/filepath"
)

type Config struct {
	ListenAddr  string
	DataDir     string
	DownloadDir string
}

func ConfigFromEnv() Config {
	data := env("DATA_DIR", filepath.Join(env("TRIM_PKGVAR", "."), "data"))
	downloads := env("DOWNLOAD_DIR", filepath.Join(env("TRIM_APPDEST_VOL", "."), "@appshare", "fnsdownload", "downloads"))
	return Config{
		ListenAddr:  ":" + env("PORT", "8088"),
		DataDir:     data,
		DownloadDir: downloads,
	}
}

func env(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}
