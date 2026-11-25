# 测试规范

## 自动化
- HMAC 规范串确定性。
- GET 空 body SHA256。
- Thunder_auth 响应失败处理。
- URL 协议白名单与文件名路径穿越拒绝。
- SDK 缺失时明确报错，单元测试不加载原生模块。

## 手工联调
使用 fnos-cli Skill 连接 x86_64 飞牛测试设备，完成 OAuth 授权和只读设备查询后，通过应用中心预检 FPK：

```bash
fnos-cli --profile <profile> app install-fpk ./fnsdownload-x86_64.fpk --volume-id <volume> --dry-run --yes
fnos-cli --profile <profile> app install-fpk ./fnsdownload-x86_64.fpk --volume-id <volume> --yes
fnos-cli --profile <profile> app status fnsdownload
fnos-cli --profile <profile> app stop fnsdownload --yes
fnos-cli --profile <profile> app status fnsdownload
fnos-cli --profile <profile> app start fnsdownload --yes
fnos-cli --profile <profile> app status fnsdownload
fnos-cli --profile <profile> app restart fnsdownload --yes
fnos-cli --profile <profile> app status fnsdownload
fnos-cli --profile <profile> app entry list
```

先读取应用中心实际使用的卷，明确填写 volume ID。安装可能升级同名应用；测试失败不代表安装已回滚。需要许可确认或自定义向导参数时，按 Skill 和命令帮助处理。

应用中心状态和入口检查不能代替登录 fnOS 桌面后打开图标，验证网关转发和用户身份。该项目本次验收使用 fnos-cli Skill，无需 NAS 内置 appcenter-cli。

- Thunder_auth 注册设备与获取共享 Token。
- SDK init/login。
- HTTP/HTTPS 下载。
- 查询进度、暂停、继续、删除。
- 应用重启和设备重启恢复。

## 发布阻断
真实 fnOS x86_64 设备未通过 SDK 原生加载与真实下载前，不得标记为正式可用版本。
