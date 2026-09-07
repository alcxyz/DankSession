package session

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/alcxyz/DankSession/internal/hyprland"
)

func savedManager(t *testing.T) (*Manager, *fakeCompositor) {
	t.Helper()
	dir := t.TempDir()
	f := &fakeCompositor{desktop: testDesktop()}
	f.desktop.Windows = []hyprland.Window{f.desktop.Windows[0], f.desktop.Windows[2]}
	m := &Manager{Compositor: f, StatePath: filepath.Join(dir, "last.json"), ConfigPath: filepath.Join(dir, "config.json")}
	if _, err := m.Capture(context.Background()); err != nil {
		t.Fatal(err)
	}
	return m, f
}

func stackedManager(t *testing.T) (*Manager, *fakeCompositor) {
	t.Helper()
	m, f := savedManager(t)
	first := f.desktop.Windows[1]
	first.At = hyprland.Point{250, 0}
	first.Size = hyprland.Point{492, 300}
	middle := f.desktop.Windows[0]
	middle.At = hyprland.Point{250, 308}
	middle.Size = hyprland.Point{492, 400}
	last := first
	last.Address = "0x4"
	last.Class, last.InitialClass = "third-app", "third-app"
	last.At = hyprland.Point{250, 716}
	last.Size = hyprland.Point{492, 284}
	// Deliberately scrambled input verifies that capture records row order.
	f.desktop.Windows = []hyprland.Window{middle, last, first}
	if _, err := m.Capture(context.Background()); err != nil {
		t.Fatal(err)
	}
	for i := range f.desktop.Windows {
		f.desktop.Windows[i].Size = hyprland.Point{742, 328}
	}
	return m, f
}

func TestRestoreStackedHeightsBottomUpBeforeColumnWidths(t *testing.T) {
	m, f := stackedManager(t)
	result, err := m.Restore(context.Background(), false, true)
	if err != nil {
		t.Fatal(err)
	}
	var heights [][2]string
	widthSeen := false
	stackPlacements := 0
	for i, call := range f.dispatches {
		if call == [2]string{"layoutmsg", "consume_or_expel prev"} {
			stackPlacements++
		}
		if call == [2]string{"layoutmsg", "movewindowto l"} {
			t.Fatal("restore used the obsolete scrolling row-placement command")
		}
		if call[0] == "layoutmsg" && strings.HasPrefix(call[1], "colresize ") {
			widthSeen = true
		}
		if call[0] != "resizewindowpixel" {
			continue
		}
		if widthSeen {
			t.Fatalf("row resize after column width: %v", f.dispatches)
		}
		heights = append(heights, call)
		_, address, _ := strings.Cut(call[1], ",")
		if i == 0 || f.dispatches[i-1] != [2]string{"focuswindow", address} {
			t.Fatalf("height resize did not target focused row: %v", f.dispatches)
		}
	}
	want := [][2]string{{"resizewindowpixel", "exact 492 284,address:0x4"}, {"resizewindowpixel", "exact 492 400,address:0x2"}}
	if len(heights) != len(want) || heights[0] != want[0] || heights[1] != want[1] || !widthSeen {
		t.Fatalf("height order %v, want %v then column widths", heights, want)
	}
	if stackPlacements != 2 {
		t.Fatalf("got %d row placements, want 2", stackPlacements)
	}
	var slots []string
	for _, operation := range result.Operations {
		if operation.Kind == "row-height" {
			slots = append(slots, operation.Slot)
		}
	}
	if len(slots) != 2 || slots[0] != "third-app#1" || slots[1] != "thunderbird#1" {
		t.Fatalf("unexpected row height operations: %v", slots)
	}
}

