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
