# DankSession

Application session restoration for [DankMaterialShell](https://github.com/AvengeMedia/DankMaterialShell). DankSession remembers open windows, workspaces, outputs, scrolling-column widths, focused windows, and floating geometry. A single Go binary provides the continuously running backend and the command-line interface used by the DMS widget.

## Status

DankSession is under pre-release QA. It is not ready for unattended restoration yet; the Home Manager module installs its daemon without enabling automatic startup by default.

## Commands

| Command | Description |
|---|---|
| `danksession capture` | Atomically save the current desktop |
| `danksession status` | Print snapshot status as JSON |
| `danksession restore --dry-run` | Preview matching, launches, and placement |
| `danksession restore` | Restore configured applications and window placement |
| `danksession configure` | Update backend preferences while retaining application rules |
| `danksession daemon` | Capture window events and optionally restore after login |

The default state path is `~/.local/state/danksession/last.json`. Files are written atomically with mode `0600`. Window titles are excluded unless explicitly enabled.

At the first daemon start in each compositor session, the incoming snapshot is retained as `last.json.previous`. Automatic restoration is attempted only once per compositor session; restarting the daemon does not repeat it. Shutdown retains the last completed capture. Capture and restore commands are mutually exclusive, including commands started by the widget while the daemon is running.

The widget shows whether the capture daemon is running and provides a **Preview** action that does not move or launch windows. Enabling “Restore after login” sets a backend preference; enable `services.dankSession.autoStart` separately to start the daemon at graphical login.

Restore closes the popout before changing the desktop so it cannot hold keyboard focus away from Hyprland's layout commands. Reopen the widget to inspect the result.

After updating an installed plugin without restarting DMS, rescan it if its manifest changed, then run `dms ipc call plugins reload dankSession` **last**. Reload busts the QML component cache; disabling and enabling alone can leave an older widget running.

## Application rules

DankSession never derives launch commands from `/proc` or saved process command lines. Applications must opt into relaunching through `~/.config/danksession/config.json`:

```json
{
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

Unconfigured windows may have their running instance repositioned, but DankSession cannot relaunch them. Scratchpad-tagged and special-workspace windows are excluded automatically.

Restoration applies current exclusions to older snapshots, skips disconnected outputs, and uses scrolling layout commands only on scrolling workspaces. Named workspaces and scaled/rotated output widths are supported. A failed staging operation attempts to return every staged window and restores the prior focus; recovery failures are reported.

Relaunched applications run in independent systemd user services so stopping DMS or the capture daemon does not terminate them. This requires `systemd-run` (systemd 254 or newer); the Nix package supplies it. Launch arguments are passed literally, and only explicitly configured application commands are started.

## QA limitations

Scrolling column widths are read directly from Hyprland's Lua layout state, including custom mouse-resized widths. Geometry-based width estimation remains a fallback when a window has no available layout state; centering is still inferred from visible geometry. Stacked row heights are restored through Hyprland's tiled resize API, subject to application minimum sizes and available output space. Workspaces containing unsaved tiled windows are not reconstructed. Non-scrolling tiled layouts do not yet support size restoration. Multi-window matching uses application identity and optional titles; the application is responsible for reopening its own documents/tabs. A successful unit test suite is not a substitute for testing a complete logout/login restore on the target compositor. Keep automatic restoration disabled until that test passes.

## Design

- **One plugin repository** — QML, Go backend, schema, Nix package, and Home Manager module are released together.
- **Long-running Go backend** — the service captures event-driven and periodic snapshots and survives DMS UI restarts.
- **GitHub-first** — GitHub is canonical for branches, pull requests, CI, and releases.
- **Safe launches** — only explicit argument arrays are executed; raw process command lines are never persisted.
- **Private local state** — snapshots never belong in a configuration repository.
- **Hyprland adapter first** — compositor integration is isolated so another adapter can be added later.

See [docs/adr](docs/adr/) for the decisions behind these boundaries.

## Development

```sh
go test ./...
go build ./cmd/danksession
bash test.sh
nix build
```

For an opt-in live scrolling-width and stacked-height test, run
`DANKSESSION_LIVE_TEST=1 go test ./internal/session -run TestLiveScrollingSizeRestore -v`.
It briefly focuses disposable `foot` windows on an unused workspace and restores
the previous focus afterward. It uses temporary snapshots, never the user's saved session.

## License

MIT
