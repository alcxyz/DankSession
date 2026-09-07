# Changelog

## 0.3.1 — QA

- Load the new popout by URL so existing DMS sessions can pick it up without a
  shell restart, even when the old plugin directory contents were cached.

## 0.3.0 — QA

- List saved application windows, workspaces, dimensions, and current restore
  eligibility directly in the popout, without exposing titles or launch commands.
- Fit the popout to its content, scroll longer lists, and consolidate saving and
  login restoration status into one line.

## 0.2.1 — QA

- Use one compact icon in horizontal and vertical bars. Keep snapshot counts,
  timestamps, and actions in the popout; show muted paused/stopped state and
  retain error coloring.

## 0.2.0 — QA

- Configure automatic saving and save frequency from plugin settings.
- Add application exclusions manually or from currently open applications,
  preview matches, and edit, disable, or remove rules.
- Restore saved scrolling-column widths and stacked-window heights.
- Use distinct versions for subsequent QA deployments; earlier QA iterations
  all reported 0.1.0.
