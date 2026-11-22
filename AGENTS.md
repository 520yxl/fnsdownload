# AGENTS.md — MoxYi Download

## Project identity
- Repository: 520yxl/fnsdownload
- Product: MoxYi Download, a native fnOS desktop FPK application.
- Supported target: x86_64 / amd64 only. Do not add ARM64 binaries, builds, package targets or compatibility claims.
- Runtime and deployment: native Go process in fnOS FPK. Docker, Compose and containers are prohibited.
- Authorization center: Thunder_auth, product_id `moxyi-download`, HMAC-SHA256, shared Login Token, scopes `xtask.p2sp` + `xindex.query`.

## Architecture
- Go backend; Vue 3 static UI embedded with Go embed.
- fnOS package structure follows the supplied FnDepot FPK reference: manifest, config, cmd, wizard, i18n, ICON.PNG, ICON_256.PNG and app.tgz containing server/ and ui/.
- FPK contains the amd64 Go executable and required runtime libraries.
- Native process lifecycle managed by fnOS scripts under cmd/.
- Persistent configuration, SQLite database, SDK state and logs live under TRIM_PKGVAR.
- Downloads live under the fnOS app-share directory.

## Xunlei SDK
- Use the official Linux x64 SDK only.
- Integrate the vendor-supported C ABI using Go CGO; do not create a separate C++ service.
- Bundle the required official shared libraries in the FPK and configure a package-relative library search path.
- Do not claim the SDK is integrated until the official SDK headers and libraries are present and a tagged CGO build succeeds.
- Never store the official Thunder API Key on the NAS or in this repository.
- Login Token is issued online by Thunder_auth; never call an SDK API that exchanges a locally stored official API Key for a token.
- Only claim HTTP(S) downloads unless official SDK documentation and device tests prove additional protocols.

## Thunder_auth
- Match `520yxl/Thunder_auth/docs/API.md` exactly.
- Canonical HMAC string: `THUNDER-AUTH-V1\nMETHOD\nPATH\nTIMESTAMP\nNONCE\nBODY_SHA256`.
- Use lowercase hex SHA256/HMAC-SHA256, Unix-second timestamp and Base64URL nonce.
- Never log Login Tokens, product secrets, cookies, passwords or signed request headers.
- Product Secret is an application credential and may be extracted by a NAS administrator; never describe it as hardware-bound.

## Development and release
- Go tests and vet must run in CI.
- Build target is Linux amd64 only.
- Build a native FPK with official fnpack; no Docker pipeline or image.
- FPK build success does not prove real SDK login or downloads.
- Validate installation, desktop launch, start/stop/restart, authorization, SDK login, HTTP(S) download, pause/resume/delete, upgrade and uninstall on a physical fnOS x86_64 NAS.
