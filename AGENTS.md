# AGENTS.md — MoxYi Download

## Product identity
- Repository: 520yxl/fnsdownload
- Product: MoxYi Download, a native fnOS desktop FPK application.
- Supported target: x86_64 / amd64 only. fnOS manifest value: platform=x86.
- Runtime/deployment: native Go process. Docker, Compose, containers and sidecars are prohibited.
- fnOS system baseline for the current test target: 1.2.1102. Do not claim broader compatibility without testing.
- Authorization center: Thunder_auth, product_id moxyi-download, HMAC-SHA256, shared Login Token, scopes xtask.p2sp + xindex.query.

## Official fnOS contract
- Follow https://developer.fnnas.com/docs/core-concepts/framework/
- Follow https://developer.fnnas.com/docs/core-concepts/manifest/
- Follow https://developer.fnnas.com/docs/core-concepts/environment-variables/
- Follow https://developer.fnnas.com/docs/core-concepts/privilege/
- Follow https://developer.fnnas.com/docs/core-concepts/resource/
- Follow https://developer.fnnas.com/docs/core-concepts/app-entry/
- Follow https://developer.fnnas.com/docs/core-concepts/gateway-registration/
- `TRIM_APPDEST` is the installed target directory, not its parent. The gateway socket is exactly `${TRIM_APPDEST}/app.sock`.
- Use `TRIM_PKGVAR` for persistent runtime data and `TRIM_PKGETC` for configuration.
- Declare user-visible download shares in `config/resource` and consume `TRIM_DATA_SHARE_PATHS`; never hard-code /vol1 or @appshare paths.
- Manifest `desktop_applaunchname` must exactly match a key in `app/ui/config`.
- Gateway routes must remain under `/app/fnsdownload`; do not expose a public TCP port.
- Use package-user execution and least privilege. Do not silently switch the long-running service to root.

## FPK contract
- FPK root: manifest, config/, cmd/, wizard/, i18n/, ICON.PNG, ICON_256.PNG and app.tgz.
- app.tgz includes server/fnsdownload-amd64 and the complete ui/ tree (index.html, config, assets and images).
- Lifecycle scripts implement install_init, install_callback, main start/stop/status, upgrade_init, upgrade_callback, uninstall_init, uninstall_callback, config_init and config_callback.
- Scripts must be idempotent, quote variables, avoid hard-coded volume paths, and write actionable failures to TRIM_TEMP_LOGFILE.
- Upgrade preserves user data. Uninstall follows wizard_keep_data and never deletes the user-visible download share without explicit user choice.

## Go and Web UI
- Go owns the backend, API, task state, SQLite and SDK bridge.
- Web UI is static content packaged under target/ui.
- Do not introduce Node.js runtime dependency on the NAS.
- Unit tests must cover env path resolution, gateway prefix routing, task persistence and path traversal protection.

## Xunlei SDK
- Use the official Linux x64 SDK only.
- Integrate the vendor-supported C ABI using Go CGO; do not create a separate C++ service.
- Do not claim SDK integration until actual official headers and shared libraries are supplied and a tagged CGO build succeeds.
- Never store the official Thunder API Key on the NAS or in this repository.
- Login Token is issued online by Thunder_auth; never call an SDK API that exchanges a locally stored official API Key for a token.
- Only claim HTTP(S) downloads unless official SDK documentation and device tests prove additional protocols.

## Thunder_auth
- Match 520yxl/Thunder_auth/docs/API.md exactly.
- Canonical HMAC string: THUNDER-AUTH-V1\nMETHOD\nPATH\nTIMESTAMP\nNONCE\nBODY_SHA256.
- Use lowercase hex SHA256/HMAC-SHA256, Unix-second timestamp and Base64URL nonce.
- Never log Login Tokens, product secrets, cookies, passwords or signed request headers.
- Product Secret is an application credential and may be extracted by a NAS administrator; never describe it as hardware-bound.

## Development and release
- Go tests and vet must run in CI.
- Build target is Linux amd64 only.
- Build a native FPK with official fnpack; no Docker pipeline or image.
- FPK build success does not prove real SDK login or downloads.
- Validate install, desktop launch, start/stop, gateway health, share path, upgrade preservation and uninstall on physical fnOS 1.2.1102 x86_64.
