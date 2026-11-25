# qBittorrent 原生隔离引擎

本应用以专属子进程运行 `server/qbittorrent/qbittorrent-nox`，不连接系统或其他应用已有的 qBittorrent，不安装系统服务，不使用容器。构建目标仅 Linux amd64。源码版本及 SHA256 固定在 `scripts/qbittorrent-sources.json`；构建、依赖、许可证随 FPK 一起打包。

## 功能与使用

新建下载支持磁力链接（批量每行一条）及单个 `.torrent` 文件（最大 8 MB），种子来源可选 NAS 文件或本地上传。NAS 文件通过飞牛文件选择器授权，后端检查应用授权路径和当前用户读取权限。保存目录继续调用飞牛目录选择器。自动模式将磁力/种子交给 qBittorrent，普通 HTTP(S) 交给既有引擎。高级设置可以指定引擎；协议不兼容时拒绝创建，BT 不参与 HTTP 引擎故障切换。

每条磁力链接是独立任务；多文件种子在本版本作为一个任务下载全部文件。每个种子使用独立 `MoxYi-qB-*` 目录。同一信息哈希在本实例只允许一个任务，重复时应继续已有任务。支持统一队列、进度、速度、耗时、暂停、继续、移除记录及明确选择后的文件删除。任务与配置跟随应用保存；关闭浏览器不影响后台任务。

设置页面包含总下载/上传限速（KB/s）、最大连接数、DHT、Peer Exchange。BT 下载期间可能上传，完成后停止做种；本版本不提供长期做种管理。不会把普通 HTTP 链接交给 qBittorrent；HTTP 指向 `.torrent` 的地址当前按普通文件下载，可下载后导入。

## 隔离与恢复

专属状态目录位于 `TRIM_PKGVAR/data/qbittorrent`，含配置、断点和实例锁。使用独立 HOME/XDG/profile、回环 Web API 随机端口、独立 BT 监听端口与随机登录密码。Web API 必须认证，CSRF/Host 检查保留；不在前端暴露 API 地址、密码或会话 Cookie，自动登录使用服务端实际返回的 Cookie 名称。禁用 UPnP 与本地 Peer Discovery。动态库搜索路径只指向本引擎目录。

启动前在实例锁保护下，将本应用专属 profile 的断点记录恢复为暂停，保留元数据及已下载片段，避免重启后越过应用队列自动运行。不启用无法从 Web API 继续的全局会话暂停。任务操作必须存在于应用任务登记表，并使用具体哈希，禁止 `all` 批量通配符。停止只针对当前持有的子进程句柄，父进程退出时发送 SIGTERM；不按名称杀进程、不扫描其他实例、不使用系统服务命令。

CPU、内存、磁盘和带宽仍属于 NAS 共享资源，进程隔离不代表没有资源竞争。

## 验证范围

Go 测试覆盖协议选择、哈希识别、设置持久化、接口权限及任务归属。Linux 集成测试必须使用打包出来的真实 `qbittorrent-nox`，启动两份独立 profile、验证未认证拒绝和错误实例删除被拒绝、种子下载字节完整性、重启与停止不影响另一实例。FPK 构建和本地预览均不等于物理 NAS 安装及下载验收。

官方接口：https://github.com/qbittorrent/qBittorrent/wiki/WebUI-API-(qBittorrent-5.0)
官方源码：https://github.com/qbittorrent/qBittorrent

## BT 排障信息

任务列表和详情展示元数据获取、等待连接、校验阶段和已连接 Peer 数。等待元数据或连接不会被误标为失败。原生 `error`、`missingFiles` 和任务记录缺失分别显示对应提示；这些状态提示不能替代实际日志原因。qBittorrent 自身日志在 `TRIM_PKGVAR/data/qbittorrent/qBittorrent/data/logs/qbittorrent.log`。

