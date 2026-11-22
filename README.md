# MoxYi Download（飞牛原生下载器）

基于 Go + 迅雷官方 Linux x64 SDK + Thunder_auth 的 fnOS 原生桌面应用。

- **仅支持 x86_64 / amd64**，不构建 ARM64 版本。
- 不使用 Docker、Compose 或容器镜像。
- Go 实现 HTTP API、授权、任务管理和后台服务。
- Vue 3 静态前端嵌入 Go 服务，由飞牛桌面入口打开。
- 迅雷官方 Linux x64 SDK 通过 CGO 调用。
- FPK 内置 Go 二进制和 SDK 动态库，不依赖 NAS 预装 Go。
- 配置位于 `TRIM_PKGVAR/data`，下载文件位于应用共享目录。

## 本地开发
```bash
go test ./...
go vet ./...
GOOS=linux GOARCH=amd64 go build -o build/fnsdownload ./cmd/fnsdownload
```

启用迅雷 SDK 的构建需要官方 Linux x64 SDK 头文件和动态库，使用 `CGO_ENABLED=1 GOOS=linux GOARCH=amd64 -tags xunlei_sdk` 编译。

## 授权
Go 客户端按 Thunder_auth API v1 的 HMAC-SHA256 规范签名请求。产品 ID 为 `moxyi-download`，登录 Token 由授权中心在线下发，Scope 由服务端配置为 `xtask.p2sp` 与 `xindex.query`。严禁在 NAS 端保存迅雷官方 API Key。

## 原生 FPK
按照 FnDepot 参考包的原生 FPK 结构打包。生命周期脚本直接启动/停止 Go 服务。禁止 Docker、docker-project、Compose 和容器镜像。

## 验收
CI 编译通过不代表真实 SDK 可用。正式发布前必须在 x86_64 飞牛 NAS 验证安装、桌面入口、服务启动、授权、SDK 登录、HTTP(S) 下载、任务暂停/恢复、升级和卸载。
