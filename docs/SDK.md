# 迅雷 SDK 接入边界

官方指南：https://open.xunlei.com/doc/download/sdk/nodejs

## 官方支持平台
当前 Node.js SDK 文档列出 Windows x64/x86、macOS Universal、Linux x64，没有列出 Linux ARM64。首版仅构建 amd64/x86_64。

## npm 包
- @xunlei-open/dlsdk@1.0.3
- @xunlei-open/dlsdk-native-linux-x64@1.0.3

## 调用顺序
```js
const sdk = new XLDownloadAPI();
sdk.init({ appId, appVersion, configPath, saveTasks: true });
sdk.login(loginToken);
const task = sdk.createP2spTask({ url, savePath, saveName });
sdk.startTask(task.taskId);
const state = sdk.getTaskState(task.taskId);
sdk.uninit();
```
生产环境 Token 必须来自 Thunder_auth，不允许在客户端调用 sdk.getLoginToken(apiKey)。

## 首版支持范围
- HTTP / HTTPS P2SP 下载。
- 创建、启动、停止、删除任务。
- 查询任务状态。

不承诺 BT、磁力链、ed2k、Linux ARM64、多用户隔离或未经实机验证的下载速度。
