# MoxYi Download Go 原生架构

## 语言与边界
- Go：HTTP API、业务逻辑、任务调度、授权客户端、配置/数据库、日志和生命周期。
- Vue 3：桌面 Web UI，静态资源嵌入 Go binary。
- 迅雷 SDK：官方 Linux x64 C++ SDK 的 C ABI；Go 通过 CGO 调用。
- 目标架构仅为 Linux x86_64 / amd64，不提供 ARM64 版本。
- 不运行 Node.js，不使用 Docker，不部署容器。

## FPK 结构
参考用户提供的 FnDepot FPK：
- 根目录包含 manifest、config、cmd、wizard、i18n、ICON.PNG、ICON_256.PNG。
- app.tgz 包含 server/fnsdownload-amd64、server 所需 SDK 动态库，以及 ui/ 前端静态资源。
- cmd/common 和生命周期脚本负责架构检查、启动、停止、状态、安装、升级和卸载。
- 启动脚本只接受 amd64 主机；其他架构明确报错退出。

## 迅雷 SDK
官方 Linux x64 SDK 通过 CGO 直接集成到 Go 服务。C++ SDK 作为动态库依赖，不另起 C++ 后台服务。正式构建必须使用真实官方 SDK 头文件和动态库，禁止以 stub 构建产物冒充可下载版本。

## 授权流程
1. 安装时生成并持久化稳定 device_id。
2. Go 端按照 Thunder_auth API v1 规范计算 HMAC-SHA256 请求签名。
3. 调用授权中心 Token 签发接口，使用 shared Login Token 策略，Scope 为 `xtask.p2sp` 与 `xindex.query`。
4. Go 端将 Token 仅保存在进程内存，调用迅雷 SDK 登录。
5. 按 expires_at / refresh_after 维护刷新任务；Token 不写日志、不写配置文件、不通过管理 API 回显。

## 运行环境
FPK 内含 Go 可执行文件、前端资源和所需 SDK 动态库。NAS 不需要预装 Go、Node.js、Python 或 Docker。配置、SQLite、SDK 状态和日志存放于 TRIM_PKGVAR；下载文件存放于应用共享目录。

## 平台范围
仅支持飞牛 fnOS x86_64（amd64）。不构建、不打包、不宣传 ARM64 支持。
