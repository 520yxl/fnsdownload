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

## 尚未完成

Windows 官方 fnpack 1.3.3 在工作区和临时目录均报告 `sync ... Access is denied`，未生成成功的新包。需在 Linux amd64 构建环境重新运行打包流程，再进行预检、安装和生命周期检查。

按用户最新要求，后续使用 fnos-cli Skill 验证，不再要求 NAS 内置 appcenter-cli。SDK 登录、真实下载、桌面网关、升级及卸载验收尚未完成。
