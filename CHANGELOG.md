# Changelog

All notable changes to this plugin are documented here. The format follows
Keep a Changelog, and the release workflow publishes each version's section
as its GitHub release notes.

## [Unreleased]

## [0.3.6] - 2026-09-20

- Reorganized the README to keep what the plugin is, requirements, install, a first-run checklist, and a table of guides; widget behavior, application rules, restore behavior and limitations, and the command table moved into `docs/widget.md`, `docs/configuration.md`, and `docs/cli.md`, with the development and design notes folded into `CONTRIBUTING.md`.
- Development builds now stamp an identifiable `X.Y.Z-dev.<commit>` version (`.dirty` for uncommitted changes) via `scripts/package.py` or the Nix `default.nix`, with the matching version in the helper, so a checkout install can be told apart from a tagged release. Release packaging still requires a clean checkout at the exact `vX.Y.Z` tag.

## [0.3.5] - 2026-09-07

First public release after local QA.

- Automatic and manual session saving, opt-in login restoration, and explicit application relaunch rules.
- Restore workspaces, outputs, floating geometry, scrolling-column widths, and stacked-window heights, with documented layout limitations.
- Configurable save frequency and application exclusions, including selection from open applications.
- Compact bar icon and a saved-window list with workspaces, dimensions, and restore eligibility.
- Add the listing screenshot and Nix/manual installation instructions.

### Pre-release QA builds (0.2.0 to 0.3.4)

These versions were deployed locally for QA and were never published.

- 0.3.4: keep the popout as a tested inline component so hot updates work even when Qt retains an old directory listing, and avoid loading a newly added sibling file.
- 0.3.3: refresh the nested popout component's cache when updating the plugin and test its dynamic loader as well as the content layout.
- 0.3.2: measure the dynamically loaded popout through its wrapper item, keeping compatibility with Qt's read-only Loader implicit dimensions.
- 0.3.1: load the new popout by URL so existing DMS sessions can pick it up without a shell restart, even when the old plugin directory contents were cached.
- 0.3.0: list saved application windows, workspaces, dimensions, and current restore eligibility directly in the popout, without exposing titles or launch commands; fit the popout to its content, scroll longer lists, and consolidate saving and login restoration status into one line.
- 0.2.1: use one compact icon in horizontal and vertical bars; keep snapshot counts, timestamps, and actions in the popout; show muted paused/stopped state and retain error coloring.
- 0.2.0: configure automatic saving and save frequency from plugin settings; add application exclusions manually or from currently open applications, preview matches, and edit, disable, or remove rules; restore saved scrolling-column widths and stacked-window heights. Earlier QA iterations all reported 0.1.0.
