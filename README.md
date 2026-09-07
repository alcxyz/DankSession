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

Column widths and centering are inferred from visible geometry, including an approximate gap allowance. Workspaces containing unsaved tiled windows are not reconstructed. Multi-window matching uses application identity and optional titles; the application is responsible for reopening its own documents/tabs. A successful unit test suite is not a substitute for testing a complete logout/login restore on the target compositor. Keep automatic restoration disabled until that test passes.

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

## License

MIT
