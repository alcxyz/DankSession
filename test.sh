#!/usr/bin/env bash
set -euo pipefail

version="$(jq -r .version plugin.json)"
[[ "$version" =~ ^[0-9]+\.[0-9]+\.[0-9]+$ ]]

go test ./...
go vet ./...
go build -ldflags "-X main.version=$version" -o danksession ./cmd/danksession
trap 'rm -f danksession' EXIT

[[ "$(./danksession --version)" == "$version" ]]
./danksession --help | grep -q 'restore'
./danksession --help | grep -q 'daemon'

jq -e '
  .id == "dankSession"
  and .type == "widget"
  and .component == "./SessionWidget.qml"
  and .settings == "./SessionSettings.qml"
  and (.permissions | index("process")) != null
' plugin.json >/dev/null

grep -q 'pluginId: "dankSession"' SessionWidget.qml
grep -q 'pluginId: "dankSession"' SessionSettings.qml
grep -q 'danksession daemon' nix/home-manager.nix

test ! -e go.sum

temporary="$(mktemp -d)"
trap 'rm -rf "$temporary"; rm -f danksession' EXIT
export DANKSESSION_STATE="$temporary/state/last.json"
export DANKSESSION_CONFIG="$temporary/config/config.json"

./danksession configure --auto-restore=false --capture-titles=false >/dev/null
[[ "$(stat -c %a "$DANKSESSION_CONFIG")" == 600 ]]
./danksession status | jq -e '.saved == false and .autoRestore == false' >/dev/null

echo "DankSession checks passed"
