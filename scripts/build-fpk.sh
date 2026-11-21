#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
SDK_ROOT="${XUNLEI_SDK_ROOT:-}"
: "${SDK_ROOT:?Set XUNLEI_SDK_ROOT to the installed official Xunlei C++ SDK prefix}"

HEADER="$SDK_ROOT/include/xl_dl/xl_dl_sdk.h"
LIB="$SDK_ROOT/lib/libdk.so"
[[ -f "$HEADER" ]] || { echo "Missing SDK header: $HEADER" >&2; exit 1; }
[[ -f "$LIB" ]] || { echo "Missing SDK library: $LIB" >&2; exit 1; }

mkdir -p "$ROOT/third_party/xunlei/include" "$ROOT/third_party/xunlei/lib" "$ROOT/app/bin" "$ROOT/app/lib"
cp -a "$SDK_ROOT/include/." "$ROOT/third_party/xunlei/include/"
cp -a "$SDK_ROOT/lib/." "$ROOT/third_party/xunlei/lib/"
cp "$LIB" "$ROOT/app/lib/libdk.so"

cd "$ROOT"
CGO_ENABLED=1 GOOS=linux GOARCH=amd64 go build -trimpath -tags xunlei_sdk -ldflags="-s -w" -o app/bin/fnsdownload ./cmd/fnsdownload

if command -v fnpack >/dev/null 2>&1; then
  fnpack build --directory . --nosign
else
  echo "fnpack not found. Go binary and SDK are staged under app/; install official fnpack to package FPK." >&2
  exit 2
fi
