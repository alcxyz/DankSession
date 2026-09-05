# ADR-003: Relaunch only explicitly configured applications

**Status:** Accepted

## Decision

DankSession stores a stable application identifier and uses a configured argument array to relaunch it. It never saves or executes a process command line discovered from `/proc`.

## Consequences

Applications without a launch rule can be repositioned while running but are not restarted after login. This avoids persisting tokens, transient flags, or unsafe arguments.
