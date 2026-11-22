# fnOS 统一网关接入规范

依据飞牛官方文档：
- https://developer.fnnas.com/docs/core-concepts/gateway-registration/
- https://developer.fnnas.com/docs/core-concepts/app-entry/
- https://developer.fnnas.com/docs/core-concepts/manifest/
- https://developer.fnnas.com/docs/core-concepts/environment-variables/
- https://developer.fnnas.com/docs/core-concepts/icon/

## 1. 访问模型

MoxYi Download 是长期运行的 Go HTTP 服务，使用 fnOS 统一网关，不使用 index.cgi，也不向桌面入口暴露独立 HTTP 端口。

## 2. 桌面入口注册

- Manifest: `desktop_uidir=ui`
- Manifest: `desktop_applaunchname=fnsdownload.main`
- Entry ID: `fnsdownload.main`（以 appname 为前缀）
- Gateway prefix: `/app/fnsdownload`
- Gateway socket: `app.sock`
- Entry URL: `/app/fnsdownload/`

入口图标使用 `images/icon_{0}.png`，必须同时提供：
- `app/ui/images/icon_64.png`
- `app/ui/images/icon_256.png`

## 3. Unix Socket

`TRIM_APPDEST` 是安装后的 target 目录。服务必须监听：

`${TRIM_APPDEST}/app.sock`

`gatewaySocket` 只写 Socket 文件名 `app.sock`，不得写绝对路径或目录。生命周期脚本从 `TRIM_APPDEST` 读取 target 目录，启动服务并等待 Socket 创建成功。

## 4. 网关路径和反向代理

fnOS 注册 `/app/fnsdownload` 并把请求转发到 Unix Socket。外部 HTTP 路径保持在该前缀下：
- 页面：`/app/fnsdownload/`
- 前端资源：`/app/fnsdownload/assets/*`
- API：`/app/fnsdownload/api/*`
- WebSocket（未来启用时）：`/app/fnsdownload/ws/*`

Go 服务收到请求后使用 `http.StripPrefix("/app/fnsdownload", handler)` 将内部路由映射到 `/`、`/assets/*`、`/api/*`。不得把 API 或静态资源请求发送到网关前缀之外。

Vite `base` 固定为 `/app/fnsdownload/`。前端 API 客户端使用相同的网关前缀，不能使用根路径 `/api`。

## 5. 身份与授权

网关在转发前校验 fnOS 登录会话，并提供：
- `X-Trim-Userid`
- `X-Trim-Isadmin`
- `X-Trim-Username`

应用不能信任客户端自行提交的用户 ID。每个任务查询、创建、暂停、继续、删除和文件操作都必须校验网关身份，并执行应用自身的业务授权。管理员专属操作还应验证 `X-Trim-Isadmin=true`。

## 6. 参考入口配置

```json
{
  ".url": {
    "fnsdownload.main": {
      "title": "MoxYi Download",
      "desc": "飞牛原生多引擎下载器",
      "icon": "images/icon_{0}.png",
      "type": "iframe",
      "gatewayPrefix": "/app/fnsdownload",
      "gatewaySocket": "app.sock",
      "url": "/app/fnsdownload/",
      "width": 1280,
      "height": 820,
      "allUsers": false,
      "control": {
        "accessPerm": "readonly"
      }
    }
  }
}
```

## 7. 构建时强制检查

构建必须验证 Manifest 桌面入口声明与 UI config 的入口 ID 一致；验证网关前缀、Socket 文件名和 URL；验证入口图标文件名与图标模板匹配；验证前端构建产物引用的资源 URL 全部位于网关前缀内；验证 Go handler 的网关路径映射测试。
