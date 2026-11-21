# MoxYi Download Go 原生架构

## 语言与边界
- Go：HTTP API、业务逻辑、任务调度、授权客户端、配置/数据库、日志和生命周期。
- Vue 3：桌面 Web UI，构建产物嵌入 Go binary（go:embed）。
- 迅雷 SDK：官方 Linux x64 C++ SDK 的 C ABI；Go 通过 CGO 调用官方导出的 `xl_dl_*` C 接口。
- 不运行 Node.js，不使用 Docker，不使用 Docker Compose，不部署容器。

## 为什么 CGO
迅雷官方 C++ SDK 的 `xl_dl_sdk.h` 对外导出 `extern "C"` 风格的 C ABI，包括 init/login/create/start/stop/delete/state/uninit。Go CGO 可直接调用这些函数；C++ 只作为官方 SDK 的实现依赖，不另起 C++ 后台服务。

## 授权流程
1. 安装时生成并持久化稳定 device_id。
2. Go 端按照 Thunder_auth API v1 规范计算 HMAC-SHA256 请求签名。
3. 调用 `POST /api/v1/auth/token`，请求 Scope 固定为 `xtask.p2sp` 与 `xindex.query` 的共享 Token 策略由服务端签发。
4. Go 端将 Token 仅保存在进程内存，调用 `xl_dl_login(token)`。
5. 按 expires_at / refresh_after 维护刷新任务；Token 不写日志、不写配置文件、不通过管理 API 回显。

## 原生 FPK
- FPK 内含 Go 单文件服务端、前端静态资源、迅雷官方 `libdk.so`、SDK headers（仅构建时）及必要运行库。
- 运行时不依赖 NAS 安装 Go/Node/Python，也不依赖 Docker。
- 生命周期脚本通过 PID 文件直接启动/停止 Go 服务。
- 配置、SQLite、SDK 状态和日志存放于 `TRIM_PKGVAR`；下载文件存放于应用共享目录。

## 首发平台
Linux x86_64。官方 SDK 上游列出 Linux x64；未验证的 ARM64 不在首发承诺范围。
