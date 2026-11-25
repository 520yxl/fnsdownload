#!/usr/bin/env python3
"""Stage the official Linux x64 SDK. Fail closed if the upstream archive changes."""
import hashlib
import pathlib
import sys
import urllib.request
import zipfile

ROOT = pathlib.Path(__file__).resolve().parent.parent
URL = "https://down.sandai.net/opensdk/download/xunlei-dlsdk-cpp.zip"
SHA256 = "718923688ac3717f8edd316f46767ed61a7b82a912edfb6143233a29e34b9819"
archive = ROOT / ".cache/xunlei-dlsdk-cpp.zip"
archive.parent.mkdir(parents=True, exist_ok=True)
if not archive.exists() or hashlib.sha256(archive.read_bytes()).hexdigest() != SHA256:
    with urllib.request.urlopen(URL, timeout=120) as response:
        payload = response.read()
    if hashlib.sha256(payload).hexdigest() != SHA256:
        sys.exit("Official SDK archive changed: review the new version before updating the pinned hash")
    archive.write_bytes(payload)
destination = ROOT / "third_party/xunlei"
with zipfile.ZipFile(archive) as sdk:
    for entry in sdk.infolist():
        name = entry.filename
        prefix = "xunlei-dlsdk-cpp/"
        if not name.startswith(prefix) or entry.is_dir():
            continue
        relative = name[len(prefix):]
        if relative.startswith("include/xl_dl/"):
            output = destination / relative
        elif relative == "prebuilt/linux-x64/lib/libdk.so":
            output = destination / "lib/libdk.so"
        else:
            continue
        if not output.resolve().is_relative_to(destination.resolve()):
            sys.exit("Unsafe SDK archive path")
        output.parent.mkdir(parents=True, exist_ok=True)
        output.write_bytes(sdk.read(entry))
print(f"Official Linux x64 SDK staged: {destination}")
print(f"Archive SHA256: {SHA256}")
