#!/usr/bin/env bash
set -euo pipefail

version="$(jq -r .version plugin.json)"
[[ "$version" =~ ^[0-9]+\.[0-9]+\.[0-9]+$ ]]

go test ./...
go vet ./...
temporary="$(mktemp -d)"
trap 'rm -rf "$temporary"' EXIT
test_binary="$temporary/danksession"
go build -ldflags "-X main.version=$version" -o "$test_binary" ./cmd/danksession

[[ "$("$test_binary" --version)" == "$version" ]]
"$test_binary" --help | rg -q 'restore'
"$test_binary" --help | rg -q 'daemon'
"$test_binary" --help | rg -q 'exclusions'

jq -e '
  .id == "dankSession"
  and .type == "widget"
  and .component == "./SessionWidget.qml"
  and .settings == "./SessionSettings.qml"
  and (.permissions | index("process")) != null
' plugin.json >/dev/null

rg -q 'pluginId: "dankSession"' SessionWidget.qml
rg -q 'pluginId: "dankSession"' SessionSettings.qml
rg -q 'ExclusionEditor' SessionSettings.qml
rg -q '"danksession", "exclusions", "preview"' ExclusionEditor.qml
rg -q 'danksession daemon' nix/home-manager.nix

test ! -e go.sum

export DANKSESSION_STATE="$temporary/state/last.json"
export DANKSESSION_CONFIG="$temporary/config/config.json"

"$test_binary" configure --auto-restore=false --capture-titles=false >/dev/null
[[ "$(stat -c %a "$DANKSESSION_CONFIG")" == 600 ]]
"$test_binary" status | jq -e '.saved == false and .autoRestore == false and .autoCapture == true' >/dev/null
"$test_binary" configure --auto-capture=false --capture-interval=25 >/dev/null
"$test_binary" status | jq -e '.autoCapture == false and .captureIntervalSeconds == 25' >/dev/null

echo "DankSession checks passed"
