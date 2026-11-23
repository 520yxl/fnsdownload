# 迅雷 SDK 接入

Go 服务通过 CGO 直接调用迅雷官方 C ABI，不需要额外 Node.js 或 C++ 服务。
仅支持 Linux x64，以及 HTTP / HTTPS P2SP 下载。

## 官方文件

运行 `python3 scripts/fetch-xunlei-sdk.py` 下载官方 C++ SDK，并校验固定 SHA256：
`718923688ac3717f8edd316f46767ed61a7b82a912edfb6143233a29e34b9819`。
只提取官方头文件与 Linux x64 `libdk.so` 到 `third_party/xunlei`；二进制不提交 Git。
下载源： https://down.sandai.net/opensdk/download/xunlei-dlsdk-cpp.zip
官方说明： https://open.xunlei.com/doc/download/sdk/cpp

## 构建

在原生 Linux amd64 构建机执行：

```bash
python3 scripts/fetch-xunlei-sdk.py
XUNLEI_SDK_ROOT="$PWD/third_party/xunlei" bash scripts/build-fpk.sh
```

CI 使用上述流程，执行带 `xunlei_sdk` 标签的 Go 测试、vet、编译和真实动态库加载测试。
FPK 将运行库打包到 `server/lib/libdk.so`，可执行文件 RUNPATH 为 `$ORIGIN/lib`。
无 SDK 的开发构建明确显示引擎不可用，不能当作 SDK 版本发布。

## 授权配置

默认 App ID 使用用户提供的 App ID；默认授权中心为 `https://thunder-auth.moxyi.cn`，产品 ID 为 `moxyi-download`。
需要在授权中心为该产品配置至少 32 字节的产品 Secret，再将同一值写入 NAS 应用私有目录
`$TRIM_PKGVAR/config/thunder-auth.secret`（纯文本、无引号，目录权限 0700，文件权限 0600，由应用用户持有）。
不要写入迅雷官方 API Key。不要把 Secret 提交到 Git 或发到聊天里。
可用环境变量 `XUNLEI_APP_ID`、`THUNDER_AUTH_URL`、`THUNDER_AUTH_SECRET_FILE` 覆盖默认值。
修改配置后重启应用。

服务自动生成持久设备标识，按照 Thunder_auth API v1 签名注册并获取共享 Login Token，检查
`xtask.p2sp`、`xindex.query` Scope 后调用 SDK login。Token 只保存在进程内存，临近过期时重新获取。
授权中心需保持有效 Token；需要管理员手动刷新时，应用显示授权错误，不保存官方 API Key。
签名协议： https://github.com/520yxl/Thunder_auth/blob/main/docs/API.md

## 验证边界

模拟 SDK 测试覆盖任务控制、64 位任务 ID、状态映射、删除失败和重启恢复。
原生测试验证真实库的版本查询、初始化和释放，不代表真实账号登录或下载通过。
NAS 上仍需验证登录、HTTP(S) 下载、暂停/继续、删除保留文件、升级后任务恢复。
不承诺 BT、磁力链、ed2k、ARM64 或未经实机验证的速度。
