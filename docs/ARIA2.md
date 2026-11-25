# 独立 aria2c

aria2c 及其非 glibc 动态依赖打包在本应用 `server/aria2/`，来自构建机 Ubuntu 22.04 签名软件源。包中附版本、文件校验值、版权文件及源码来源。不会安装、替换或升级 NAS 的系统 aria2。

Go 后端只创建并管理自己的子进程，不接受外部 ARIA2_RPC/ARIA2_SECRET 连接配置。RPC 仅监听回环地址，使用空闲端口和每次启动随机生成的密钥；配置文件权限 0600。aria2 的 HOME、XDG 目录、运行目录和动态库路径均独立，不继承其他 aria2 配置或迅雷动态库目录。

应用数据目录下的 aria2 子目录保存配置、会话及实例锁。实例锁防止同一个应用状态目录被两进程同时使用。停止时只操作已持有的子进程句柄，不使用按名称杀进程、端口查杀或系统服务管理命令。父进程退出后，子进程也会退出。

当前接入仅支持 HTTP/HTTPS，关闭 DHT、PEX、LPD、种子和 Metalink 自动跟随。BT 监听端口也独立选取，避免占用其他实例的默认端口。不宣称 BT、磁力功能已接入。

每个任务在用户所选目录内创建 `MoxYi-aria2-*` 独立子目录，隔离同名文件及 `.aria2` 断点文件。仅管理应用登记的带 aria2 前缀的完整 GID；不枚举或导入其他实例任务。删除文件先等待本任务停止，再删除它的文件；不递归删除共享目录。

会话只保存错误及未完成任务，force-save=false；重启后未完成任务保持暂停。已完成记录由应用 SQLite 保存。

隔离测试运行两个真实 aria2c 实例，验证身份、任务和下载目录不同，错用密钥被拒绝，本实例完成下载及删除文件不改变预置文件，关闭后另一个实例仍下载，并验证专属会话重启恢复。

CPU、内存、磁盘和带宽仍为 NAS 共享资源。进程与数据隔离不能保证没有资源竞争。

官方参考：https://aria2.github.io/manual/en/html/aria2c.html

## Application settings and task recovery

The settings page exposes split count, per-server connections (both 1–16), minimum split size (1–1024 MB), private-instance overall download rate, connect/no-data timeout, bounded native retry count and retry interval. Defaults are 8 splits, 8 server connections, 10 MB minimum split, unlimited rate, 30/60 second timeouts and 3 attempts separated by 5 seconds. These settings live in the application's protected aria2/settings.json, separate from runtime RPC credentials. New downloads and resumed paused downloads receive the task options. Changes do not restart active downloads; the overall download cap applies immediately to the private instance. The shared application queue still governs total concurrency; private aria2's internal ceiling is 100 to avoid silently enforcing its default five-job limit beneath that queue. Actual parallel connections depend on server Range support and file size, and increasing them does not guarantee speed.

JSON-RPC error envelopes are decoded even on HTTP 400. Unknown response messages are not reflected into the UI because they can contain credentials or signed URLs; typed internal errors retain method/code/status while user-facing text describes the operation. Repeated pause on an already paused task succeeds without a redundant forcePause call. If an app-owned paused task's GID is missing from the running private process, its cached progress remains visible and continuing rebuilds that exact GID, URL, filename and private directory with continuation enabled and overwriting disabled. No foreign GID is admitted and no other process is contacted. Missing records can also be removed from this app's list. A screenshot showing HTTP 400 alone does not establish which JSON-RPC request failed on the NAS; package and physical-device verification are distinct.

Tests include HTTP-400 JSON error redaction, owned-record reconstruction, option validation/persistence and authorized settings routes. Linux CI exercises live setting changes, confirms another private instance's global options stay unchanged, induces a missing owned GID and verifies the reconstructed download's contents using real aria2c. Reference: https://aria2.github.io/manual/en/html/aria2c.html#rpc-interface .
