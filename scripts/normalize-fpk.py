#!/usr/bin/env python3
"""Normalize fnpack's unsigned archive to the known-installable fnOS 0.3.2 layout.

fnpack 1.3.3 can accidentally include the package-level config directory in
app.tgz. The fnOS 0.3.2 reference FPK has only server/ and ui/ inside app.tgz.
Update the app payload MD5 in manifest after normalization.
"""
import argparse
import gzip
import hashlib
import io
import os
import re
import tarfile
import tempfile
from pathlib import Path

def archive_bytes(entries):
    out = io.BytesIO()
    with gzip.GzipFile(fileobj=out, mode="wb", mtime=0) as gz:
        with tarfile.open(fileobj=gz, mode="w", format=tarfile.GNU_FORMAT) as tar:
            for info, data in entries:
                info = tarfile.TarInfo(name=info.name)
                info.mode = data[1]
                info.uid = info.gid = 0
                info.uname = info.gname = ""
                info.type = data[2]
                info.linkname = data[3]
                info.mtime = 0
                info.size = len(data[0]) if info.isfile() else 0
                tar.addfile(info, io.BytesIO(data[0]) if info.isfile() else None)
    return out.getvalue()

def entries_from(data):
    result = []
    with tarfile.open(fileobj=io.BytesIO(data), mode="r:gz") as tf:
        for info in tf:
            content = tf.extractfile(info).read() if info.isfile() else b""
            result.append((info, (content, info.mode, info.type, info.linkname)))
    return result

def normalize(path):
    outer = entries_from(path.read_bytes())
    app_item = next((x for x in outer if x[0].name == "app.tgz"), None)
    if not app_item:
        raise RuntimeError("FPK missing app.tgz")
    app = entries_from(app_item[1][0])
    filtered = [(i, data) for i, data in app if i.name.lstrip("./").split("/")[0] in ("server", "ui")]
    if not filtered:
        raise RuntimeError("app.tgz has no server/ or ui/ files")
    required = {"server/fnsdownload-amd64", "ui/config", "ui/index.html"}
    present = {i.name.lstrip("./") for i, _ in filtered}
    if not required.issubset(present):
        raise RuntimeError("Missing app files: " + str(required - present))
    app_bytes = archive_bytes(filtered)
    checksum = hashlib.md5(app_bytes).hexdigest()
    normalized = []
    for info, data in outer:
        if info.name == "app.tgz":
            data = (app_bytes, data[1], data[2], data[3])
        elif info.name == "manifest":
            text = data[0].decode("utf-8")
            if not re.search(r"(?m)^checksum\s*=", text):
                raise RuntimeError("manifest missing checksum")
            text = re.sub(r"(?m)^(checksum\s*=\s*)[^\r\n]+", lambda m: m.group(1) + checksum, text)
            data = (text.encode(), data[1], data[2], data[3])
        normalized.append((info, data))
    result = archive_bytes(normalized)
    # Verify checksum and content from the final bytes, not the source.
    final = entries_from(result)
    payload = next(data[0] for info, data in final if info.name == "app.tgz")
    manifest = next(data[0].decode() for info, data in final if info.name == "manifest")
    assert hashlib.md5(payload).hexdigest() == checksum
    assert re.search(r"(?m)^checksum\s*=\s*" + checksum + r"$", manifest)
    assert not any(i.name.lstrip("./").startswith("config/") for i, _ in entries_from(payload))
    tmp = path.with_suffix(path.suffix + ".tmp")
    tmp.write_bytes(result)
    os.replace(tmp, path)
    print(f"Normalized FPK: {path}, app checksum={checksum}")

if __name__ == "__main__":
    parser = argparse.ArgumentParser()
    parser.add_argument("fpk", type=Path)
    normalize(parser.parse_args().fpk)
