#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
DIST="$ROOT/dist"
WORK_ROOT="$ROOT/.fpk-work-root"
WORK="$WORK_ROOT/fnsdownload"
SDK_ROOT="${XUNLEI_SDK_ROOT:-}"

for tool in go fnpack jq npm python3; do
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
jq -e 'has(".url") and (.".url" | has("fnsdownload.main")) and (.".url"."fnsdownload.main".gatewayPrefix == "/app/fnsdownload") and (.".url"."fnsdownload.main".gatewaySocket == "app.sock") and (.".url"."fnsdownload.main".url == "/app/fnsdownload/") and (.".url"."fnsdownload.main".icon == "images/icon_{0}.png")' "$ROOT/app/ui/config" >/dev/null
grep -Fxq 'desktop_uidir=ui' "$ROOT/manifest" || { echo "manifest must set desktop_uidir=ui"; exit 1; }
grep -Fxq 'desktop_applaunchname=fnsdownload.main' "$ROOT/manifest" || { echo "manifest desktop_applaunchname must match app/ui/config entry ID"; exit 1; }
for icon in icon_64.png icon_256.png; do
  [[ -s "$ROOT/app/ui/images/$icon" ]] || { echo "Missing entry icon: $icon"; exit 1; }
done

# Build the Vue frontend first. Vite emits assets under /app/fnsdownload/.
(
  cd "$ROOT/frontend"
  npm ci --no-audit --no-fund
  npm run build
)
[[ -s "$ROOT/internal/app/web/index.html" ]] || { echo "Vue build did not produce index.html" >&2; exit 1; }
[[ -s "$ROOT/internal/app/web/assets/app.js" ]] || { echo "Vue build did not produce assets/app.js" >&2; exit 1; }
[[ -s "$ROOT/internal/app/web/assets/app.css" ]] || { echo "Vue build did not produce assets/app.css" >&2; exit 1; }
grep -q '/app/fnsdownload/assets/app.js' "$ROOT/internal/app/web/index.html" || { echo "Frontend JS URL is outside the fnOS gateway prefix"; exit 1; }
grep -q '/app/fnsdownload/assets/app.css' "$ROOT/internal/app/web/index.html" || { echo "Frontend CSS URL is outside the fnOS gateway prefix"; exit 1; }

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
cp "$ROOT/app/ui/images/icon_64.png" "$WORK/app/ui/images/icon_64.png"
cp "$ROOT/app/ui/images/icon_256.png" "$WORK/app/ui/images/icon_256.png"

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
  if [[ "$(cd "$SDK_ROOT" && pwd)" != "$ROOT/third_party/xunlei" ]]; then
    cp -a "$SDK_ROOT/include/." "$ROOT/third_party/xunlei/include/"
    cp -a "$SDK_ROOT/lib/." "$ROOT/third_party/xunlei/lib/"
  fi
  export LD_LIBRARY_PATH="$ROOT/third_party/xunlei/lib${LD_LIBRARY_PATH:+:$LD_LIBRARY_PATH}"
  CGO_ENABLED=1 GOOS=linux GOARCH=amd64 go test -tags xunlei_sdk ./...
  CGO_ENABLED=1 GOOS=linux GOARCH=amd64 go vet -tags xunlei_sdk ./...
  CGO_ENABLED=1 GOOS=linux GOARCH=amd64 go build -trimpath -tags xunlei_sdk -ldflags="-s -w" -o "$WORK/app/server/fnsdownload-amd64" ./cmd/fnsdownload
  cp -a "$SDK_ROOT/lib/." "$WORK/app/server/lib/"
  ldd "$WORK/app/server/fnsdownload-amd64"
  if ldd "$WORK/app/server/fnsdownload-amd64" | grep -q 'not found'; then echo 'Missing SDK runtime dependency'; exit 1; fi
  readelf -d "$WORK/app/server/fnsdownload-amd64" | grep -F '$ORIGIN/lib'
  echo "SDK_BUILD=enabled" > "$DIST/build-info.txt"
else
  CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags="-s -w" -o "$WORK/app/server/fnsdownload-amd64" ./cmd/fnsdownload
  echo "SDK_BUILD=stub (official SDK not supplied)" > "$DIST/build-info.txt"
fi
chmod 755 "$WORK/app/server/fnsdownload-amd64"

for lifecycle in main install_init install_callback upgrade_init upgrade_callback uninstall_init uninstall_callback config_init config_callback; do
  cp "$ROOT/cmd/$lifecycle" "$WORK/cmd/$lifecycle"
  chmod 755 "$WORK/cmd/$lifecycle"
done
for wizard_file in install upgrade uninstall config; do
  # fnOS 1.2.1102 install-info crashes when wizard/install contains [].
  # Omit unused wizard files instead of declaring an empty wizard.
  if jq -e 'length > 0' "$ROOT/wizard/$wizard_file" >/dev/null; then
    cp "$ROOT/wizard/$wizard_file" "$WORK/wizard/$wizard_file"
  else
    rm -f "$WORK/wizard/$wizard_file"
  fi
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
python3 - "$DIST/fnsdownload-x86_64.fpk" <<'PY'
import json, sys, tarfile
with tarfile.open(sys.argv[1], "r:gz") as archive:
    entries = {entry.name.removeprefix("./"): entry for entry in archive}
    for name in ("main", "install_init", "install_callback", "upgrade_init", "upgrade_callback",
                 "uninstall_init", "uninstall_callback", "config_init", "config_callback"):
        entry = entries.get("cmd/" + name)
        if entry is None or not entry.isfile() or entry.mode & 0o111 != 0o111:
            raise SystemExit("Lifecycle script must be executable in FPK: cmd/" + name)
        content = archive.extractfile(entry).read()
        if not content.startswith(b"#!/") or b"\r" in content:
            raise SystemExit("Lifecycle script requires a shebang and Unix line endings: cmd/" + name)
    for name, entry in entries.items():
        if name.startswith("wizard/") and entry.isfile():
            if not json.load(archive.extractfile(entry)):
                raise SystemExit("Empty wizard must be omitted from FPK: " + name)
print("FPK lifecycle script permissions and line endings passed")
PY
APP_TGZ_ENTRY="$(awk '$0 ~ /app[.]tgz$/ {print; exit}' "$DIST/fpk-files.txt")"
[[ -n "$APP_TGZ_ENTRY" ]] || { echo "FPK is missing app.tgz"; exit 1; }
tar -xOf "$DIST/fnsdownload-x86_64.fpk" "$APP_TGZ_ENTRY" | tar -tzf - > "$DIST/app-files.txt"
for required in "server/fnsdownload-amd64" "www/index.html" "www/assets/app.js" "www/assets/app.css" "ui/config" "ui/images/icon_64.png" "ui/images/icon_256.png"; do
  grep -Eq "(^|\\./)$required$" "$DIST/app-files.txt" || { echo "Missing package entry: $required"; cat "$DIST/app-files.txt"; exit 1; }
done
for required in "manifest" "config/privilege" "config/resource" "cmd/main" "cmd/install_init" "cmd/install_callback" "wizard/uninstall" "app.tgz"; do
  grep -Eq "(^|\\./)$required$" "$DIST/fpk-files.txt" || { echo "Missing outer FPK entry: $required"; cat "$DIST/fpk-files.txt"; exit 1; }
done
if grep -Eq '(^|/)bin/' "$DIST/app-files.txt"; then echo "Unexpected app/bin directory"; exit 1; fi
sha256sum "$DIST/fnsdownload-x86_64.fpk" > "$DIST/SHA256SUMS.txt"
echo "FPK created: $DIST/fnsdownload-x86_64.fpk"
