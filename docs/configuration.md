# Configuration and restore behavior

## Application rules

DankSession never derives launch commands from `/proc` or saved process
command lines. Applications must opt into relaunching through
`~/.config/danksession/config.json`:

```json
{
  "autoCapture": true,
  "autoRestore": false,
  "captureUnconfigured": true,
  "captureTitles": false,
  "debounceMs": 750,
  "captureIntervalSeconds": 15,
  "restoreTimeoutSeconds": 20,
  "applications": [
    {
      "id": "browser",
      "command": ["my-browser"],
      "match": {"initialClass": "^my-browser$"}
    },
    {
      "id": "mail",
      "command": ["my-mail-client"],
      "match": {"initialClass": "^my-mail-client$"}
    }
  ],
  "exclude": [
    {"class": "^(pinentry|polkit-.*)$"}
  ]
}
```

Unconfigured windows may have their running instance repositioned, but
DankSession cannot relaunch them. Scratchpad-tagged and special-workspace
windows are excluded automatically. Exclusions can also be managed from the
widget; see [Application exclusions](widget.md#application-exclusions).

Relaunched applications run in independent systemd user services so stopping
DMS or the capture daemon does not terminate them. This requires `systemd-run`
(systemd 254 or newer); the Nix package supplies it. Launch arguments are
passed literally, and only explicitly configured application commands are
started. See [ADR-003](adr/ADR-003-explicit-launch-rules.md).

## State files

The default state path is `~/.local/state/danksession/last.json`. Files are
written atomically with mode `0600`. Window titles are excluded unless
explicitly enabled.

At the first daemon start in each compositor session, the incoming snapshot is
retained as `last.json.previous`. Automatic restoration is attempted only once
per compositor session; restarting the daemon does not repeat it. Shutdown
retains the last completed capture. Capture and restore commands are mutually
exclusive, including commands started by the widget while the daemon is
running. See [ADR-004](adr/ADR-004-private-versioned-state.md).

## How restoration works

Restoration applies current exclusions to older snapshots, skips disconnected
outputs, and uses scrolling layout commands only on scrolling workspaces.
Named workspaces and scaled/rotated output widths are supported. A failed
staging operation attempts to return every staged window and restores the
prior focus; recovery failures are reported.

## Restore limitations

Scrolling column widths are read directly from Hyprland's Lua layout state,
including custom mouse-resized widths. Geometry-based width estimation remains
a fallback when a window has no available layout state; centering is still
inferred from visible geometry. Stacked row heights are restored through
Hyprland's tiled resize API, subject to application minimum sizes and
available output space.

- Workspaces containing unsaved tiled windows are not reconstructed.
- Non-scrolling tiled layouts do not yet support size restoration.
- Multi-window matching uses application identity and optional titles; the
  application is responsible for reopening its own documents and tabs.
- Other compositors are not supported yet.

A successful unit test suite is not a substitute for testing a complete
logout/login restore on the target compositor. Keep automatic restoration
disabled until that test passes.
