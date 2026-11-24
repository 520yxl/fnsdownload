package app

import (
	xunleiengine "github.com/520yxl/fnsdownload/internal/engine/xunlei"
	"os"
	"path/filepath"
)

type Config struct {
	Xunlei        xunleiengine.Config
	SocketPath    string
	DataDir       string
	DatabasePath  string
	DownloadDir   string
	UIPath        string
	GatewayPrefix string
	Aria2Binary   string
	Aria2StateDir string
}

func ConfigFromEnv() Config {
	appDest := env("TRIM_APPDEST", ".")
	data := env("DATA_DIR", filepath.Join(env("TRIM_PKGVAR", "."), "data"))
	downloads := env("DOWNLOAD_DIR", filepath.Join(env("TRIM_APPDEST_VOL", "."), "@appshare", "fnsdownload", "downloads"))
	return Config{
		Xunlei:        xunleiengine.Config{BundledSecretFile: filepath.Join(appDest, "config", "thunder-auth.secret"), AppID: env("XUNLEI_APP_ID", "eGwtQ28zcDExODg2AAAAA24uAAA="), Version: "0.5.26", StateDir: filepath.Join(data, "xunlei"), AuthURL: env("THUNDER_AUTH_URL", "https://thunder-auth.moxyi.cn"), SecretFile: env("THUNDER_AUTH_SECRET_FILE", filepath.Join(env("TRIM_PKGVAR", "."), "config", "thunder-auth.secret"))},
		SocketPath:    filepath.Join(appDest, "app.sock"),
		DataDir:       data,
		DatabasePath:  filepath.Join(data, "tasks.db"),
		DownloadDir:   downloads,
		UIPath:        env("UI_DIR", filepath.Join(appDest, "www")),
		GatewayPrefix: "/app/fnsdownload",
		Aria2Binary:   filepath.Join(appDest, "server", "aria2", "aria2c"),
		Aria2StateDir: filepath.Join(data, "aria2"),
	}
}

func env(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}
