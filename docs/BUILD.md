# 构建与发布

## Docker 镜像
```bash
docker build --platform linux/amd64 -t ghcr.io/520yxl/fnsdownload:dev .
```
GitHub Actions 在 develop/main 分支通过 Node 测试后自动构建 Linux amd64 镜像并推送 GHCR。

## fnOS FPK
本仓库使用飞牛 Docker 应用结构：manifest、cmd、config、wizard、app/docker 和 app/ui。
本地使用飞牛官方 fnpack 1.3.3：
```bash
fnpack build
```
需要导入开发者证书才能生成签名包。GitHub Actions 使用 `fnpack build --nosign` 生成未签名开发测试包，并作为 Workflow Artifact 保存 14 天。该包仅供本地调试，不等同于可正式分发的签名包。

## 数据路径
- 配置和 SDK 状态：${TRIM_PKGVAR}/data -> /data
- 下载文件：${TRIM_APPDEST_VOL}/@appshare/fnsdownload/downloads -> /downloads
- Web 端口：${wizard_port} -> 8088

## 设备验收
先安装未签名开发包验证 FPK 结构；正式签名安装需要导入有效开发者证书。
随后验证首次初始化、设备注册、Token 获取、SDK 登录、HTTP/HTTPS 测试下载、暂停/继续/删除、应用重启恢复、升级和卸载。
没有通过真实 fnOS x86_64 设备的原生库加载和下载测试，不得标记为正式可用。
