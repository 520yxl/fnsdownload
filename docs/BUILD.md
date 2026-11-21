# 构建与发布

## Docker 镜像
```bash
docker build --platform linux/amd64 -t ghcr.io/520yxl/fnsdownload:dev .
```

GitHub Actions 在 develop/main 分支通过测试后自动构建 Linux amd64 镜像并推送 GHCR。

## fnOS FPK
本仓库按 fnOS Docker 应用目录布局组织：根目录包含 manifest、cmd（待按目标 fnOS 模板补齐）、config、wizard（待补齐）和 app/docker/docker-compose.yaml。使用飞牛官方 fnpack CLI：
```bash
fnpack build --directory .
```
注意：当前仅完成 Docker 项目配置骨架，尚未完成 fnpack 官方模板校验；不能把 Docker 镜像构建成功等同于 FPK 构建成功。

持久化目录：
- 配置与 SDK 状态：${TRIM_PKGVAR}/data -> /data
- 用户下载目录：${TRIM_APPDEST_VOL}/@appshare/fnsdownload/downloads -> /downloads
- Web 端口：${TRIM_SERVICE_PORT} -> 8088

## 设备验收
安装 FPK、首次初始化、设备注册、Token 获取、SDK 登录、HTTP/HTTPS 测试下载、暂停/继续/删除、应用重启恢复、升级和卸载。
没有通过真实 fnOS x86_64 设备的原生库加载和下载测试，不得标记为正式可用。
