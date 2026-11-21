# SQLite task persistence (V0.5.0)

## Location
The database is stored at `DATA_DIR/tasks.db`. The default DATA_DIR resolves under the fnOS package persistent variable directory, not the versioned application installation directory. Keep this directory across application upgrades and uninstall/reinstall flows according to the package's preserve-data policy.

## Durability
- SQLite is embedded through modernc.org/sqlite, so the x86_64 service remains a pure-Go build with CGO disabled.
- WAL journaling, FULL synchronous mode, foreign keys, and a 5-second busy timeout are enabled.
- Task payloads are stored as JSON with indexed engine/status/timestamps for future filtering and migration.
- Task creation is persisted before the engine is started. Progress and state are checkpointed on task-list polling and on orderly shutdown.

## Recovery
- HTTP tasks are rehydrated into the HTTP adapter at service startup.
- Tasks that were running or queued when the process stopped are restored as paused.
- Existing .part files are inspected and their actual byte size reconciled into the task record.
- HTTP resume sends a Range request. If the origin server does not support ranges, the response is safely written from byte zero rather than appending corrupt data.
- Aria2 remains an external daemon adapter; its runtime tasks are queried from aria2 and merged with persisted task records.

## Operational notes
- Keep the package variable directory when upgrading.
- The download directory is separate from the database directory.
- A task record is deleted only when the user removes the task through the API.
