#!/usr/bin/env python3
"""Build private native amd64 qBittorrent from pinned official source archives.
No system install, service registration, containers or pre-existing qB profile.
"""
import concurrent.futures,hashlib,json,os,pathlib,platform,re,shutil,subprocess,sys,tarfile,urllib.request
root=pathlib.Path(__file__).resolve().parents[1]
cache=root/'.cache/qb-build';cache.mkdir(parents=True,exist_ok=True)
prefix=cache/'prefix';runtime=root/'.cache/qbittorrent-runtime'
sources=json.loads((root/'scripts/qbittorrent-sources.json').read_text())
if sys.platform!='linux' or platform.machine()!='x86_64':sys.exit('Native Linux amd64 build host required')
env=dict(os.environ,CC='gcc-12',CXX='g++-12',CFLAGS='-O2 -march=x86-64 -mtune=generic',CXXFLAGS='-O2 -march=x86-64 -mtune=generic')
jobs=str(min(os.cpu_count() or 2,4))
def run(args,cwd=None):subprocess.run([str(x) for x in args],cwd=cwd,env=env,check=True)
def download(item):
 name,meta=item;p=cache/name
 if not p.exists() or hashlib.sha256(p.read_bytes()).hexdigest()!=meta['sha256']:
  with urllib.request.urlopen(meta['url'],timeout=180) as r,p.open('wb') as f:shutil.copyfileobj(r,f)
 if hashlib.sha256(p.read_bytes()).hexdigest()!=meta['sha256']:sys.exit('Source checksum mismatch: '+name)
 return name,p
with concurrent.futures.ThreadPoolExecutor(max_workers=4) as pool:archives=dict(pool.map(download,sources.items()))
def unpack(name):
 dest=cache/name.split('.')[0]
 if not dest.exists():
  dest.mkdir()
  with tarfile.open(archives[name]) as t:
   members=t.getmembers();top=members[0].name.split('/')[0]
   # Official archives can contain symlinks; tarfile's data filter rejects escapes.
   for m in members:
    if m.name==top:continue
    if not m.name.startswith(top+'/'):sys.exit('Unexpected archive root')
    m.name=m.name[len(top)+1:]
    t.extract(m,dest,filter='data')
 return dest
qt=unpack('qtbase.tar.xz');tools=unpack('qttools.tar.xz');boost=unpack('boost.tar.bz2');lt=unpack('libtorrent.tar.gz');qb=unpack('qbittorrent.tar.gz')
if 'try-signal.tar.gz' in archives:
 signal=unpack('try-signal.tar.gz');shutil.copytree(signal,lt/'deps/try_signal',dirs_exist_ok=True)
for patch in ['qtbase-fix1.diff','qtbase-fix2.diff','qtbase-fix3.diff']:
 marker=qt/('.'+patch)
 if not marker.exists():
  check=subprocess.run(['patch','-p1','--dry-run','--forward','-i',str(archives[patch])],cwd=qt,capture_output=True)
  if check.returncode==0:run(['patch','-p1','--forward','-i',archives[patch]],qt)
  else:
   reverse=subprocess.run(['patch','-p1','--dry-run','--reverse','-i',str(archives[patch])],cwd=qt,capture_output=True)
   if reverse.returncode!=0:sys.exit('Security patch neither applies nor is already present: '+patch)
   print('Verified security fix already included upstream:',patch,flush=True)
  marker.touch()
def cmake(source,name,opts):
 build=cache/('build-'+name)
 run(['cmake','-S',source,'-B',build,'-G','Ninja','-DCMAKE_BUILD_TYPE=Release','-DCMAKE_INSTALL_PREFIX='+str(prefix),'-DCMAKE_PREFIX_PATH='+str(prefix),'-DCMAKE_BUILD_RPATH='+str(prefix/'lib'),*opts])
 run(['cmake','--build',build,'--parallel',jobs]);run(['cmake','--install',build])
