package app

import (
	"path/filepath"
	"testing"
)

func TestConfigFromEnvUsesTargetDirectoryForGatewaySocket(t *testing.T) {
	t.Setenv("TRIM_APPDEST", "/var/apps/fnsdownload/target")
	t.Setenv("TRIM_PKGVAR", "/var/apps/fnsdownload/var")
	t.Setenv("TRIM_APPDEST_VOL", "/vol1")
	cfg := ConfigFromEnv()

	wantSocket := filepath.Join("/var/apps/fnsdownload/target", "app.sock")
	if cfg.SocketPath != wantSocket {
		t.Fatalf("SocketPath = %q, want %q", cfg.SocketPath, wantSocket)
	}
	if cfg.UIPath != filepath.Join("/var/apps/fnsdownload/target", "ui") {
		t.Fatalf("UIPath = %q", cfg.UIPath)
	}
	if cfg.DataDir != filepath.Join("/var/apps/fnsdownload/var", "data") {
		t.Fatalf("DataDir = %q", cfg.DataDir)
	}
	if cfg.DownloadDir != filepath.Join("/vol1", "@appshare", "fnsdownload", "downloads") {
		t.Fatalf("DownloadDir = %q", cfg.DownloadDir)
	}
}
