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

默认 App ID 使用用户提供的 App ID；默认授权中心为 `https://thunder-auth.moxyi.cn`，产品 ID 为 `fnsdownload`。
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


## 私有内置版

可选地将产品 Secret 作为构建输入：

```bash
XUNLEI_PRODUCT_SECRET_FILE=/private/thunder-auth.secret XUNLEI_SDK_ROOT="$PWD/third_party/xunlei" bash scripts/build-fpk.sh
```

私有包将该文件放在 `target/config/thunder-auth.secret`，权限 0600。应用先读取
`$TRIM_PKGVAR/config/thunder-auth.secret`；该文件不存在时才读取包内默认值。
已有配置不会被覆盖；权限错误或无效配置不会静默回退。Token 仍从授权中心动态获取，不内置 Token 或官方 API Key。

普通 PR/推送 CI 不注入产品凭据。手动 workflow_dispatch 只有启用 `bundle_product_secret` 时才使用
GitHub Actions 加密 Secret `THUNDER_AUTH_PRODUCT_SECRET`；配置此 Secret 需单独授权。
该构建产物包含可提取的产品凭据，不自动发布 Release，Actions artifact 仅保留 1 天。
凭据值不能写入源码、工作流输入、日志或文档。


## 0.5.19 SDK 功能入口

- 新建下载：单个 HTTP(S) P2SP，或每行一个链接创建 SDK 原生批量任务，支持批量并发数。
- 新建单文件迅雷任务：自定义 HTTP 请求头，在启动前传入 SDK；不写入应用 SQLite 数据库。
- 下载器设置：SDK 最大并发任务数、下载/上传限速、P2P 上传开关、动态链接加速开关。迅雷设置原子写入 NAS SDK 状态目录的 settings.json，重启重新应用。0 代表不限速，按 SDK 要求映射 UINT32_MAX。
- 任务详情：SDK 流量统计（源站/P2P/P2S）和可用的完成时间。
- 恢复 SDK 任务：通过 finished/unfinished 原生枚举和 task info 导入遗漏记录；检查目录访问权限，未完成任务先暂停，已有记录去重。
- 移除确认：默认保留文件；明确勾选后才调用 SDK 删除文件。
- Token 获取、登录、会话刷新和 SDK 生命周期留在后端。低层 set_task_token 有 C ABI 包装，不提供手工输入或暴露授权 Token 的 UI。
- 用户界面只支持 HTTP(S)，不宣称 BT、磁力、ED2K、迅雷会员账号或离线下载支持。
- 浏览器无活动任务不轮询，任务按创建时间倒序并使用 ID 打破时间相同的排序平局。

参考官方 API：https://open.xunlei.com/doc/download/sdk/cpp
动态链接加速存在极低概率文件不一致风险；界面提供关闭开关与校验提示。P2P 上传默认关闭。

验证层次：伪 SDK 测试覆盖设置持久化、厂商拒绝与磁盘失败回滚、批量任务、请求头注入拒绝、恢复权限和去重；原生测试加载官方库，验证配置 setter 和空任务枚举。创建/下载、非空流量、批量任务和恢复效果仍须 NAS 实机验收，构建通过不代表这些场景已通过。
