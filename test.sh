#!/usr/bin/env bash
set -euo pipefail

# The shared CI runner does not necessarily provide ripgrep.
has_match() {
  if command -v rg >/dev/null 2>&1; then
    rg -q "$@"
  else
    grep -qE "$@"
  fi
}

version="$(jq -r .version plugin.json)"
[[ "$version" =~ ^[0-9]+\.[0-9]+\.[0-9]+$ ]]

go test ./...
go vet ./...
temporary="$(mktemp -d)"
trap 'rm -rf "$temporary"' EXIT
test_binary="$temporary/danksession"
go build -ldflags "-X main.version=$version" -o "$test_binary" ./cmd/danksession

[[ "$("$test_binary" --version)" == "$version" ]]
"$test_binary" --help | has_match 'restore'
"$test_binary" --help | has_match 'daemon'
"$test_binary" --help | has_match 'exclusions'

jq -e '
  .id == "dankSession"
  and .type == "widget"
  and .component == "./SessionWidget.qml"
  and .settings == "./SessionSettings.qml"
  and (.permissions | index("process")) != null
' plugin.json >/dev/null

has_match 'pluginId: "dankSession"' SessionWidget.qml
has_match 'horizontalBarPill: compactBarIcon' SessionWidget.qml
has_match 'verticalBarPill: compactBarIcon' SessionWidget.qml
has_match 'SessionPopout' SessionWidget.qml
has_match 'savedWindows' SessionPopout.qml
has_match 'pluginId: "dankSession"' SessionSettings.qml
has_match 'ExclusionEditor' SessionSettings.qml
has_match '"danksession", "exclusions", "preview"' ExclusionEditor.qml
has_match 'danksession daemon' nix/home-manager.nix

test ! -e go.sum

export DANKSESSION_STATE="$temporary/state/last.json"
export DANKSESSION_CONFIG="$temporary/config/config.json"

"$test_binary" configure --auto-restore=false --capture-titles=false >/dev/null
[[ "$(stat -c %a "$DANKSESSION_CONFIG")" == 600 ]]
"$test_binary" status | jq -e '.saved == false and .autoRestore == false and .autoCapture == true' >/dev/null
"$test_binary" configure --auto-capture=false --capture-interval=25 >/dev/null
"$test_binary" status | jq -e '.autoCapture == false and .captureIntervalSeconds == 25' >/dev/null

if command -v quickshell >/dev/null 2>&1; then
  QT_QPA_PLATFORM=offscreen quickshell -p tests/qml --no-duplicate >"$temporary/qml.log" 2>&1 || {
    cat "$temporary/qml.log"
    exit 1
  }
  if ! has_match 'POPOUT QA PASSED' "$temporary/qml.log" || has_match 'ERROR|FAIL' "$temporary/qml.log"; then
    cat "$temporary/qml.log"
    exit 1
  fi
else
  echo "Quickshell unavailable; skipping optional offscreen popout layout checks"
fi

echo "DankSession checks passed"
