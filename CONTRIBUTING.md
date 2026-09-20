# Contributing

Development happens on `dev`. Promote tested changes to `main` through a pull
request. Releases are tagged from the version in `plugin.json` only after the
plugin, backend, live restore behavior, and Nix package have been validated
together.

## Versioning

Keep `plugin.json` at the current release version during ordinary development.
Packaged QA builds add `dev.<commit>` to that base version in both the staged
manifest and backend (`danksession --version`), so each build remains
identifiable without consuming a release version. Increment the patch for
fixes or the minor for features on the release branch. Keep a short entry in
`CHANGELOG.md` for each iteration.

## Checks

Before opening a pull request, run:

```sh
go test ./...
go build ./cmd/danksession
bash test.sh
nix build 'path:.#default'
```

`QT_QPA_PLATFORM=offscreen quickshell -p tests/qml --no-duplicate` runs
isolated popout layout checks with lightweight visual stubs, without loading
DMS services or touching saved sessions. Expect `POPOUT QA PASSED`; also
check the installed popout in DMS during UI QA.

For an opt-in live scrolling-width and stacked-height test, run:

```sh
DANKSESSION_LIVE_TEST=1 go test ./internal/session -run TestLiveScrollingSizeRestore -v
```

It briefly focuses disposable `foot` windows on an unused workspace and
restores the previous focus afterward. It uses temporary snapshots, never the
user's saved session.

## Design boundaries

- **One plugin repository:** QML, Go backend, schema, Nix package, and Home
  Manager module are released together.
- **Long-running Go backend:** the service captures event-driven and periodic
  snapshots and survives DMS UI restarts.
- **GitHub-first:** GitHub is canonical for branches, pull requests, CI, and
  releases.
- **Safe launches:** only explicit argument arrays are executed; raw process
  command lines are never persisted.
- **Private local state:** snapshots never belong in a configuration
  repository.
- **Hyprland adapter first:** compositor integration is isolated so another
  adapter can be added later.

See [docs/adr](docs/adr/) for the decisions behind these boundaries.
