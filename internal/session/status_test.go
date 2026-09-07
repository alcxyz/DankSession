package session

import (
	"encoding/json"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/alcxyz/DankSession/internal/hyprland"
)

func TestStatusSavedWindowsUsesSnapshotAndPrivateSummary(t *testing.T) {
	dir := t.TempDir()
	// No compositor is supplied: listing a saved session must work offline and
	// must not substitute current windows for what is actually on disk.
	m := Manager{StatePath: filepath.Join(dir, "last.json"), ConfigPath: filepath.Join(dir, "config.json")}
	cfg := DefaultConfig()
	cfg.Applications = []Application{{ID: "browser", Command: []string{"private-launch-command"}, Match: Match{Class: "^browser-class$"}}}
	if err := m.SaveConfig(cfg); err != nil {
		t.Fatal(err)
	}
	savedAt := time.Date(2026, 9, 7, 12, 0, 0, 0, time.UTC)
	ws2 := hyprland.WorkspaceRef{ID: 2, Name: "work"}
	ws5 := hyprland.WorkspaceRef{ID: 5, Name: "5"}
	snapshot := Snapshot{Schema: SchemaVersion, SavedAt: savedAt, Windows: []Window{
		{Application: "browser", Class: "browser-class", InitialClass: "browser-initial", Workspace: ws5, Size: hyprland.Point{1234, 987}, Title: "private-window-title", InitialTitle: "private-initial-title", Monitor: "private-monitor-identity", Slot: "private-slot", At: hyprland.Point{111, 222}},
		{InitialClass: "terminal", Workspace: ws2, Size: hyprland.Point{800, 600}},
		{Class: "editor", Workspace: ws2, Size: hyprland.Point{900, 700}},
		{InitialClass: "terminal", Workspace: ws2, Size: hyprland.Point{801, 601}},
	}}
	if err := writeJSONAtomic(m.StatePath, snapshot); err != nil {
		t.Fatal(err)
	}
	status, err := m.Status()
	if err != nil {
		t.Fatal(err)
	}
	want := []SavedWindowSummary{
		{Application: "editor", Workspace: ws2, Width: 900, Height: 700, Restore: "placement"},
		{Application: "terminal", Workspace: ws2, Width: 800, Height: 600, Restore: "placement"},
		{Application: "terminal", Workspace: ws2, Width: 801, Height: 601, Restore: "placement"},
		{Application: "browser", Workspace: ws5, Width: 1234, Height: 987, Restore: "relaunch"},
	}
	if !reflect.DeepEqual(status.SavedWindows, want) {
		t.Fatalf("summary = %+v; want %+v", status.SavedWindows, want)
	}
	if !status.Saved || status.Windows != 4 || status.Managed != 1 || status.Workspaces != 2 || !status.SavedAt.Equal(savedAt) {
		t.Fatalf("legacy summary changed: %+v", status)
	}
	data, err := json.Marshal(status)
	if err != nil {
		t.Fatal(err)
	}
	for _, private := range []string{"private-launch-command", "private-window-title", "private-initial-title", "private-monitor-identity", "private-slot", "browser-class", "browser-initial"} {
		if strings.Contains(string(data), private) {
			t.Errorf("status exposed %q", private)
		}
	}
	var rows []map[string]json.RawMessage
	rowData, _ := json.Marshal(status.SavedWindows)
	if err := json.Unmarshal(rowData, &rows); err != nil {
		t.Fatal(err)
	}
	for _, row := range rows {
		if len(row) != 5 {
			t.Fatalf("unexpected summary fields: %s", rowData)
		}
	}
}

func TestStatusNoSnapshotHasEmptySavedWindows(t *testing.T) {
	dir := t.TempDir()
	m := Manager{StatePath: filepath.Join(dir, "last.json"), ConfigPath: filepath.Join(dir, "config.json")}
	status, err := m.Status()
	if err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(status)
	if err != nil {
		t.Fatal(err)
	}
	if status.Saved || status.SavedWindows == nil || len(status.SavedWindows) != 0 || !strings.Contains(string(data), `"savedWindows":[]`) {
		t.Fatalf("missing snapshot status: %s", data)
	}
}

func TestSavedWindowSummaryUsesCurrentRules(t *testing.T) {
	saved := Window{Application: "browser", Class: "zen", InitialClass: "zen", Workspace: hyprland.WorkspaceRef{ID: 5, Name: "5"}, Size: hyprland.Point{800, 600}}
	app := Application{ID: "browser", Command: []string{"zen"}, Match: Match{Class: "^zen$"}}
	for _, tc := range []struct {
		name    string
		apps    []Application
		exclude []Match
		want    string
	}{
		{name: "current rule", apps: []Application{app}, want: "relaunch"},
		{name: "deleted rule", want: "placement"},
		{name: "renamed rule", apps: []Application{{ID: "new-browser", Command: app.Command, Match: app.Match}}, want: "placement"},
		{name: "updated command and match retains explicit ID", apps: []Application{{ID: "browser", Command: []string{"new-browser"}, Match: Match{Class: "^new-browser$"}}}, want: "relaunch"},
		{name: "excluded", apps: []Application{app}, exclude: []Match{{Class: "^zen$"}}, want: "excluded"},
		{name: "disabled exclusion", apps: []Application{app}, exclude: []Match{{Class: "^zen$", Disabled: true}}, want: "relaunch"},
		{name: "unknown historical title conservatively excluded", apps: []Application{app}, exclude: []Match{{Class: "^zen$", Title: "private"}}, want: "excluded"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg := DefaultConfig()
			cfg.Applications, cfg.Exclude = tc.apps, tc.exclude
			rows := summarizeSavedWindows([]Window{saved}, cfg)
			if len(rows) != 1 || rows[0].Restore != tc.want {
				t.Fatalf("summary = %+v; want %q", rows, tc.want)
			}
		})
	}
}

func TestSavedWindowSummaryMarksSkippedWindows(t *testing.T) {
	cfg := DefaultConfig()
	cfg.CaptureUnconfigured = false
	rows := summarizeSavedWindows([]Window{{Workspace: hyprland.WorkspaceRef{ID: 1}}}, cfg)
	if rows[0].Restore != "excluded" || rows[0].Application != "Unknown application" {
		t.Fatalf("unconfigured summary = %+v", rows)
	}
	cfg.CaptureUnconfigured = true
	rows = summarizeSavedWindows([]Window{{Class: "terminal", Workspace: hyprland.WorkspaceRef{ID: -99, Name: "special:test"}}}, cfg)
	if rows[0].Restore != "excluded" {
		t.Fatalf("special workspace summary = %+v", rows)
	}
	if rows := summarizeSavedWindows(nil, cfg); rows == nil || len(rows) != 0 {
		t.Fatalf("empty snapshot summary = %+v", rows)
	}
}
