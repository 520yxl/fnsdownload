# fnOS Unified Gateway Integration

Source of truth: https://developer.fnnas.com/docs/core-concepts/gateway-registration/

- Use the unified gateway because the application is a long-running Go service that needs fnOS login-session validation and future WebSocket support.
- app/ui/config declares gatewayPrefix=/app/fnsdownload and gatewaySocket=app.sock.
- TRIM_APPDEST is already the installed target directory. The socket path is exactly ${TRIM_APPDEST}/app.sock.
- protocol and port are ignored for gateway routes. The app must not declare or bind a separate public port.
- All HTTP routes must be served below /app/fnsdownload; the Go handler strips this prefix before dispatching application routes.
- fnOS validates the login session before proxying and forwards X-Trim-Userid, X-Trim-Isadmin, and X-Trim-Username.
- These headers are trusted only because they arrive over the local gateway socket. The app still enforces business authorization and must not trust client-supplied identity fields.
- File paths must be normalized, traversal rejected, and downloads constrained to TRIM_DATA_SHARE_PATHS.
