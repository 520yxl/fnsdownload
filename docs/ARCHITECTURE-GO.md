# Go Native Runtime

- Go owns the HTTP API, authorization client, task manager, persistence and lifecycle runtime.
- The UI is static Web content packaged under target/ui.
- The native Go process listens only on the fnOS Unix gateway socket.
- fnOS platform value is x86; Go target is GOOS=linux GOARCH=amd64.
- No Docker, Node.js runtime or exposed TCP service is part of the NAS deployment.

## Paths supplied by fnOS
- TRIM_APPDEST: installed target directory; app.sock is created directly here.
- TRIM_PKGETC: persistent configuration.
- TRIM_PKGVAR: persistent app data, database and logs.
- TRIM_PKGHOME: package user home.
- TRIM_DATA_SHARE_PATHS: system-created shared download directory.

## Security
The application uses config/privilege run-as package. The gateway validates the fnOS login session and supplies X-Trim identity headers. The app still authorizes business operations and constrains file access to the configured download share.

## SDK
A production SDK build requires the actual official Linux x64 SDK headers and shared libraries. An SDK-less build is a gateway/UI development artifact only.
