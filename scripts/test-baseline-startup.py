#!/usr/bin/env python3
"""Start the actual service on QEMU's baseline x86 CPU and query its gateway."""
import http.client
import json
import os
import pathlib
import socket
import subprocess
import sys
import tempfile
import time

binary = str(pathlib.Path(sys.argv[1]).resolve())
with tempfile.TemporaryDirectory(prefix="fns-baseline-") as temporary:
    root = pathlib.Path(temporary)
    env = dict(os.environ, TRIM_APPDEST=temporary, TRIM_PKGVAR=temporary,
               DATA_DIR=str(root / "data"), DOWNLOAD_DIR=str(root / "downloads"))
    with (root / "runtime.log").open("w+") as log:
        process = subprocess.Popen(["qemu-x86_64", "-cpu", "qemu64", binary], env=env, stdout=log, stderr=log)
        try:
            for _ in range(100):
                if process.poll() is not None:
                    log.seek(0)
                    raise RuntimeError("Baseline startup failed: " + log.read())
                if (root / "app.sock").exists():
                    break
                time.sleep(0.1)
            connection = http.client.HTTPConnection("localhost", timeout=5)
            connection.sock = socket.socket(socket.AF_UNIX, socket.SOCK_STREAM)
            connection.sock.settimeout(5)
            connection.sock.connect(str(root / "app.sock"))
            connection.request("GET", "/app/fnsdownload/api/health")
            response = connection.getresponse()
            result = json.loads(response.read())
            assert response.status == 200 and result.get("ok"), result
            connection.close()
            print("Actual gateway service starts on baseline qemu64 CPU")
        finally:
            process.terminate()
            try:
                process.wait(timeout=10)
            except subprocess.TimeoutExpired:
                process.kill()
                process.wait()
