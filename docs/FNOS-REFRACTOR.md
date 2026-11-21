# fnOS Native Refactor

Target test device: fnOS 1.2.1102, Intel x86_64, 16 GB RAM.

## Official platform references
- Framework: https://developer.fnnas.com/docs/core-concepts/framework/
- Manifest: https://developer.fnnas.com/docs/core-concepts/manifest/
- Environment variables: https://developer.fnnas.com/docs/core-concepts/environment-variables/
- Privilege: https://developer.fnnas.com/docs/core-concepts/privilege/
- Resource: https://developer.fnnas.com/docs/core-concepts/resource/
- App entry: https://developer.fnnas.com/docs/core-concepts/app-entry/
- Unified gateway: https://developer.fnnas.com/docs/core-concepts/gateway-registration/
- Wizard: https://developer.fnnas.com/docs/core-concepts/wizard/

## Implemented in this branch
- Manifest uses the documented key/value format, platform=x86, minimum OS 1.2.0, and desktop entry fnsdownload.main.
- app/ui/config registers /app/fnsdownload and app.sock.
- TRIM_APPDEST is treated as the target directory; the Go process listens at TRIM_APPDEST/app.sock.
- config/privilege uses the dedicated package account.
- config/resource declares fnsdownload/downloads; Go resolves the path from TRIM_DATA_SHARE_PATHS.
- cmd/main implements idempotent start, stop and status with socket readiness checks.
- install and upgrade callbacks validate the x86_64 payload and prepare persistent data/log directories.
- uninstall honors the keep-data choice and preserves the download share.
- Go tests cover target/share path resolution and gateway-prefix routing.
- Build script validates shell syntax, JSON configs, Manifest/entry consistency, Go tests and FPK contents.

## Not yet verified
- Real installation and desktop launch on the physical fnOS device.
- SDK native loading and actual Xunlei login/download. The current CI artifact is explicitly an SDK stub build because the official Linux x64 SDK headers and libraries are not present in the repository/CI.
- Upgrade and uninstall behavior on the target NAS.

## Release rule
Do not merge this branch as a production downloader release until the physical fnOS 1.2.1102 test and official SDK integration pass.
