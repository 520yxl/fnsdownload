# MoxYi Download（飞牛原生下载器）

基于 Go + 迅雷官方 Download SDK + Thunder_auth 的 fnOS 原生桌面应用。

- 不使用 Docker / Compose / 容器镜像。
- Go 实现 HTTP API、授权、任务管理和后台服务。
- Vue 3 静态前端嵌入 Go 服务，飞牛桌面 iframe 打开。
- Go CGO 直接调用迅雷官方 Linux x64 SDK 导出的 C ABI。
- FPK 内置 Go 二进制和迅雷 SDK 动态库，不依赖 NAS 预装 Go。
- 配置位于 `TRIM_PKGVAR/data`，下载文件位于应用共享目录。
- 首发平台 Linux x86_64；ARM64 需等待迅雷官方 SDK 对应架构支持及实机验证。

## 本地开发
```bash
go test ./...
go build -o build/fnsdownload ./cmd/fnsdownload
```

启用迅雷 SDK 的构建需准备官方 C++ SDK Linux x64 发行包，并使用 `CGO_ENABLED=1 -tags xunlei_sdk` 编译。开发环境默认使用 SDK stub，便于不依赖迅雷动态库执行单元测试。

## 授权
Go 客户端按 Thunder_auth API v1 的 HMAC-SHA256 规范签名请求。产品 ID 为 `moxyi-download`，登录 Token 由授权中心下发，Scope 由服务端配置为 `xtask.p2sp` 与 `xindex.query`。严禁在 NAS 端保存迅雷官方 API Key 或调用本地 API Key 换取 Token。

## 原生 FPK
通过飞牛 fnpack 构建原生 FPK。生命周期脚本直接启动/停止 Go 服务。禁止 docker-project、Compose、Dockerfile 和容器镜像。

## 验收
CI 单元测试通过不代表真实 SDK 可用。正式发布前必须在 x86_64 飞牛 NAS 验证安装、桌面入口、服务启动、授权、SDK 登录、HTTP(S) 下载、任务暂停/恢复、升级和卸载。
