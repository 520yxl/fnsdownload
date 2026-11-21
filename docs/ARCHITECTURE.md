# 架构

- fnOS FPK：安装、升级和应用中心入口。
- Docker 容器：Node.js 22 + 迅雷官方 Node.js SDK。
- Web UI：下载任务、授权状态、设置。
- HTTP API：本机 UI 与 SDK Worker 的控制面。
- ThunderAuthClient：按 Thunder_auth v1 规范生成 HMAC，注册设备并请求共享 Login Token。
- XLDownloadAPI：唯一 SDK 适配层。
- 持久化：/data/config.json、/data/tasks.json、/data/sdk。
- 下载目录：/downloads，映射到用户选择的 NAS 共享文件夹。

## 数据流
1. 首次打开页面，设置本地密码、Thunder_auth URL、Product ID/Secret、迅雷 App ID。
2. 点击连接迅雷服务。
3. 使用稳定 device_id 调用 POST /api/v1/auth/register。
4. 调用 POST /api/v1/auth/token 获取共享 JWT；不持有迅雷 API Key。
5. 调用 SDK init，再调用 login(loginToken)。
6. 下载请求调用 createP2spTask、startTask；状态由 getTaskState 查询。

## 安全边界
- 产品 Secret 保存在 NAS /data/config.json，文件权限 0600。
- 本地管理密码保存 SHA-256 摘要；Session 存进程内存，重启后失效。
- Token 不写入磁盘或日志。
- 当前管理 UI 应仅在可信 LAN 访问，不要直接暴露公网。
- HMAC Secret 可由 NAS 管理员提取，不是硬件绑定凭据。

## 当前限制
- 官方 Node.js SDK 文档当前支持 Linux x64，不包含 Linux ARM64。
- 官方 SDK 当前只开放 HTTP(S) 协议下载；BT、磁力链、ed2k 不支持。
- 原生库须在真实 fnOS x86_64 设备验证动态库依赖、重启恢复和并发行为。
