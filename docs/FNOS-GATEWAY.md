# fnOS 统一网关接入规范

依据飞牛开发文档：
https://developer.fnnas.com/docs/core-concepts/gateway-registration/

## 访问模式
MoxYi Download 是长期运行的 Go 服务，采用统一网关，不采用 index.cgi，也不向桌面入口暴露独立 HTTP 端口。

## 桌面入口
`app/ui/config` 注册：
- `gatewayPrefix`: `/app/fnsdownload`
- `gatewaySocket`: `app.sock`
- `url`: `/app/fnsdownload`
- `protocol` 和 `port` 不参与统一网关路由。

## Unix Socket
Socket 必须位于已安装应用的 target 目录：
```
${TRIM_APPDEST}/target/app.sock
```
Go 服务从 `TRIM_APPDEST` 计算该路径；生命周期脚本负责创建 target 目录、启动服务并等待 Socket 就绪。禁止将 Socket 放在应用根目录或 PKGVAR。

## 路由
所有 HTTP 与 WebSocket 路由都必须位于 `/app/fnsdownload` 前缀之下。桌面入口访问 `/app/fnsdownload`，网关校验 fnOS 用户会话后转发到 Unix Socket。

## 用户身份 Header
网关转发：
- `X-Trim-Userid`
- `X-Trim-Isadmin`
- `X-Trim-Username`

这些 Header 仅代表网关提供的登录身份上下文，不等于业务授权。应用仍须对管理操作、下载任务和文件访问执行自己的权限校验。不能信任浏览器端自行提交的用户 ID。由于 Socket 不暴露为普通 TCP 端口，应用只接受经本地网关转发的请求。

## 安全要求
- 下载路径必须规范化并限制在用户配置的下载根目录内。
- 管理 API 必须检查网关身份与管理员权限。
- 不把 Header、Token、Secret、Cookie 写入日志。
- WebSocket 路由同样放在网关前缀下，并绑定已验证的用户身份。
