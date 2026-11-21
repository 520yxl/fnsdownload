# 测试规范

## 自动化
- HMAC 规范串确定性。
- GET 空 body SHA256。
- Thunder_auth 响应失败处理。
- URL 协议白名单与文件名路径穿越拒绝。
- SDK 缺失时明确报错，单元测试不加载原生模块。

## 手工联调
- Thunder_auth 注册设备与获取共享 Token。
- SDK init/login。
- HTTP/HTTPS 下载。
- 查询进度、暂停、继续、删除。
- 应用重启和设备重启恢复。

## 发布阻断
真实 fnOS x86_64 设备未通过 SDK 原生加载与真实下载前，不得标记为正式可用版本。
