# ADR-002: Run capture and restoration as a graphical-session service

**Status:** Accepted

## Decision

The Go backend runs under the systemd user graphical-session lifecycle. DMS invokes the same binary for interactive operations but does not own the daemon process.

## Consequences

Snapshots continue while DMS reloads, shutdown can be captured, and restoration can begin before the widget is constructed.
