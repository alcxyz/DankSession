# DankSession

Application session restoration for [DankMaterialShell](https://github.com/AvengeMedia/DankMaterialShell). DankSession remembers open windows, workspaces, outputs, scrolling-column widths, focused windows, and floating geometry. A single Go binary provides the continuously running backend and the command-line interface used by the DMS widget.

## Status

DankSession is under local development. It is not published or ready for unattended restoration yet.

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