if not (prefix/'lib/cmake/Qt6Core/Qt6CoreConfig.cmake').exists():
 cmake(qt,'qt',['-DQT_BUILD_TESTS=OFF','-DQT_BUILD_EXAMPLES=OFF','-DFEATURE_gui=OFF','-DFEATURE_widgets=OFF','-DFEATURE_opengl=OFF','-DFEATURE_dbus=OFF','-DFEATURE_icu=OFF','-DFEATURE_x86intrin=OFF','-DFEATURE_openssl_linked=ON','-DFEATURE_system_sqlite=ON','-DFEATURE_glib=OFF'])
if not (prefix/'lib/cmake/Qt6LinguistTools/Qt6LinguistToolsConfig.cmake').exists():
 cmake(tools,'qttools',['-DQT_BUILD_TESTS=OFF','-DQT_BUILD_EXAMPLES=OFF','-DFEATURE_assistant=OFF','-DFEATURE_designer=OFF','-DFEATURE_qdoc=OFF','-DFEATURE_linguist=ON','-DFEATURE_clang=OFF'])
cmake(lt,'libtorrent',['-DBOOST_ROOT='+str(boost),'-DBoost_NO_BOOST_CMAKE=ON','-DBUILD_SHARED_LIBS=ON','-Dbuild_tests=OFF','-Dbuild_examples=OFF','-Dbuild_tools=OFF'])
cmake(qb,'qbittorrent',['-DBOOST_ROOT='+str(boost),'-DBoost_NO_BOOST_CMAKE=ON','-DGUI=OFF','-DDBUS=OFF','-DSYSTEMD=OFF','-DSTACKTRACE=OFF','-DTESTING=OFF','-DCMAKE_INSTALL_RPATH=$ORIGIN/lib'])
runtime.mkdir(exist_ok=True);(runtime/'lib').mkdir(exist_ok=True);binary=prefix/'bin/qbittorrent-nox';shutil.copy2(binary,runtime/'qbittorrent-nox')
ldenv=dict(env,LD_LIBRARY_PATH=str(prefix/'lib'))
out=subprocess.check_output(['ldd',binary],env=ldenv,text=True)
if 'not found' in out:sys.exit(out)
glibc={'libc.so.6','libm.so.6','libpthread.so.0','libdl.so.2','librt.so.1','libresolv.so.2','libutil.so.1'}
files=[]
for name,path in re.findall(r'^\s*(\S+) => (/\S+)',out,re.M):
 if name not in glibc:shutil.copy2(path,runtime/'lib'/name);files.append(pathlib.Path(path))
# SQLite Qt plugin is a runtime dependency, not visible through ldd.
plugins=runtime/'plugins/sqldrivers';plugins.mkdir(parents=True,exist_ok=True)
plugin=prefix/'plugins/sqldrivers/libqsqlite.so'
if plugin.exists():shutil.copy2(plugin,plugins/plugin.name)
(runtime/'qt.conf').write_text('[Paths]\nPrefix=.\nPlugins=plugins\n')
licenses=runtime/'licenses';licenses.mkdir(exist_ok=True)
for source,label in [(qt,'qtbase'),(tools,'qttools'),(lt,'libtorrent'),(qb,'qbittorrent'),(boost,'boost')]:
 for pattern in ['COPYING*','LICENSE*','LICENSES/*']:
  for file in source.glob(pattern):
   if file.is_file():shutil.copy2(file,licenses/(label+'-'+file.name))
for file in files:
 result=subprocess.run(['dpkg-query','-S',str(file)],capture_output=True,text=True)
 for line in result.stdout.splitlines():
  package=line.split(': ',1)[0].split(':')[0];copyright=pathlib.Path('/usr/share/doc')/package/'copyright'
  if copyright.is_file():shutil.copy2(copyright,licenses/(package+'.copyright'))
(runtime/'SOURCE.json').write_text(json.dumps(sources,indent=2)+'\n')
(runtime/'BUILD.txt').write_text('qBittorrent 5.2.4; Qt 6.11.3 + official November security fixes; libtorrent 2.0.11; Boost 1.85.0\nNative amd64, x86-64 baseline, Ubuntu 22.04 glibc build host. No system services installed.\nCorresponding upstream sources and exact SHA256 are in SOURCE.json.\n')
run(['strip',runtime/'qbittorrent-nox'])
print('Private native qBittorrent runtime ready:',runtime)
