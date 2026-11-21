#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
DIST="$ROOT/dist"
WORK="$ROOT/.fpk-work"
SDK_ROOT="${XUNLEI_SDK_ROOT:-}"
command -v go >/dev/null 2>&1 || { echo "Go is required on the build host" >&2; exit 1; }
command -v fnpack >/dev/null 2>&1 || { echo "Official fnpack is required on the build host" >&2; exit 1; }
cd "$ROOT"
for script in cmd/* scripts/build-fpk.sh; do [[ -f "$script" ]] && bash -n "$script"; done
python3 - <<'PY'
import json
from pathlib import Path
manifest = {}
for line in Path("manifest").read_text().splitlines():
    line = line.strip()
    if not line or line.startswith("#"):
        continue
    key, value = line.split("=", 1)
    manifest[key.strip()] = value.strip()
entry = json.loads(Path("app/ui/config").read_text())[".url"]
json.loads(Path("config/privilege").read_text())
json.loads(Path("config/resource").read_text())
assert manifest["platform"] == "x86", manifest
assert manifest["desktop_uidir"] == "ui", manifest
assert manifest["desktop_applaunchname"] in entry, manifest
assert manifest["desktop_applaunchname"] == "fnsdownload.Application", manifest
desktop = entry[manifest["desktop_applaunchname"]]
assert desktop["gatewayPrefix"] == "/app/fnsdownload", desktop
assert desktop["gatewaySocket"] == "app.sock", desktop
assert desktop["url"] == "/app/fnsdownload", desktop
assert "service_port" not in manifest, manifest
print("fnOS manifest, desktop entry, privilege and resource validation passed")
PY
go test ./...
go vet ./...
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
rm -rf "$WORK/cmd/fnsdownload"
cp -a "$ROOT/wizard/." "$WORK/wizard/"
cp -a "$ROOT/i18n/." "$WORK/i18n/"
cp "$ROOT/ICON.PNG" "$WORK/ICON.PNG"
cp "$ROOT/ICON_256.PNG" "$WORK/ICON_256.PNG"
# fnOS packages should extract app payload as root:root, not the CI runner UID.
if [[ "$(id -u)" -eq 0 ]]; then chown -R 0:0 "$WORK/app" "$WORK/config" "$WORK/cmd" "$WORK/wizard" "$WORK/i18n" "$WORK/manifest" "$WORK/ICON.PNG" "$WORK/ICON_256.PNG"; elif command -v sudo >/dev/null 2>&1; then sudo -n chown -R 0:0 "$WORK/app" "$WORK/config" "$WORK/cmd" "$WORK/wizard" "$WORK/i18n" "$WORK/manifest" "$WORK/ICON.PNG" "$WORK/ICON_256.PNG"; else echo "Need root ownership for FPK payload; run as root or install sudo." >&2; exit 1; fi
(
  cd "$WORK"
  if [[ "$(id -u)" -eq 0 ]]; then
    fnpack build --directory . --nosign
  else
    sudo -n fnpack build --directory . --nosign
  fi
)
FPK="$(find "$WORK" -type f -name "*.fpk" -print -quit)"
if [[ -z "$FPK" ]]; then FPK="$(find "$ROOT" -maxdepth 1 -type f -name "*.fpk" -print -quit)"; fi
[[ -n "$FPK" ]] || { echo "fnpack reported success but no FPK file was found" >&2; find "$WORK" -maxdepth 3 -type f -printf "%p (%s bytes)\n"; exit 1; }
cp "$FPK" "$DIST/fnsdownload-x86_64.fpk"
# Keep fnpack's original output intact; do not rewrite its tar metadata.
file "$DIST/fnsdownload-x86_64.fpk"
tar -tzf "$DIST/fnsdownload-x86_64.fpk" > "$DIST/fpk-files.txt"
APP_TGZ_ENTRY="$(awk '$0 ~ /app[.]tgz$/ {print; exit}' "$DIST/fpk-files.txt")"
[[ -n "$APP_TGZ_ENTRY" ]] || { echo "FPK is missing app.tgz" >&2; cat "$DIST/fpk-files.txt"; exit 1; }
tar -xOf "$DIST/fnsdownload-x86_64.fpk" "$APP_TGZ_ENTRY" | tar -tzf - > "$DIST/app-files.txt"
python3 - "$DIST/app-files.txt" <<'PY'
from pathlib import Path
names = {line.strip().removeprefix("./").rstrip("/") for line in Path(__import__("sys").argv[1]).read_text().splitlines()}
required = {"ui/index.html", "ui/config", "server/fnsdownload-amd64", "ui/assets/app.css", "ui/assets/app.js", "ui/images/icon_0.png", "ui/images/icon_1.png"}
missing = required - names
if missing:
    raise SystemExit(f"Missing package entries: {sorted(missing)}")
print("Runtime UI and Go binary entries present")
PY
python3 - "$DIST/fnsdownload-x86_64.fpk" <<'PY'
import io
import sys
import tarfile

with tarfile.open(sys.argv[1], "r:gz") as outer:
    files = {member.name.removeprefix("./"): member for member in outer.getmembers()}
    required_outer = {"manifest", "app.tgz", "config/privilege", "config/resource", "cmd/main"}
    missing = required_outer - set(files)
    if missing:
        raise SystemExit(f"Missing FPK outer files: {sorted(missing)}")
    payload = outer.extractfile(files["app.tgz"]).read()
    with tarfile.open(fileobj=io.BytesIO(payload), mode="r:gz") as inner:
        names = {m.name.removeprefix("./") for m in inner.getmembers()}
        expected = {"server/fnsdownload-amd64", "ui/index.html", "ui/config"}
        if not expected.issubset(names):
            raise SystemExit(f"Missing runtime files: {sorted(expected - names)}")
        forbidden = {"manifest", "config/privilege", "config/resource", "cmd/main"}
        duplicated = forbidden & names
        if duplicated:
            raise SystemExit(f"FPK metadata wrongly duplicated inside app.tgz: {sorted(duplicated)}")
        print("fnOS FPK outer metadata and app.tgz runtime payload separation passed")
PY
echo "FPK created: $DIST/fnsdownload-x86_64.fpk"
echo "Build details: $DIST/build-info.txt"
