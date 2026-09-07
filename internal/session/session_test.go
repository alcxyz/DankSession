package session

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/alcxyz/DankSession/internal/hyprland"
)

type fakeCompositor struct {
	desktop    hyprland.Desktop
	dispatches [][2]string
	failAt     int
}

func (f *fakeCompositor) Desktop(context.Context) (hyprland.Desktop, error) {
	return f.desktop, nil
}

func (f *fakeCompositor) Dispatch(_ context.Context, dispatcher, argument string) error {
	f.dispatches = append(f.dispatches, [2]string{dispatcher, argument})
	if f.failAt > 0 && len(f.dispatches) == f.failAt {
		return errors.New("injected dispatch failure")
	}
	return nil
}

func testDesktop() hyprland.Desktop {
	zen := hyprland.Window{
		Address:      "0x1",
		Mapped:       true,
		At:           hyprland.Point{250, 0},
		Size:         hyprland.Point{492, 1000},
		Workspace:    hyprland.WorkspaceRef{ID: 5, Name: "5"},
		Monitor:      1,
		Class:        "zen",
		InitialClass: "zen",
		Title:        "private page title",
	}
	mail := hyprland.Window{
		Address:      "0x2",
		Mapped:       true,
		At:           hyprland.Point{750, 0},
		Size:         hyprland.Point{242, 1000},
		Workspace:    hyprland.WorkspaceRef{ID: 5, Name: "5"},
		Monitor:      1,
		Class:        "thunderbird",
		InitialClass: "thunderbird",
	}
	scratch := hyprland.Window{
		Address:      "0x3",
		Mapped:       true,
		Workspace:    hyprland.WorkspaceRef{ID: 5, Name: "5"},
		Monitor:      1,
		Class:        "dropterm",
		InitialClass: "dropterm",
		Tags:         []string{"scratchpad"},
	}
	return hyprland.Desktop{
		Windows:      []hyprland.Window{mail, scratch, zen},
		Monitors:     []hyprland.Monitor{{ID: 1, Name: "DP-1", Width: 1000, Height: 1000, ActiveWorkspace: hyprland.WorkspaceRef{ID: 5, Name: "5"}}},
		ActiveWindow: zen,
		Workspaces:   []hyprland.Workspace{{ID: 5, TiledLayout: "scrolling"}},
	}
}

func TestBuildSnapshotCapturesScrollingGeometryWithoutTitles(t *testing.T) {
	now := time.Date(2026, 9, 5, 12, 0, 0, 0, time.UTC)
	snapshot := BuildSnapshot(testDesktop(), DefaultConfig(), now)
	if len(snapshot.Windows) != 2 {
		t.Fatalf("got %d windows, want 2", len(snapshot.Windows))
	}
	if snapshot.Windows[0].Class != "zen" || snapshot.Windows[0].Layout.ColumnWidth != 0.5 || !snapshot.Windows[0].Layout.Centered {
		t.Fatalf("unexpected first window: %#v", snapshot.Windows[0])
	}
	if snapshot.Windows[1].Class != "thunderbird" || snapshot.Windows[1].Layout.Column != 1 || snapshot.Windows[1].Layout.ColumnWidth != 0.25 {
		t.Fatalf("unexpected second window: %#v", snapshot.Windows[1])
	}
	if snapshot.Windows[0].Title != "" {
		t.Fatalf("title was captured without opt-in: %q", snapshot.Windows[0].Title)
	}
	if !snapshot.Windows[0].Focused {
		t.Fatal("active window was not recorded")
	}
}

func TestBuildSnapshotCapturesTitlesOnlyWhenEnabled(t *testing.T) {
	cfg := DefaultConfig()
	cfg.CaptureTitles = true
	snapshot := BuildSnapshot(testDesktop(), cfg, time.Now())
	if snapshot.Windows[0].Title != "private page title" {
		t.Fatalf("title not captured after opt-in: %q", snapshot.Windows[0].Title)
	}
}

func TestCaptureWritesPrivateAtomicState(t *testing.T) {
	directory := t.TempDir()
	manager := Manager{
		Compositor: &fakeCompositor{desktop: testDesktop()},
		StatePath:  filepath.Join(directory, "state", "last.json"),
		ConfigPath: filepath.Join(directory, "missing-config.json"),
	}
	if _, err := manager.Capture(context.Background()); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(manager.StatePath)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("state permissions are %o, want 600", info.Mode().Perm())
	}
}

func TestRestoreDryRunPlansTopologyWithoutDispatching(t *testing.T) {
	directory := t.TempDir()
	compositor := &fakeCompositor{desktop: testDesktop()}
	compositor.desktop.Windows = []hyprland.Window{compositor.desktop.Windows[0], compositor.desktop.Windows[2]}
	manager := Manager{
		Compositor: compositor,
		StatePath:  filepath.Join(directory, "last.json"),
		ConfigPath: filepath.Join(directory, "config.json"),
	}
	if _, err := manager.Capture(context.Background()); err != nil {
		t.Fatal(err)
	}
	result, err := manager.Restore(context.Background(), true, true)
	if err != nil {
		t.Fatal(err)
	}
	if result.Matched != 2 || result.Missing != 0 {
		t.Fatalf("unexpected result: %#v", result)
	}
	if len(compositor.dispatches) != 0 {
		t.Fatalf("dry run dispatched actions: %#v", compositor.dispatches)
	}
	foundStage := false
	for _, operation := range result.Operations {
		if operation.Kind == "stage" {
			foundStage = true
		}
	}
	if !foundStage {
		t.Fatal("topology reconstruction was not planned")
	}
}

func TestRestoreRebuildsColumnsAndReturnsFocus(t *testing.T) {
	directory := t.TempDir()
	compositor := &fakeCompositor{desktop: testDesktop()}
	manager := Manager{
		Compositor: compositor,
		StatePath:  filepath.Join(directory, "last.json"),
		ConfigPath: filepath.Join(directory, "config.json"),
	}
	if _, err := manager.Capture(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.Restore(context.Background(), false, true); err != nil {
		t.Fatal(err)
	}
	if len(compositor.dispatches) == 0 {
		t.Fatal("restore dispatched no actions")
	}
	foundMonitorRestore := false
	for _, dispatch := range compositor.dispatches {
		if dispatch == [2]string{"moveworkspacetomonitor", "5 DP-1"} {
			foundMonitorRestore = true
		}
	}
	if !foundMonitorRestore {
		t.Fatalf("single-output workspace ownership was not restored: %#v", compositor.dispatches)
	}
	last := compositor.dispatches[len(compositor.dispatches)-1]
	if last != [2]string{"focuswindow", "address:0x1"} {
		t.Fatalf("last dispatch was %#v, want focus restoration", last)
	}
}
