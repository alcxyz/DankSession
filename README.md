# DankSession

Application session restoration for
[DankMaterialShell](https://github.com/AvengeMedia/DankMaterialShell) on
Hyprland. DankSession remembers open windows, workspaces, outputs,
scrolling-column widths, focused windows, and floating geometry. A single Go
binary provides the background service and the commands the DMS widget uses.

![DankSession showing saved windows, workspaces, and restore actions](assets/screenshot.png)

## What you get

- **A saved snapshot of your desktop**, captured on window events and on a
  timer by a service that survives DMS restarts.
- **Preview before you act.** The widget shows what a restore would launch,
  move, or skip without touching the desktop.
- **Restore on demand or after login.** Configured applications relaunch in
  their own systemd user services; already-running windows are placed back.
- **Scrolling-layout aware.** Column widths and stacked heights are read from
  Hyprland's layout state, not guessed.
- **Explicit and private.** Only applications you list can be relaunched,
  launch commands are never derived from running processes, and snapshots
  stay in local state with owner-only permissions.

Automatic saving is on by default; automatic restoration is opt-in.
Applications remain responsible for restoring their own documents and tabs.
The similarly named
[DMS Sessionizer](https://github.com/leonardofranco01/dms-sessionizer) handles
tmux project switching instead of desktop windows.

## Requirements

Hyprland, DankMaterialShell, `hyprctl`, and the `danksession` backend.
Relaunching applications needs systemd 254 or newer. Size restoration targets
Hyprland's Lua scrolling layout; other compositors are not supported yet.

## Install

Installing the DMS widget alone does not install or start the backend.

### Nix / Home Manager

```nix
inputs.danksession.url = "github:alcxyz/DankSession/v0.3.5";
```

```nix
{ inputs, pkgs, ... }: {
  imports = [ inputs.danksession.homeManagerModules.default ];

  services.dankSession = {
    enable = true;
    package = inputs.danksession.packages.${pkgs.stdenv.hostPlatform.system}.release;
    autoStart = true;
  };

  # Use the plugin files staged beside the same helper build.
  programs.dank-material-shell.plugins.dankSession = {
    enable = true;
    src = "${inputs.danksession.packages.${pkgs.stdenv.hostPlatform.system}.release}/share/dms-plugins/DankSession";
  };
}
```

The service starts with the next graphical session. `autoStart` defaults to
`false` so installing the module does not silently enable restoration.

### Manual

Package the backend and widget together (Go 1.24 or newer and Python 3):

```sh
git clone --branch v0.3.5 https://github.com/alcxyz/DankSession.git
cd DankSession
python3 scripts/package.py --release --output dist/release
install -Dm755 dist/release/bin/danksession "$HOME/.local/bin/danksession"
mkdir -p "$HOME/.config/DankMaterialShell/plugins/DankSession"
cp -R dist/release/share/dms-plugins/DankSession/. \
  "$HOME/.config/DankMaterialShell/plugins/DankSession/"
```

Create `~/.config/systemd/user/danksession.service`:

```ini
[Unit]
Description=Capture and restore the DankSession desktop state
PartOf=graphical-session.target
After=graphical-session.target

[Service]
ExecStart=%h/.local/bin/danksession daemon
Restart=on-failure
RestartSec=2

[Install]
WantedBy=graphical-session.target
```

Then run `systemctl --user daemon-reload` and
`systemctl --user enable --now danksession.service`. Your graphical session
must activate `graphical-session.target` and provide Hyprland's environment
to the systemd user manager. Ensure `~/.local/bin` is in DMS's `PATH`.

## First run

1. Enable DankSession in DMS plugin settings and add it to your bar.
2. Check `danksession status` reports `daemonRunning: true`.
3. List the applications you want relaunched in
   `~/.config/danksession/config.json`
   (see [Application rules](docs/configuration.md#application-rules)).
4. Click **Preview** in the widget and confirm it launches and places what
   you expect.
5. Only then enable **Restore after login** in plugin settings, and test a
   full logout/login on your compositor.

## Learn more

| Topic | Read |
|---|---|
| Bar icon, popout, settings, application exclusions, reloading after updates | [docs/widget.md](docs/widget.md) |
| Application rules, state files, how restoration works, restore limitations | [docs/configuration.md](docs/configuration.md) |
| Command-line reference | [docs/cli.md](docs/cli.md) |
| Design decisions | [docs/adr/README.md](docs/adr/README.md) |
| Development, versioning, and QA checks | [CONTRIBUTING.md](CONTRIBUTING.md) |
| Changes per version | [CHANGELOG.md](CHANGELOG.md) |

## License

[MIT](LICENSE)

## Build identity

The tracked `plugin.json` remains a release version. Development packages stamp
`X.Y.Z-dev.<commit>` (plus `.dirty` for local changes) into both the helper and
the installed manifest. Build them with:

```sh
python3 scripts/package.py --output dist/dev
```

Install `dist/dev/bin/danksession` and use
`dist/dev/share/dms-plugins/DankSession` as the DMS plugin directory. In Nix,
use `packages.<system>.default` and its `share/dms-plugins/DankSession`
subdirectory, passing the source revision when using `callPackage`.

Release packages use the stable version: manual `--release` requires a clean
checkout at the manifest's `vX.Y.Z` tag; use `#release` with a published tag for
Nix release builds. Source-only Nix imports use a public-source fingerprint;
manual archives without Git metadata are labelled `dev.unknown`. Direct
`go build` identifies the helper's commit but does not stage a DMS manifest.
