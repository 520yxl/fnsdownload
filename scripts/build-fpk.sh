#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
DIST="$ROOT/dist"
WORK="$ROOT/.fpk-work"
SDK_ROOT="${XUNLEI_SDK_ROOT:-}"
command -v go >/dev/null 2>&1 || { echo "Go is required on the build host" >&2; exit 1; }
command -v fnpack >/dev/null 2>&1 || { echo "Official fnpack is required on the build host" >&2; exit 1; }
cd "$ROOT"
go test ./...
go vet ./...
rm -rf "$WORK" "$DIST"
mkdir -p "$WORK/config" "$WORK/cmd" "$WORK/wizard" "$WORK/i18n" "$WORK/app/bin" "$WORK/app/server" "$WORK/app/ui/assets" "$WORK/app/ui/images" "$DIST"
if [[ -n "$SDK_ROOT" ]]; then
  HEADER="$SDK_ROOT/include/xl_dl/xl_dl_sdk.h"
  LIB="$SDK_ROOT/lib/libdk.so"
  [[ -s "$HEADER" ]] || { echo "Missing official SDK header: $HEADER" >&2; exit 1; }
  [[ -s "$LIB" ]] || { echo "Missing official SDK library: $LIB" >&2; exit 1; }
  mkdir -p "$ROOT/third_party/xunlei/include" "$ROOT/third_party/xunlei/lib"
  cp -a "$SDK_ROOT/include/." "$ROOT/third_party/xunlei/include/"
  cp -a "$SDK_ROOT/lib/." "$ROOT/third_party/xunlei/lib/"
  CGO_ENABLED=1 GOOS=linux GOARCH=amd64 go build -trimpath -tags xunlei_sdk -ldflags="-s -w" -o "$WORK/app/server/fnsdownload-amd64" ./cmd/fnsdownload
  mkdir -p "$WORK/app/server/lib"
  cp -a "$SDK_ROOT/lib/." "$WORK/server/lib/"
  echo "SDK_BUILD=enabled" > "$DIST/build-info.txt"
else
  CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags="-s -w" -o "$WORK/app/server/fnsdownload-amd64" ./cmd/fnsdownload
  echo "SDK_BUILD=stub (official SDK not supplied)" > "$DIST/build-info.txt"
fi
ln -s ../server/fnsdownload-amd64 "$WORK/app/bin/fnsdownload-amd64"
cp "$ROOT/internal/app/web/index.html" "$WORK/app/ui/index.html"
cp -a "$ROOT/internal/app/web/assets/." "$WORK/app/ui/assets/"
cp "$ROOT/app/ui/config" "$WORK/app/ui/config"
cp -a "$ROOT/app/ui/images/." "$WORK/app/ui/images/"
cp "$ROOT/manifest" "$WORK/manifest"
cp -a "$ROOT/config/." "$WORK/config/"
cp -a "$ROOT/cmd/." "$WORK/cmd/"
cp -a "$ROOT/wizard/." "$WORK/wizard/"
if [[ -d "$ROOT/i18n" ]]; then cp -a "$ROOT/i18n/." "$WORK/i18n/"; fi
cp "$ROOT/ICON.PNG" "$WORK/ICON.PNG"
cp "$ROOT/ICON_256.PNG" "$WORK/ICON_256.PNG"
(
  cd "$WORK"
  fnpack build --directory . --nosign
)
FPK="$(find "$ROOT" -type f -name "*.fpk" ! -path "$DIST/*" -print -quit)"
[[ -n "$FPK" ]] || { echo "fnpack did not produce an FPK" >&2; exit 1; }
cp "$FPK" "$DIST/fnsdownload-x86_64.fpk"
file "$DIST/fnsdownload-x86_64.fpk"
tar -tzf "$DIST/fnsdownload-x86_64.fpk" > "$DIST/fpk-files.txt"
cat "$DIST/fpk-files.txt"
APP_TGZ_ENTRY="$(grep -E '(^|/)app.tgz
for required in "ui/index.html" "ui/config" "server/fnsdownload-amd64" "ui/assets/app.css" "ui/assets/app.js" "ui/images/icon_0.png" "ui/images/icon_1.png"; do grep -Fxq "$required" "$DIST/app-files.txt" || { echo "Missing package entry: $required" >&2; exit 1; }; done
echo "FPK created: $DIST/fnsdownload-x86_64.fpk"
echo "Build details: $DIST/build-info.txt"
 "$DIST/fpk-files.txt" | head -n 1)"
[[ -n "$APP_TGZ_ENTRY" ]] || { echo "FPK is missing app.tgz" >&2; exit 1; }
tar -xOf "$DIST/fnsdownload-x86_64.fpk" "$APP_TGZ_ENTRY" | tar -tzf - > "$DIST/app-files.txt"
for required in "ui/index.html" "ui/config" "server/fnsdownload-amd64" "ui/assets/app.css" "ui/assets/app.js" "ui/images/icon_0.png" "ui/images/icon_1.png"; do grep -Fxq "$required" "$DIST/app-files.txt" || { echo "Missing package entry: $required" >&2; exit 1; }; done
echo "FPK created: $DIST/fnsdownload-x86_64.fpk"
echo "Build details: $DIST/build-info.txt"
