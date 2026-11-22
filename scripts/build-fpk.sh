#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
DIST="$ROOT/dist"
WORK_ROOT="$ROOT/.fpk-work-root"
WORK="$WORK_ROOT/fnsdownload"
SDK_ROOT="${XUNLEI_SDK_ROOT:-}"

for tool in go fnpack jq npm; do
  command -v "$tool" >/dev/null 2>&1 || { echo "$tool is required on the build host" >&2; exit 1; }
done

cd "$ROOT"
go test ./...
go vet ./...
for wizard_file in install upgrade uninstall config; do
  path="$ROOT/wizard/$wizard_file"
  [[ -s "$path" ]] || { echo "Missing wizard file: $path" >&2; exit 1; }
  jq -e 'type == "array" and all(.[]; (.stepTitle | type == "string") and (.items | type == "array"))' "$path" >/dev/null || { echo "Invalid fnOS wizard JSON/schema: $path" >&2; exit 1; }
done
jq -e 'has(".url") and (.".url" | has("fnsdownload.main"))' "$ROOT/app/ui/config" >/dev/null

# Build the Vue frontend first. Vite emits assets under /app/fnsdownload/.
(
  cd "$ROOT/frontend"
  npm install --no-audit --no-fund
  npm run build
)
[[ -s "$ROOT/internal/app/web/index.html" ]] || { echo "Vue build did not produce index.html" >&2; exit 1; }
[[ -s "$ROOT/internal/app/web/assets/app.js" ]] || { echo "Vue build did not produce assets/app.js" >&2; exit 1; }
[[ -s "$ROOT/internal/app/web/assets/app.css" ]] || { echo "Vue build did not produce assets/app.css" >&2; exit 1; }

rm -rf "$WORK_ROOT" "$DIST"
mkdir -p "$WORK_ROOT" "$DIST"

# Important: start from fnpack's own HelloFnos application template.
# This keeps the package tree, app/config defaults and lifecycle placeholders
# identical to the fnOS Quick Start project that was installed successfully.
(
  cd "$WORK_ROOT"
  fnpack create fnsdownload
)
[[ -d "$WORK" ]] || { echo "fnpack create did not create $WORK" >&2; exit 1; }

mkdir -p "$WORK/app/server" "$WORK/app/www" "$WORK/app/ui/images"
cp "$ROOT/manifest" "$WORK/manifest"
cp "$ROOT/config/privilege" "$WORK/config/privilege"
cp "$ROOT/config/resource" "$WORK/config/resource"
cp "$ROOT/app/ui/config" "$WORK/app/ui/config"
cp "$ROOT/app/ui/images/icon_0.png" "$WORK/app/ui/images/icon_0.png"
cp "$ROOT/app/ui/images/icon_1.png" "$WORK/app/ui/images/icon_1.png"

# The successful HelloFnos package serves web content from target/www.
# The Go gateway service serves the same directory after StripPrefix.
cp "$ROOT/internal/app/web/index.html" "$WORK/app/www/index.html"
cp -a "$ROOT/internal/app/web/assets/." "$WORK/app/www/assets/"

if [[ -n "$SDK_ROOT" ]]; then
  HEADER="$SDK_ROOT/include/xl_dl/xl_dl_sdk.h"
  LIB="$SDK_ROOT/lib/libdk.so"
  [[ -s "$HEADER" ]] || { echo "Missing official SDK header: $HEADER" >&2; exit 1; }
  [[ -s "$LIB" ]] || { echo "Missing official SDK library: $LIB" >&2; exit 1; }
  mkdir -p "$ROOT/third_party/xunlei/include" "$ROOT/third_party/xunlei/lib" "$WORK/app/server/lib"
  cp -a "$SDK_ROOT/include/." "$ROOT/third_party/xunlei/include/"
  cp -a "$SDK_ROOT/lib/." "$ROOT/third_party/xunlei/lib/"
  CGO_ENABLED=1 GOOS=linux GOARCH=amd64 go build -trimpath -tags xunlei_sdk -ldflags="-s -w" -o "$WORK/app/server/fnsdownload-amd64" ./cmd/fnsdownload
  cp -a "$SDK_ROOT/lib/." "$WORK/app/server/lib/"
  echo "SDK_BUILD=enabled" > "$DIST/build-info.txt"
else
  CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags="-s -w" -o "$WORK/app/server/fnsdownload-amd64" ./cmd/fnsdownload
  echo "SDK_BUILD=stub (official SDK not supplied)" > "$DIST/build-info.txt"
fi
chmod 755 "$WORK/app/server/fnsdownload-amd64"

for lifecycle in main install_init install_callback upgrade_init upgrade_callback uninstall_init uninstall_callback config_init config_callback; do
  cp "$ROOT/cmd/$lifecycle" "$WORK/cmd/$lifecycle"
done
for wizard_file in install upgrade uninstall config; do
  cp "$ROOT/wizard/$wizard_file" "$WORK/wizard/$wizard_file"
done
cp -a "$ROOT/i18n/." "$WORK/i18n/"
cp "$ROOT/ICON.PNG" "$WORK/ICON.PNG"
cp "$ROOT/ICON_256.PNG" "$WORK/ICON_256.PNG"

# fnpack owns archive generation and metadata normalization. Never patch the FPK.
(
  cd "$WORK"
  fnpack build --nosign
)
FPK="$(find "$WORK" -maxdepth 2 -type f -name "*.fpk" -print -quit)"
[[ -n "$FPK" ]] || { echo "fnpack completed without producing an FPK"; find "$WORK" -maxdepth 4 -type f -print; exit 1; }
cp "$FPK" "$DIST/fnsdownload-x86_64.fpk"

file "$DIST/fnsdownload-x86_64.fpk"
tar -tzf "$DIST/fnsdownload-x86_64.fpk" > "$DIST/fpk-files.txt"
APP_TGZ_ENTRY="$(awk '$0 ~ /app[.]tgz$/ {print; exit}' "$DIST/fpk-files.txt")"
[[ -n "$APP_TGZ_ENTRY" ]] || { echo "FPK is missing app.tgz"; exit 1; }
tar -xOf "$DIST/fnsdownload-x86_64.fpk" "$APP_TGZ_ENTRY" | tar -tzf - > "$DIST/app-files.txt"
for required in "server/fnsdownload-amd64" "www/index.html" "www/assets/app.js" "www/assets/app.css" "ui/config" "ui/images/icon_0.png" "ui/images/icon_1.png"; do
  grep -Eq "(^|\\./)$required$" "$DIST/app-files.txt" || { echo "Missing package entry: $required"; cat "$DIST/app-files.txt"; exit 1; }
done
for required in "manifest" "config/privilege" "config/resource" "cmd/main" "cmd/install_init" "cmd/install_callback" "wizard/install" "wizard/upgrade" "wizard/uninstall" "wizard/config" "app.tgz"; do
  grep -Eq "(^|\\./)$required$" "$DIST/fpk-files.txt" || { echo "Missing outer FPK entry: $required"; cat "$DIST/fpk-files.txt"; exit 1; }
done
if grep -Eq '(^|/)bin/' "$DIST/app-files.txt"; then echo "Unexpected app/bin directory"; exit 1; fi
sha256sum "$DIST/fnsdownload-x86_64.fpk" > "$DIST/SHA256SUMS.txt"
echo "FPK created: $DIST/fnsdownload-x86_64.fpk"
