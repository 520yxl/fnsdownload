# Download engine architecture

## Contract
Every engine implements `internal/engine.Engine`. The API and task layer must not call engine-specific SDKs directly. Engine IDs are stable names (`http`, `aria2`, `xunlei`).

## Initial adapters
- `http`: native Go HTTP/HTTPS downloader. It writes to a .part file and atomically renames on completion. The initial implementation does not yet provide multi-connection range downloads or restart-safe resume.
- `aria2`: JSON-RPC adapter. aria2 must be installed and configured separately until the native FPK bundling/licensing and daemon lifecycle are completed.
- `xunlei`: existing official SDK boundary; only enabled in builds with the vendor Linux x64 headers and shared libraries.

## Safety
- Never accept arbitrary filesystem paths outside the configured download root at the API layer.
- Do not log URLs containing credentials, authorization headers, RPC secrets, or Thunder tokens.
- Engine availability is determined by health checks, not by package presence alone.
- Do not report an engine as ready until its runtime health check succeeds.
