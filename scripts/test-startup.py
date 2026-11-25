#!/usr/bin/env python3
"""Exercise actual startup failure reporting without a NAS or vendor login."""
import os
import pathlib
import subprocess
import tempfile

ROOT = pathlib.Path(__file__).resolve().parent.parent
(ROOT / ".cache").mkdir(exist_ok=True)
with tempfile.TemporaryDirectory(prefix="fnsdownload-startup-", dir=ROOT / ".cache") as temporary:
    base = pathlib.Path(temporary)
    target, state = base / "target", base / "state"
    (target / "server/lib").mkdir(parents=True)
    (target / "www").mkdir()
    (target / "www/index.html").write_text("test")
    (state / "logs").mkdir(parents=True)
    (state / "logs/app.log").write_text("OLD_FAILURE_MUST_NOT_APPEAR\n")
    binary = target / "server/fnsdownload-amd64"
    binary.write_text('''#!/usr/bin/env bash
if [[ "$LD_LIBRARY_PATH" != "$TRIM_APPDEST/server/lib"* ]]; then
  echo "private library path not configured" >&2
  exit 1
fi
echo "CURRENT_STARTUP_FAILURE" >&2
exit 17
''', newline='\n')
    binary.chmod(0o755)
    report = base / "startup-error.txt"
    env = dict(os.environ, TRIM_APPDEST=target.as_posix(), TRIM_PKGVAR=state.as_posix(), TRIM_TEMP_LOGFILE=report.as_posix())
    result = subprocess.run([os.environ.get("BASH_BIN", "bash"), (ROOT / "cmd/main").as_posix(), "start"], env=env, capture_output=True, text=True, encoding="utf-8", errors="replace", timeout=20)
    assert result.returncode != 0, "failed service was reported as started"
    assert report.exists(), (result.stdout, result.stderr)
    message = report.read_text()
    assert "CURRENT_STARTUP_FAILURE" in message, message
    assert "OLD_FAILURE_MUST_NOT_APPEAR" not in message, message
    assert not (state / "fnsdownload.pid").exists(), "stale PID retained"
print("Startup failure details and private library path verified")
