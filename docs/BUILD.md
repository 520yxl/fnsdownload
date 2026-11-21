# Native fnOS FPK Build

Target: Linux x86_64 / amd64 only. fnOS Manifest platform value: x86.

## Package layout
FPK root: manifest, config/, cmd/, wizard/, i18n/, ICON.PNG, ICON_256.PNG and app.tgz.
The app archive must contain server/fnsdownload-amd64 and the complete ui/ tree including ui/config and both desktop icons.

## fnOS runtime paths
Use TRIM_APPDEST, TRIM_PKGETC, TRIM_PKGVAR, TRIM_PKGHOME, TRIM_DATA_SHARE_PATHS and TRIM_TEMP_LOGFILE. Do not hard-code volume numbers or treat TRIM_APPDEST as the app's parent directory.

## Build and verify
```bash
go test ./...
go vet ./...
bash -n cmd/*
./scripts/build-fpk.sh
```

CI package checks are structural checks only. The resulting FPK must be installed and launched on a physical fnOS x86_64 NAS before it is considered compatible. A build without the official Xunlei SDK is only a gateway/UI development package, not a production downloader.
