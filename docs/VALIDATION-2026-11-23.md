# 2026-11-23 验证记录

## 已验证

- 使用用户提供的 fnos-cli Skill 0.5.1，通过 OAuth 连接测试 NAS；令牌保存成功，只读查询成功。
- NAS：Mainland-PE，平台 x86，系统版本 1.2.1102，内核 6.18.18.c1186-trim。
- 应用中心未找到 fnsdownload；默认应用卷为 1。
- 本地 Go 测试、go vet、Vue/Vite 构建通过。
- 修复 HTTP 下载完成时未关闭文件就重命名的问题；现有断点恢复测试通过。

## CI 基线包预检失败

产物来自 develop 提交 `03dcf0bb5d0315498bf5c369bd54de956e9d3a0e`，工作流运行 `44986043877`，产物 ID `14738954611`。下载 ZIP 的 SHA256 已与 GitHub 返回值核对。

版本 0.5.11，SDK_BUILD=stub。此产物不包含本次本地修改。

执行 `app install-fpk <file> --volume-id 1 --dry-run --yes`：上传后，安装信息 GET 返回错误 10111，`runtime error: invalid memory address or nil pointer dereference`。未启动实际安装。NAS 历史日志存在多次“应用包格式不符合系统版本要求”，不能将历史记录当成本次安装结果。

包内 app.tgz 的 MD5 与 manifest checksum 相符。发现包内 cmd/main 权限为 0644。本次修改构建脚本，对全部生命周期脚本设置 0755，并检查最终包权限、shebang 和 Unix 换行。该问题与 NAS 预检错误的因果关系尚未验证。

## 定位与修复

Windows 官方 fnpack 1.3.3 在工作区和临时目录均报告 `sync ... Access is denied`；已改用 GitHub Linux CI 成功构建。PR： https://github.com/520yxl/fnsdownload/pull/4 。

对照测试均使用同一 fnos-cli 0.5.1、同一 NAS 和卷 1：

| 软件包 | 安装预检 |
| --- | --- |
| 仓库中的官方 HelloFnos 最小示例 | 通过 |
| 下载器：修正权限，保留空向导和国际化文件 | 10111 |
| 下载器：仅移除国际化文件，仍保留空向导 | 10111 |
| 下载器：移除空向导，保留国际化文件 | 通过 |

空安装向导 `wizard/install` 为 `[]` 时，本设备安装信息接口异常。构建脚本现省略所有无步骤的向导，保留非空卸载向导，并检查最终包不含空向导。已移除临时诊断构建步骤。

## Skill 实机测试

使用通过预检的 `no-empty-wizards` 0.5.11 包安装，安装任务 `1795436646581108169-fnsdownload` 返回 status=2、progress=100。

- 安装后回读：running。
- stop 后回读：stopped。
- start 后回读：running。
- restart 后回读：running。
- 入口查询：fnsdownload.main，可见，iframe 路径 `/app/fnsdownload/`。

后续 0.5.12 正式修复包构建和升级结果另行记录。

## 验证边界

按用户要求，全程使用 fnos-cli Skill 验证，不要求 NAS 内置 appcenter-cli。软件包仍为 SDK stub；SDK 登录、真实下载、浏览器页面交互、升级数据保留及卸载验收尚未完成。入口已注册不等于浏览器端网关功能已通过。
