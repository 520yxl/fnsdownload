# MoxYi Download 原生 FPK 构建

目标仅为 Linux x86_64/amd64。禁止 Docker、Compose、ARM64 产物。

## 包内结构
FPK 根目录包含 manifest、config、cmd、wizard、i18n、图标和 app.tgz。app.tgz 内必须有：
- server/fnsdownload-amd64
- ui/index.html
- ui/config
- ui/assets/app.css、ui/assets/app.js
- ui/images/icon_0.png、ui/images/icon_1.png

Go 后端监听 `${TRIM_APPDEST}/target/app.sock`，由 fnOS 统一网关以 /app/fnsdownload 转发。

## 本地构建
```bash
go test ./...
go vet ./...
./scripts/build-fpk.sh
```

正式迅雷 SDK 构建需要迅雷官方 Linux x64 SDK，头文件和动态库必须真实存在；没有 SDK 时只能生成 UI/网关开发测试包，不能标记为正式下载器。
