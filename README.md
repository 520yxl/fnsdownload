# MoxYi Download（飞牛原生下载器）

基于 Go + 迅雷官方 Linux x64 SDK + Thunder_auth 的 fnOS 原生桌面应用。

- 仅支持 x86_64 / amd64，Manifest 平台值为 x86。
- 原生 FPK，不使用 Docker、Compose 或容器镜像。
- Go 后端与静态 Web UI 一同打包进 FPK。
- 使用 fnOS 统一网关，服务监听 `${TRIM_APPDEST}/app.sock`。
- 配置与数据库位于 `${TRIM_PKGVAR}`，下载目录由 `config/resource` 声明并通过 `TRIM_DATA_SHARE_PATHS` 获取。
- 不要求 NAS 预装 Go、Node.js 或 Python。

## 开发
```bash
go test ./...
go vet ./...
bash -n cmd/*
```

## fnOS 约定
Manifest、桌面入口、权限、共享目录、生命周期和打包结构必须与飞牛官方开发文档一致。参阅 [官方统一网关文档](https://developer.fnnas.com/docs/core-concepts/gateway-registration/)。

## 迅雷 SDK
正式 SDK 构建需要官方 Linux x64 SDK 头文件和动态库。没有真实 SDK 的 CI 构建只能用于桌面入口、UI 和网关联调，不代表已具备迅雷下载能力。

## Thunder_auth
产品 ID：`moxyi-download`；认证：HMAC-SHA256；Token 模式：shared；Scope 由授权中心下发为 `xtask.p2sp` 与 `xindex.query`。迅雷官方 API Key 不得保存在 NAS 客户端。