func TestRestoreStackedHeightsDryRunDoesNotDispatch(t *testing.T) {
	m, f := stackedManager(t)
	result, err := m.Restore(context.Background(), true, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(f.dispatches) != 0 {
		t.Fatalf("dry-run mutated desktop: %v", f.dispatches)
	}
	heights := 0
	for _, operation := range result.Operations {
		if operation.Kind == "row-height" {
			heights++
		}
	}
	if heights != 2 {
		t.Fatalf("planned %d row resizes, want 2", heights)
	}
}

func TestCaptureAndRestoreShareCrossProcessLock(t *testing.T) {
	m, _ := savedManager(t)
	other := *m
	unlock, err := m.LockOperation()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := other.Capture(context.Background()); !errors.Is(err, ErrBusy) {
		t.Fatalf("capture: %v", err)
	}
	if _, err := other.Restore(context.Background(), false, true); !errors.Is(err, ErrBusy) {
		t.Fatalf("restore: %v", err)
	}
	unlock()
	if _, err := other.Capture(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestPrepareSessionPreservesSnapshotAndRunsOnce(t *testing.T) {
	m, f := savedManager(t)
	before, _ := os.ReadFile(m.StatePath)
	first, err := m.PrepareSession("session-one")
	if err != nil || !first {
		t.Fatalf("first=%v err=%v", first, err)
	}
	f.desktop.Windows = nil
	if _, err := m.Capture(context.Background()); err != nil {
		t.Fatal(err)
	}
	first, err = m.PrepareSession("session-one")
	if err != nil || first {
		t.Fatalf("restart: first=%v err=%v", first, err)
	}
	backup, err := os.ReadFile(m.StatePath + ".previous")
	if err != nil || string(backup) != string(before) {
		t.Fatalf("incoming snapshot lost: %v", err)
	}
}

func TestCaptureRejectsDisconnectedOrCancelledDesktop(t *testing.T) {
	m, f := savedManager(t)
	before, _ := os.ReadFile(m.StatePath)
	f.desktop.Monitors = nil
	if _, err := m.Capture(context.Background()); err == nil {
		t.Fatal("captured disconnected desktop")
	}
	f.desktop = testDesktop()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := m.Capture(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("capture: %v", err)
	}
	after, _ := os.ReadFile(m.StatePath)
	if string(before) != string(after) {
		t.Fatal("saved desktop overwritten")
	}
}

func TestRestoreHonorsNewExclusionsWithoutDispatch(t *testing.T) {
	m, f := savedManager(t)
	cfg := DefaultConfig()
	cfg.Exclude = []Match{{Class: ".*"}}
	if err := m.SaveConfig(cfg); err != nil {
		t.Fatal(err)
	}
	result, err := m.Restore(context.Background(), false, true)
	if err != nil {
		t.Fatal(err)
	}
	if result.Matched != 0 || len(f.dispatches) != 0 {
		t.Fatalf("excluded desktop changed: %+v, %+v", result, f.dispatches)
	}
}

func TestMatchingSkipsSpecialAndScratchpadWindows(t *testing.T) {
	desktop := testDesktop()
	saved := BuildSnapshot(desktop, DefaultConfig(), time.Now())
	for i := range desktop.Windows {
		desktop.Windows[i].Tags = []string{"scratchpad"}
	}
	matched, _ := matchWindows(saved.Windows, desktop.Windows, DefaultConfig())
	if len(matched) != 0 {
		t.Fatal("matched scratchpad windows")
	}
	for i := range desktop.Windows {
		desktop.Windows[i].Tags = nil
		desktop.Windows[i].Workspace = hyprland.WorkspaceRef{ID: -98, Name: "special:game"}
	}
	matched, _ = matchWindows(saved.Windows, desktop.Windows, DefaultConfig())
	if len(matched) != 0 {
		t.Fatal("matched special workspace windows")
	}
}

func TestRestoreRecoversStagedWindowsOnDispatchFailure(t *testing.T) {
	m, f := savedManager(t)
	// Workspace monitor move, first stage, then fail staging the second window.
	f.failAt = 3
	if _, err := m.Restore(context.Background(), false, true); err == nil {
		t.Fatal("expected failure")
	}
	recovered := map[string]bool{}
	for _, call := range f.dispatches[f.failAt:] {
		if call[0] == "movetoworkspacesilent" {
			recovered[call[1]] = true
		}
	}
	if !recovered["5,address:0x1"] || !recovered["5,address:0x2"] {
		t.Fatalf("windows stranded: %+v", f.dispatches)
	}
	if last := f.dispatches[len(f.dispatches)-1]; last != [2]string{"focuswindow", "address:0x1"} {
		t.Fatalf("focus not recovered: %v", last)
	}
}

func TestRestoreSkipsTopologyWithUnsavedTiledWindows(t *testing.T) {
	m, f := savedManager(t)
	f.desktop.Windows = append(f.desktop.Windows, hyprland.Window{Address: "0x4", Mapped: true, Class: "new-app", Workspace: hyprland.WorkspaceRef{ID: 5}})
	if _, err := m.Restore(context.Background(), false, true); err != nil {
		t.Fatal(err)
	}
	for _, call := range f.dispatches {
		if strings.Contains(call[1], "staging") {
			t.Fatalf("unsaved workspace rebuilt: %v", call)
		}
	}
}

func TestRestoreSkipsAbsentOutputs(t *testing.T) {
	m, f := savedManager(t)
	f.desktop.Monitors[0].Name = "OTHER"
	if _, err := m.Restore(context.Background(), false, true); err != nil {
		t.Fatal(err)
	}
	for _, call := range f.dispatches {
		if strings.Contains(call[1], "DP-1") {
			t.Fatalf("absent output addressed: %v", call)
		}
	}
}

func TestNamedWorkspacesAndScaledGeometry(t *testing.T) {
	m, f := savedManager(t)
	f.desktop.Monitors[0].Width = 2000
	f.desktop.Monitors[0].Scale = 2
	f.desktop.Workspaces = []hyprland.Workspace{{ID: -1337, TiledLayout: "scrolling"}}
	for i := range f.desktop.Windows {
		f.desktop.Windows[i].Workspace = hyprland.WorkspaceRef{ID: -1337, Name: "work"}
	}
	snapshot, err := m.Capture(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshot.Windows) != 2 || snapshot.Windows[0].Layout.ColumnWidth != 0.5 || !snapshot.Windows[0].Layout.Centered {
		t.Fatalf("wrong scaled/named snapshot: %+v", snapshot)
	}
	f.desktop.Windows[1].Workspace = hyprland.WorkspaceRef{ID: 2, Name: "2"}
	if _, err := m.Restore(context.Background(), false, true); err != nil {
		t.Fatal(err)
	}
	found := false
	for _, call := range f.dispatches {
		if call == [2]string{"movetoworkspacesilent", "name:work,address:0x1"} {
			found = true
		}
	}
	if !found {
		t.Fatalf("named workspace not restored: %v", f.dispatches)
	}
}

func TestConfigRejectsDuplicateIDsAndEmptyExecutables(t *testing.T) {
	m, _ := savedManager(t)
	for _, apps := range [][]Application{
		{{ID: "browser", Command: []string{"browser"}, Match: Match{Class: "browser"}}, {ID: "browser", Command: []string{"other"}, Match: Match{Class: "other"}}},
		{{ID: "browser", Command: []string{""}, Match: Match{Class: "browser"}}},
	} {
		cfg := DefaultConfig()
		cfg.Applications = apps
		if err := m.SaveConfig(cfg); err != nil {
			t.Fatal(err)
		}
		if _, err := m.LoadConfig(); err == nil {
			t.Fatalf("accepted invalid apps: %+v", apps)
		}
	}
}

func TestRestoreDoesNotSendScrollingCommandsToOtherLayouts(t *testing.T) {
	m, f := savedManager(t)
	f.desktop.Workspaces[0].TiledLayout = "dwindle"
	if _, err := m.Restore(context.Background(), false, true); err != nil {
		t.Fatal(err)
	}
	for _, call := range f.dispatches {
		if call[0] == "layoutmsg" || strings.Contains(call[1], "staging") {
			t.Fatalf("wrong layout modified: %v", call)
		}
	}
	snapshot := BuildSnapshot(f.desktop, DefaultConfig(), time.Now())
	for _, window := range snapshot.Windows {
		if window.Layout != nil {
			t.Fatal("recorded scrolling layout for dwindle")
		}
	}
}

func TestDaemonStatusTracksTheLockOwner(t *testing.T) {
	m, _ := savedManager(t)
	unlock, err := m.LockDaemon()
	if err != nil {
		t.Fatal(err)
	}
	status, err := m.Status()
	if err != nil || !status.DaemonRunning {
		t.Fatalf("status=%+v err=%v", status, err)
	}
	unlock()
	status, err = m.Status()
	if err != nil || status.DaemonRunning {
		t.Fatalf("status=%+v err=%v", status, err)
	}
}

func TestNumberedWorkspaceRetainsIDWhenRenamed(t *testing.T) {
	if got := workspaceSelector(hyprland.WorkspaceRef{ID: 8, Name: "games"}); got != "8" {
		t.Fatalf("got %q", got)
	}
}
