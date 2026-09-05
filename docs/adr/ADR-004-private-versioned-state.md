# ADR-004: Store private, versioned snapshots outside configuration repositories

**Status:** Accepted

## Decision

The last-session snapshot uses a versioned JSON schema under `$XDG_STATE_HOME/danksession`, is written atomically, and has mode `0600`. Window titles require explicit opt-in.

## Consequences

Session contents remain local and schema migrations can reject unsupported data instead of silently corrupting a desktop.
