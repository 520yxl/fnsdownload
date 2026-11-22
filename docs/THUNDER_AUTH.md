# Thunder_auth 接入

固定产品：moxyi-download；认证：HMAC-SHA256；Token：shared；Scopes：xtask.p2sp + xindex.query。

签名规范必须与 520yxl/Thunder_auth/docs/API.md 一致：
```
THUNDER-AUTH-V1
{METHOD}
{PATH_WITHOUT_QUERY}
{TIMESTAMP}
{NONCE}
{SHA256_RAW_BODY}
```
HMAC-SHA256 使用 Product Secret，签名为小写十六进制。GET body 为空字符串；POST body 为实际发送 JSON 字节。时间戳为 Unix 秒，nonce 为随机 Base64URL。

调用顺序：
1. POST /api/v1/auth/register：注册设备。
2. POST /api/v1/auth/token：获取共享 JWT。
3. 校验 token 与 expires_at，不记录 JWT。
4. 调用 SDK login(token)。

授权中心地址与产品 Secret 由 NAS 管理员首次设置。Secret 不得写入源码、镜像层、Compose 或 CI。
