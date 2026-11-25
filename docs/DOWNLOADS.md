# Download queue and retries

Each submitted URL creates an independent task. Duplicate URLs are checked in the UI and again under the backend queue lock. Confirmation can skip duplicates or explicitly create new tasks in private subdirectories. URL fragments are ignored; path case and query parameters are preserved.

The application-wide concurrency setting governs Xunlei, the private aria2 instance and HTTP together. A backend worker starts waiting tasks in creation order without a browser. Lowering the limit does not interrupt active transfers.

Failed tasks can be retried individually or through multi-select. Retries retain their task identity and use the queue. aria2 re-adds its owned failed GID using the original directory, filename and partial file; it never enables overwriting or manages foreign GIDs.

Automatic retries default to off. A NAS-persisted setting permits 1–5 additional attempts. Backoff starts at 30 seconds and doubles after each attempt. Waiting does not occupy a download slot. HTTP 408/429/500/502/503/504 and recognized network timeouts/resets are eligible; aria2 codes 2/5/6/19/29 are eligible according to its official manual. Authorization, missing resources, disk errors and unknown failures do not retry automatically. Unknown Xunlei SDK error codes require manual retry.

A task's automatic retry can be cancelled while waiting. Manual retry/resume resets its automatic budget; completion or deletion clears it. Disabling automatic retries cancels pending timers. Attempt state is runtime state; application restart blocks automatic continuation of restored tasks until the user explicitly resumes or retries them. Preferences persist across browsers and application restarts.

CI covers queue sharing, retry budgets/backoff, cancellation, duplicate confirmation and real aria2 failure-to-success retry. A successful build is not proof of Xunlei login/downloads or physical fnOS acceptance.


## Storage protection

A NAS-persisted reserve setting defaults to 512 MB (64–102400 MB). Before each queued task starts, the worker reads available filesystem space for its actual destination. Linux uses statfs available blocks and device identity; paths on the same device share the sum of known remaining bytes of this application's active transfers. Unknown-size transfers require the reserve but cannot be fully estimated. This is a conservative estimate, not allocation of disk space or a guarantee against changes by other applications, filesystem quotas or preallocation.

Insufficient space leaves tasks queued with an actionable reason; they can start after space is freed. An unreadable destination or missing volume blocks new starts with a distinct warning. While registered tasks are active, the worker monitors their destinations even if the browser is closed. Falling below the reserve requests pause only for this application's tasks and preserves partial files. Paused tasks require manual resume; a failed pause request is displayed as a warning rather than reported as a successful pause. Other applications' task IDs and processes are not controlled.

CI tests cover same-device reservations across directories, independent devices, unknown sizes, read failures, low-space pause, explicit resume and native filesystem queries. Physical fnOS verification is still required.

## Task creation and file access

New downloads show only URLs by default. The authorized destination remains visible below the URL input. Advanced settings expand to engine selection and Xunlei single-file HTTP headers. Engine selection defaults to automatic on each new dialog; explicit choices apply to every submitted URL. Unavailable engines are disabled. Switching away from Xunlei-capable modes omits hidden request headers. File-manager actions in task rows and details use the official TrimApp.openFileManager API and require the fnOS host; standalone previews show a clear error.

## Download time

Task payloads persist elapsed_ms in SQLite. The backend queue checkpoints running tasks even without a browser; pauses, queue waits, retry delays and application downtime do not accrue time. Fresh engine snapshots retain the persisted application counter. Sampling uses observed running transitions (normally once a second); abrupt process termination can lose the last uncommitted sample. Existing records start accumulating after this update; historical elapsed time cannot be recovered. Completion keeps accumulated time, while deletion removes it. The UI estimates remaining time from remaining bytes/current speed only for running tasks, and shows calculating when size or speed is unknown. ETA fluctuates with speed and is not a completion guarantee.

## Automatic engine fallback

New tasks created in automatic mode retain that preference in SQLite. Documented transient failures, including Xunlei 111176 (long time without receiving data) and 119212 (timeout) with normal task-token state, can trigger a switch to an untried available engine. A running automatic task with no byte progress for two minutes can also switch; progress resets the timer. Each engine is tried at most once, using Xunlei, aria2, HTTP priority among untried available engines. Manual selections, custom-header tasks, paused/restored-blocked tasks, authorization errors, HTTP 403/404, certificate and storage failures do not qualify. Unknown SDK errors are not guessed. Existing 0.5.25 records do not identify automatic versus manual selection, so they are not silently opted in; recreate such tasks with automatic selection to use fallback.

The queue retains a stable public task ID and original creation time while persisting the current native engine ID. A running old transfer must stop before replacement is queued. Replacement downloads use a new private subdirectory under the authorized root and start afresh; partial files are retained rather than combined across incompatible engines. Cumulative download time survives switches. Pause/resume/delete and restart restoration resolve the stable ID to the current native task, and unbound old native records do not create duplicate list rows. Automatic fallback shares the global queue, concurrency and disk-space protection. Physical fnOS fallback acceptance remains required.

The supplied 0.5.25 task error was `迅雷任务错误 111176，授权状态 0`: official SDK headers name 111176 XL_DL_ERROR_LONG_TIME_NO_RECV_DATA and task-token state 0 XL_DL_TASK_TOKEN_NORMAL. This identifies a no-data timeout, not which source/network component caused it. Skill CLI 0.5.1 confirmed the updated app running and a growing app.log but redacted its download URI, so the log body was not inspected. Reference: https://open.xunlei.com/doc/download/api/error-code .
