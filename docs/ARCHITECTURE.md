# MoxYi Download — fnOS Native Architecture

This document replaces the obsolete Docker/Node.js deployment description.

## Product constraints
- Native fnOS FPK; no Docker, Compose, container runtime or sidecar.
- Go backend and static Web UI.
- x86_64 / amd64 only; fnOS manifest platform is x86.
- Thunder_auth is the online product authorization service. The NAS never stores the official Thunder API Key.

## fnOS package contract
The FPK root contains manifest, config/, cmd/, wizard/, i18n/, app icons and app.tgz. The extracted app payload contains:
- server/fnsdownload-amd64: native Go service.
- ui/index.html, ui/config, ui/assets/, ui/images/: desktop UI and entry registration.
- app.sock: created at runtime directly under TRIM_APPDEST.

## fnOS paths
- TRIM_APPDEST: installed target directory. Gateway socket is ${TRIM_APPDEST}/app.sock.
- TRIM_PKGETC: application configuration directory.
- TRIM_PKGVAR: persistent application data, SQLite, state and logs.
- TRIM_PKGHOME: application home.
- TRIM_DATA_SHARE_PATHS: system-created download share declared in config/resource.

No script may hard-code a volume number or manually invent an @appshare path.

## Desktop gateway
The stable desktop entry ID is fnsdownload.main, declared both in Manifest and app/ui/config. It registers /app/fnsdownload and app.sock. The Go HTTP server serves all UI/API paths below the gateway prefix. WebSocket paths, when implemented, must remain under the same prefix.

## Process lifecycle
cmd/main owns start/stop/status. It validates x86_64, checks the packaged Go binary and UI, starts one background process, waits for the Unix socket, terminates gracefully, and returns fnOS-compatible status codes. Lifecycle scripts are idempotent and report actionable failures to TRIM_TEMP_LOGFILE.

## Security and access
The service runs as the dedicated package user (config/privilege). The gateway authenticates fnOS sessions and forwards X-Trim-Userid, X-Trim-Isadmin, and X-Trim-Username. The application must enforce business permissions; never trust client-supplied user IDs. Downloads are stored only in the declared share. All paths are normalized and constrained to the download root.

## SDK boundary
The official Linux x64 Thunder SDK is integrated only when the actual vendor SDK headers and shared libraries are supplied to the build. A stub/dev build is not a functional download client and must not be represented as production-ready.
