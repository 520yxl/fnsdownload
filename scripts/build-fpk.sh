#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
DIST="$ROOT/dist"
WORK="$ROOT/.fpk-work"
SDK_ROOT="${XUNLEI_SDK_ROOT:-}"
command -v go >/dev/null 2>&1 || { echo "Go is required on the build host" >&2; exit 1; }
command -v fnpack >/dev/null 2>&1 || { echo "Official fnpack is required on the build host" >&2; exit 1; }
command -v jq >/dev/null 2>&1 || { echo "jq is required to validate wizard JSON" >&2; exit 1; }
cd "$ROOT"
launch_name="$(sed -n 's/^desktop_applaunchname=//p' manifest | tr -d '\r')"
[[ -n "$launch_name" ]] || { echo "Missing desktop_applaunchname in manifest" >&2; exit 1; }
jq -e --arg name "$launch_name" 'has(".url") and (.".url" | has($name))' app/ui/config >/dev/null || { echo "Desktop launcher mismatch: manifest=$launch_name" >&2; exit 1; }
go test ./...
go vet ./...
for wizard_file in install upgrade uninstall config; do
  path="$ROOT/wizard/$wizard_file"
  [[ -s "$path" ]] || { echo "Missing wizard file: $path" >&2; exit 1; }
  jq -e 'type == "array" and all(.[]; (.stepTitle | type == "string") and (.items | type == "array"))' "$path" >/dev/null || { echo "Invalid fnOS wizard JSON/schema: $path" >&2; exit 1; }
done
rm -rf "$WORK" "$DIST"
mkdir -p "$WORK/config" "$WORK/cmd" "$WORK/wizard" "$WORK/i18n" "$WORK/app/server" "$WORK/app/ui/assets" "$WORK/app/ui/images" "$DIST"
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
  cp -a "$SDK_ROOT/lib/." "$WORK/app/server/lib/"
  echo "SDK_BUILD=enabled" > "$DIST/build-info.txt"
else
  CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags="-s -w" -o "$WORK/app/server/fnsdownload-amd64" ./cmd/fnsdownload
  echo "SDK_BUILD=stub (official SDK not supplied)" > "$DIST/build-info.txt"
fi
cp "$ROOT/internal/app/web/index.html" "$WORK/app/ui/index.html"
cp -a "$ROOT/internal/app/web/assets/." "$WORK/app/ui/assets/"
cp "$ROOT/app/ui/config" "$WORK/app/ui/config"
cp -a "$ROOT/app/ui/images/." "$WORK/app/ui/images/"
cp "$ROOT/manifest" "$WORK/manifest"
cp -a "$ROOT/config/." "$WORK/config/"
cp -a "$ROOT/cmd/." "$WORK/cmd/"
for wizard_file in install upgrade uninstall config; do cp "$ROOT/wizard/$wizard_file" "$WORK/wizard/$wizard_file"; done
cp -a "$ROOT/i18n/." "$WORK/i18n/"
cp "$ROOT/ICON.PNG" "$WORK/ICON.PNG"
cp "$ROOT/ICON_256.PNG" "$WORK/ICON_256.PNG"
if [[ "$(id -u)" -eq 0 ]]; then chown -R 0:0 "$WORK/app" "$WORK/config" "$WORK/cmd" "$WORK/wizard" "$WORK/i18n" "$WORK/manifest" "$WORK/ICON.PNG" "$WORK/ICON_256.PNG"; elif command -v sudo >/dev/null 2>&1; then sudo -n chown -R 0:0 "$WORK/app" "$WORK/config" "$WORK/cmd" "$WORK/wizard" "$WORK/i18n" "$WORK/manifest" "$WORK/ICON.PNG" "$WORK/ICON_256.PNG"; else echo "Need root ownership for FPK payload; run as root or install sudo." >&2; exit 1; fi
(
  cd "$WORK"
  sudo -n fnpack build --directory . --nosign
)
FPK="$(find "$WORK" -type f -name "*.fpk" -print -quit)"
if [[ -z "$FPK" ]]; then FPK="$(find "$ROOT" -maxdepth 1 -type f -name "*.fpk" -print -quit)"; fi
[[ -n "$FPK" ]] || { echo "fnpack reported success but no FPK file was found"; find "$WORK" -maxdepth 3 -type f -printf "%p (%s bytes)\n"; exit 1; }
cp "$FPK" "$DIST/fnsdownload-x86_64.fpk"
file "$DIST/fnsdownload-x86_64.fpk"
tar -tzf "$DIST/fnsdownload-x86_64.fpk" > "$DIST/fpk-files.txt"
APP_TGZ_ENTRY="$(awk '$0 ~ /app[.]tgz$/ {print; exit}' "$DIST/fpk-files.txt")"
[[ -n "$APP_TGZ_ENTRY" ]] || { echo "FPK is missing app.tgz"; cat "$DIST/fpk-files.txt"; exit 1; }
tar -xOf "$DIST/fnsdownload-x86_64.fpk" "$APP_TGZ_ENTRY" | tar -tzf - > "$DIST/app-files.txt"
for required in "ui/index.html" "ui/config" "server/fnsdownload-amd64" "ui/assets/app.css" "ui/assets/app.js" "ui/images/icon_0.png" "ui/images/icon_1.png"; do grep -Fxq "$required" "$DIST/app-files.txt" || { echo "Missing package entry: $required"; cat "$DIST/app-files.txt"; exit 1; }; done
for required in "install" "upgrade" "uninstall" "config"; do grep -Eq "(^|\\./)wizard/$required$" "$DIST/fpk-files.txt" || { echo "Missing outer FPK entry: wizard/$required"; cat "$DIST/fpk-files.txt"; exit 1; }; done
if grep -Eq '(^|/)bin/' "$DIST/app-files.txt"; then echo "Unexpected app/bin directory in app.tgz"; cat "$DIST/app-files.txt"; exit 1; fi
echo "FPK created: $DIST/fnsdownload-x86_64.fpk"
echo "Build details: $DIST/build-info.txt"
