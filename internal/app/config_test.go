package app

import (
	"path/filepath"
	"testing"
)

func TestConfigFromEnvUsesFnOSTargetAndPersistentPaths(t *testing.T) {
	t.Setenv("TRIM_APPDEST", "/var/apps/fnsdownload/target")
	t.Setenv("TRIM_PKGVAR", "/var/apps/fnsdownload/var")
	t.Setenv("TRIM_DATA_SHARE_PATHS", "/vol1/@appshare/fnsdownload/downloads")
	t.Setenv("TRIM_APPDEST_VOL", "/vol1")
	t.Setenv("DATA_DIR", "")
	t.Setenv("DOWNLOAD_DIR", "")
	t.Setenv("UI_DIR", "")

	cfg := ConfigFromEnv()
	if want := "/var/apps/fnsdownload/target/app.sock"; cfg.SocketPath != want {
		t.Fatalf("SocketPath = %q, want %q", cfg.SocketPath, want)
	}
	if want := filepath.Join("/var/apps/fnsdownload/target", "ui"); cfg.UIPath != want {
		t.Fatalf("UIPath = %q, want %q", cfg.UIPath, want)
	}
	if want := filepath.Join("/var/apps/fnsdownload/var", "data"); cfg.DataDir != want {
		t.Fatalf("DataDir = %q, want %q", cfg.DataDir, want)
	}
	if want := "/vol1/@appshare/fnsdownload/downloads"; cfg.DownloadDir != want {
		t.Fatalf("DownloadDir = %q, want %q", cfg.DownloadDir, want)
	}
}

func TestConfigFromEnvFallsBackToPersistentDownloadsDirectory(t *testing.T) {
	t.Setenv("TRIM_APPDEST", "/target")
	t.Setenv("TRIM_PKGVAR", "/var/app")
	t.Setenv("TRIM_DATA_SHARE_PATHS", "")
	t.Setenv("DATA_DIR", "")
	t.Setenv("DOWNLOAD_DIR", "")
	t.Setenv("UI_DIR", "")
	cfg := ConfigFromEnv()
	if want := filepath.Join("/var/app", "downloads"); cfg.DownloadDir != want {
		t.Fatalf("DownloadDir = %q, want %q", cfg.DownloadDir, want)
	}
}

func TestFirstSharePath(t *testing.T) {
	if got := firstSharePath(":/vol1/share:/vol2/share"); got != "/vol1/share" {
		t.Fatalf("firstSharePath() = %q", got)
	}
}
