# ADR-002: Run capture and restoration as a graphical-session service

**Status:** Accepted

## Decision

The Go backend runs under the systemd user graphical-session lifecycle. DMS invokes the same binary for interactive operations but does not own the daemon process.

## Consequences

Snapshots continue while DMS reloads, and restoration can begin before the widget is constructed. Shutdown retains the last completed capture rather than recording desktop teardown. The incoming snapshot is archived once per compositor session, and a session marker prevents automatic restoration from repeating after a daemon restart.

Capture and restore share an inter-process lock. Relaunched applications use independent transient systemd user services; neither DMS nor the capture daemon owns their process lifetime. Home Manager keeps an active capture daemon running during configuration switches, applying its new executable at the next start.
