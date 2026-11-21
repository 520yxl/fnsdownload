# MoxYi Download（飞牛下载器）

基于迅雷官方 Download SDK + Thunder_auth 授权中心的 fnOS 下载应用。

## 当前开发目标
- fnOS FPK（Docker 应用），首发 Linux x86_64。
- 迅雷官方 Node.js SDK 1.0.3。
- Thunder_auth 产品认证：moxyi-download + HMAC-SHA256。
- 下载任务：新增、开始、暂停、继续、删除、进度查询。
- 数据持久化：应用配置、SDK 配置、任务数据保存在 /data。
- 下载文件保存在 NAS 下载目录并映射到容器 /downloads。

## 官方 SDK 兼容边界
迅雷官方 Node.js SDK 当前文档列出 Linux x64，不包含 Linux ARM64。当前构建仅支持 x86_64。官方 SDK 当前只开放 HTTP(S) 下载；BT、磁力链、ed2k 不在本版本支持范围内。

## 本地开发
```bash
npm install
npm test
npm start
```
运行环境变量：PORT=8088、DATA_DIR=./data、DOWNLOAD_DIR=./downloads。

首次打开 Web UI，设置本地管理密码和 Thunder_auth 地址、产品 ID、产品 Secret、迅雷 App ID。产品 Secret 只保存在 NAS 本地数据目录。

## Docker 构建
```bash
docker build --platform linux/amd64 -t ghcr.io/520yxl/fnsdownload:dev .
docker run --rm -p 8088:8088 -v ./data:/data -v ./downloads:/downloads ghcr.io/520yxl/fnsdownload:dev
```

## 仓库目录
- AGENTS.md：AI 开发规范
- src/：服务端、授权客户端、SDK 适配器
- public/：Web UI
- app/：fnOS Docker FPK 配置
- docs/：架构、授权、SDK、构建和测试文档
- .github/workflows/：CI 与镜像构建

## 状态
早期工程骨架。GitHub CI、Docker 镜像构建、FPK 实机安装和真实迅雷 SDK 下载分别验收，不混淆状态。