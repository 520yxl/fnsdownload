# 构建与发布

## Docker 镜像
```bash
docker build --platform linux/amd64 -t ghcr.io/520yxl/fnsdownload:dev .
```
GitHub Actions 在 develop/main 分支通过 Node 测试后自动构建 Linux amd64 镜像并推送 GHCR。

## fnOS FPK
本仓库采用飞牛 Docker 应用结构：manifest、cmd、config、wizard、app/docker 和 app/ui。
安装飞牛官方 fnpack 1.2.3 后，在仓库根目录执行：
```bash
fnpack build
```
fnpack 需要官方 CLI 工具。当前 GitHub Actions 尚未安装 fnpack，因此 CI 目前验证 Node 服务和 Docker 镜像，不代表 FPK 已构建成功。

## 数据路径
- 配置和 SDK 状态：${TRIM_PKGVAR}/data -> /data
- 下载文件：${TRIM_APPDEST_VOL}/@appshare/fnsdownload/downloads -> /downloads
- Web 端口：${wizard_port} -> 8088

## 设备验收
安装 FPK、首次初始化、设备注册、Token 获取、SDK 登录、HTTP/HTTPS 测试下载、暂停/继续/删除、应用重启恢复、升级和卸载。
没有通过真实 fnOS x86_64 设备的原生库加载和下载测试，不得标记为正式可用。
