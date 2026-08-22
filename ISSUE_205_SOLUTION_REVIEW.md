# Issue #205 solution review

## Candidate 1: keep `File.Sync` in every `DebugLog` call

- Preserves the strongest immediate durability guarantee.
- Fails the reported workload: every debug message blocks the caller on a disk flush, which is especially visible on an HDD and can stall the UI/event loop.
- Rejected.

## Candidate 2: remove syncing entirely

- Removes the per-message latency and is simple.
- Weakens crash diagnostics: a process or machine failure shortly after a message could leave recent log data only in the OS cache.
- Rejected because debug logging is primarily a diagnostic feature.

## Candidate 3: rate-limit syncing and flush on close (chosen)

- Append each formatted line under the existing mutex, sync at most once per two seconds, and sync before closing or switching the log file.
- Keeps recent logs durable within the intended diagnostic window while removing the repeated HDD flush from the hot path.
- Does not add a background worker or public lifecycle API, so there is no ticker/goroutine lifetime to coordinate with tests, reconfiguration, or process shutdown.

## Three-pass review

### Pass 1: correctness

- Concurrent calls remain serialized by `logMu`.
- A failed sync does not advance the sync timestamp, so a later call retries it.
- Rotation, disk logging disablement, and filename changes flush before closing the file.

### Pass 2: compatibility and failure modes

- The existing file format, rotation policy, environment variable behavior, memory ring buffer, stderr mode, and test logger are unchanged.
- Log write errors remain non-fatal, as before.
- A process crash can still lose less than two seconds of OS-cached log data; this is the explicit trade-off for avoiding a flush on every message.

### Pass 3: scope and regression risk

- The change is isolated to the shared `vtui` logger, so it benefits f4 and other consumers of `DebugLog`.
- Tests cover the two-second sync interval and close-time flush, in addition to existing content and rotation tests.
- Native Windows validation will exercise a real f4 build with `VTUI_DEBUG` enabled and a high-volume startup/debug workload.
