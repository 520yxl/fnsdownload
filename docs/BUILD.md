# MoxYi Download 原生 FPK 构建

目标仅为 Linux x86_64/amd64。禁止 Docker、Compose、ARM64 产物。

## 包内结构
FPK 根目录包含 manifest、config、cmd、wizard、i18n、图标和 app.tgz。app.tgz 内必须有：
- server/fnsdownload-amd64
- www/index.html
- ui/config
- www/assets/app.css、www/assets/app.js
- ui/images/icon_64.png、ui/images/icon_256.png

Go 后端监听 `${TRIM_APPDEST}/app.sock`（安装后通常为 `/var/apps/fnsdownload/target/app.sock`），由 fnOS 统一网关以 /app/fnsdownload 转发。

## 本地构建
```bash
go test ./...
go vet ./...
./scripts/build-fpk.sh
```

构建时不打包内容为 `[]` 的空向导文件；在 fnOS 1.2.1102 上，空安装向导会导致安装信息接口返回 10111。最终包会检查所有生命周期脚本的执行权限、Unix 换行和向导非空性。

正式迅雷 SDK 构建需要迅雷官方 Linux x64 SDK，头文件和动态库必须真实存在；没有 SDK 时只能生成 UI/网关开发测试包，不能标记为正式下载器。
