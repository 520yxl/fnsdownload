# 构建与发布

## Docker 镜像
```bash
docker build --platform linux/amd64 -t ghcr.io/520yxl/fnsdownload:dev .
```

## fnOS FPK
当前工程使用 Docker FPK 目录布局。使用飞牛官方 fnpack CLI 构建：
```bash
cd app
fnpack build
```
实际 fnpack 版本、manifest 和向导字段应以目标 fnOS 版本官方模板为准。生成的 .fpk 不提交 Git。

## CI
- PR：Node 语法检查、单元测试。
- develop/main：构建 Linux amd64 Docker 镜像并推送 GHCR。
- Release tag：生成版本化镜像；FPK 打包须在已安装 fnpack 的构建环境执行。

## 设备验收
安装 FPK、首次初始化、设备注册、Token 获取、SDK 登录、HTTP/HTTPS 测试下载、暂停/继续/删除、应用重启恢复、升级和卸载。
没有通过真实 fnOS x86_64 设备的原生库加载和下载测试，不得标记为正式可用。
