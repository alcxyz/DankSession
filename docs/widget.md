# The widget

## Bar icon and popout

The bar shows only the session icon: accented while automatic saving is
running, muted while paused or stopped, and red on errors. Click it for window
counts, the last save time, and **Save now**, **Restore**, and **Preview**
actions.

The popout lists the actual saved snapshot, not currently open windows. Each
row shows the application identifier, workspace, saved dimensions, and whether
it can reopen, needs an already-running window (placement only), or is skipped
by current rules. This is not a guarantee of successful restoration; Preview
checks the live desktop. The panel fits short lists and scrolls longer ones.
This summary never includes window titles or launch commands, even when title
capture is enabled.

The widget shows whether the capture daemon is running. **Preview** does not
move or launch windows. **Restore** closes the popout before changing the
desktop so it cannot hold keyboard focus away from Hyprland's layout commands;
reopen the widget to inspect the result.

## Settings

- **Automatic saving** pauses or resumes background capture without stopping
  the daemon. Manual **Save now** remains available while paused.
- **Save frequency** controls periodic safety saves (5 to 120 seconds, default
  15); desktop events can save sooner.
- **Restore after login** sets a backend preference. Enable
  `services.dankSession.autoStart` separately to start the daemon at graphical
  login.

Preference changes are picked up by the running daemon without a restart. A
stopped-service warning explains when automatic saving and login restoration
are unavailable.

## Application exclusions

Use **Add manually** for an exact application identifier, or select an
application from the searchable **Currently open applications** list.
Selection uses the initial application class (falling back to its current
class), not a changing title. Preview shows matching application identifiers,
window counts, and workspaces before confirmation, without exposing window
titles. Rules can be edited, disabled/enabled, or removed with confirmation.

Advanced fields accept Go regular expressions; all nonempty fields must match.
Exclusions prevent capture, relaunch, and repositioning, including restoration
from older snapshots. Editing a rule does not change the desktop or overwrite
the saved snapshot. Stale edits are rejected and refreshed so another edit
cannot redirect a rule index.

Title rules are conservative: snapshots saved without titles skip every
application matching the rule's class fields; a title-only rule skips all
windows whose saved title is unknown. An excluded open window is never treated
as a reason to relaunch its application. Leave title empty for ordinary
application exclusions.

## Reloading after an update

After updating an installed plugin without restarting DMS, rescan it if its
manifest changed, then run `dms ipc call plugins reload dankSession` **last**.
Reload busts the QML component cache; disabling and enabling alone can leave
an older widget running.
