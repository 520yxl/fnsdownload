#!/usr/bin/env python3
"""Reject loader-enforced x86-64-v2/v3/v4 requirements; do not strip ELF notes."""
import subprocess
import sys

for path in sys.argv[1:]:
    notes = subprocess.check_output(["readelf", "-n", path], text=True)
    for line in notes.splitlines():
        if "ISA needed:" in line and any(level in line for level in ["x86-64-v2", "x86-64-v3", "x86-64-v4"]):
            sys.exit(f"Non-baseline CPU requirement in {path}: {line.strip()}")
    print(f"Baseline x86-64 ELF requirements verified: {path}")
