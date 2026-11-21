# AGENTS.md — MoxYi Download

## Project identity
- Repository: 520yxl/fnsdownload
- Product: MoxYi Download for fnOS (飞牛 NAS)
- Authorization center: https://auth.moxyi.cn (deployment URL is configurable; do not assume this hostname is live)
- Thunder authorization product: product_id `moxyi-download`
- Thunder_auth protocol: HMAC-SHA256, shared Login Token, scopes `xtask.p2sp` + `xindex.query`.
- All application code, packaging, tests and CI belong in this repository. Thunder_auth remains a separate service/repository.

## Confirmed official SDK boundary
- Official Node.js SDK package: `@xunlei-open/dlsdk`.
- Official native package for Linux x64: `@xunlei-open/dlsdk-native-linux-x64`.
- Official Node.js SDK docs currently list Linux x64 but do not list Linux ARM64. Do not claim ARM64 support until the vendor publishes/supports a matching native package and it passes hardware tests.
- SDK initialization uses `XLDownloadAPI.init({appId, appVersion, configPath, saveTasks})`; login uses `sdk.login(loginToken)`; P2SP task uses `sdk.createP2spTask({url, savePath, saveName})`; start uses `sdk.startTask(taskId)`; release uses `sdk.uninit()`.
- Never call `sdk.getLoginToken(apiKey)` in production. Login Token must come from Thunder_auth. The official API Key must never be stored on NAS, in this repository, in the image or FPK.
- Do not invent BT/magnet/ed2k support. Official docs currently state HTTP(S) only.

## Architecture
- Docker-based fnOS FPK, Node.js 22 LTS, Linux x64 first.
- Node HTTP server (no unnecessary framework), web UI served from `public/`.
- Persistent data: `/data`; downloads: `/downloads`.
- SDK config: `/data/sdk`; app config and task metadata in `/data`.
- Thunder_auth HMAC signer must match `Thunder_auth/docs/API.md` exactly:
  canonical string: `THUNDER-AUTH-V1\nMETHOD\nPATH\nTIMESTAMP\nNONCE\nBODY_SHA256`, lowercase hex SHA256/HMAC-SHA256, timestamp Unix seconds, nonce base64url.
- HMAC secret is an application credential and may be extracted by a NAS administrator. Do not describe it as hardware-bound or unextractable.
- Never log Login Tokens, product secrets, cookies, passwords or signed request headers.

## Development rules
- Use ESM and Node built-ins where possible.
- Keep the SDK behind `src/thunder-sdk.js`; unit tests must run without the proprietary native module.
- Fail clearly when the SDK native package is missing or architecture is unsupported.
- Validate download URLs as HTTP(S); reject other protocols.
- Avoid path traversal: save names must be basenames.
- Bind web UI to 0.0.0.0 inside the container; expose only the configured fnOS application port.
- No secrets in Docker build args, image layers, GitHub Actions logs or commits.

## Build and validation
- `npm test` runs unit tests without native SDK.
- `docker build` creates the runtime image.
- FPK packaging uses fnOS `fnpack`; real install/upgrade/stop/uninstall must be validated on a physical fnOS device.
- CI may build Linux amd64 only until ARM64 SDK support is confirmed.
- Do not claim real SDK login/download or FPK installation passed unless the workflow/device actually confirms it.
