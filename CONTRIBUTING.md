# Contributing

Development happens on `dev`. Promote tested changes to `main` through a pull request. Releases are tagged from the version in `plugin.json` only after the plugin, backend, live restore behavior, and Nix package have been validated together.

Before opening a pull request, run:

```sh
go test ./...
bash test.sh
nix build
```
