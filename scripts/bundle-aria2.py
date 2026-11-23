#!/usr/bin/env python3
"""Bundle the CI distribution's aria2 and its non-glibc dependencies privately."""
import hashlib
import pathlib
import re
import shutil
import subprocess
import sys

target = pathlib.Path(sys.argv[1]).resolve()
binary = shutil.which('aria2c')
if not binary:
    sys.exit('aria2c is required on the Linux packaging host')
target.mkdir(parents=True, exist_ok=True)
(target / 'lib').mkdir(exist_ok=True)
shutil.copy2(binary, target / 'aria2c')
(target / 'aria2c').chmod(0o755)
output = subprocess.check_output(['ldd', binary], text=True)
if 'not found' in output:
    sys.exit('aria2 dependencies are missing')
glibc = {'libc.so.6', 'libm.so.6', 'libpthread.so.0', 'libdl.so.2', 'librt.so.1', 'libresolv.so.2', 'libutil.so.1'}
files = [pathlib.Path(binary)]
for name, source in re.findall(r'^\s*(\S+) => (/\S+)', output, re.M):
    if name not in glibc:
        shutil.copy2(source, target / 'lib' / name)
        files.append(pathlib.Path(source))
licenses = target / 'licenses'
licenses.mkdir(exist_ok=True)
packages = {'aria2'}
for file in files:
    result = subprocess.run(['dpkg-query', '-S', str(file)], capture_output=True, text=True)
    for line in result.stdout.splitlines():
        packages.add(line.split(': ', 1)[0])
for package in sorted(packages):
    short = package.split(':')[0]
    copyright_file = pathlib.Path('/usr/share/doc') / short / 'copyright'
    if copyright_file.exists():
        shutil.copy2(copyright_file, licenses / (short + '.copyright'))
for license_name in ['GPL-2', 'GPL-3', 'LGPL-2.1', 'LGPL-3']:
    source = pathlib.Path('/usr/share/common-licenses') / license_name
    if source.exists():
        shutil.copy2(source, licenses / license_name)
version = subprocess.check_output([binary, '--version'], text=True).splitlines()[0]
hashes = [hashlib.sha256(p.read_bytes()).hexdigest() + '  ' + str(p.relative_to(target)) for p in sorted(target.rglob('*')) if p.is_file()]
(target / 'SHA256SUMS').write_text('\n'.join(hashes) + '\n')
(target / 'SOURCE.txt').write_text(version + '\nBinary and dependency source: Ubuntu 22.04 signed apt repositories.\nCorresponding sources: https://archive.ubuntu.com/ubuntu/pool/universe/a/aria2/ and each dependency copyright file.\nUpstream: https://github.com/aria2/aria2\n')
print(version)
print('Bundled aria2 privately with', len(files) - 1, 'non-glibc dependencies')
