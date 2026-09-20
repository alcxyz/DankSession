# Command-line reference

The `danksession` binary provides both the background daemon and the commands
the widget calls.

| Command | Description |
|---|---|
| `danksession capture` | Atomically save the current desktop |
| `danksession status` | Print snapshot status as JSON |
| `danksession restore --dry-run` | Preview matching, launches, and placement |
| `danksession restore` | Restore configured applications and window placement |
| `danksession configure` | Update backend preferences while retaining application rules |
| `danksession exclusions list` | List exclusion rules and currently open application identifiers |
| `danksession exclusions preview` | Read a JSON matcher from stdin and preview matching open windows |
| `danksession exclusions update` | Read a revision-checked exclusion edit from stdin |
| `danksession daemon` | Capture window events and optionally restore after login |
| `danksession --version` | Print the backend version (matches the packaged `plugin.json`) |

Capture and restore commands are mutually exclusive, including commands
started by the widget while the daemon is running. Check `danksession status`
for `daemonRunning: true` after installing the service.

Where files live and how restoration behaves is covered in
[Configuration and restore behavior](configuration.md).
