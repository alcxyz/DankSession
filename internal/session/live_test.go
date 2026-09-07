package session

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/alcxyz/DankSession/internal/hyprland"
)

// Opt-in only: creates one disposable terminal on an unused workspace. Never
// reads/writes the user's snapshot and never launches a configured application.
func TestLiveScrollingSizeRestore(t *testing.T) {
	if os.Getenv("DANKSESSION_LIVE_TEST") != "1" {
		t.Skip("set DANKSESSION_LIVE_TEST=1 inside Hyprland with foot installed")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	client := hyprland.NewClient()
	original, err := client.Desktop(ctx)
	if err != nil {
		t.Fatal(err)
	}
	workspace := 9900
	for _, current := range original.Workspaces {
		if current.ID >= workspace {
			workspace = current.ID + 1
		}
	}
	appID := fmt.Sprintf("danksession-qa-%d", os.Getpid())
	command := fmt.Sprintf("[workspace %d silent] foot --app-id=%s --hold true", workspace, appID)
	out, err := exec.CommandContext(ctx, "hyprctl", "dispatch", "hl.dsp.exec_cmd("+strconv.Quote(command)+")").CombinedOutput()
	if err != nil {
		t.Fatalf("create test terminal: %v: %s", err, out)
	}
	var address string
	var secondAddress string
	defer func() {
		cleanup, done := context.WithTimeout(context.Background(), 3*time.Second)
		defer done()
		if secondAddress != "" {
			exec.CommandContext(cleanup, "hyprctl", "dispatch", "hl.dsp.window.close({window="+strconv.Quote("address:"+secondAddress)+"})").Run()
		}
		if address != "" {
			exec.CommandContext(cleanup, "hyprctl", "dispatch", "hl.dsp.window.close({window="+strconv.Quote("address:"+address)+"})").Run()
		}
		if original.ActiveWindow.Address != "" {
			client.Dispatch(cleanup, "focuswindow", "address:"+original.ActiveWindow.Address)
		}
	}()
	for address == "" && ctx.Err() == nil {
		desktop, err := client.Desktop(ctx)
		if err != nil {
			t.Fatal(err)
		}
		for _, window := range desktop.Windows {
			if window.InitialClass == appID && window.Workspace.ID == workspace {
				address = window.Address
			}
		}
		if address == "" {
			time.Sleep(25 * time.Millisecond)
		}
	}
	if address == "" {
		t.Fatal("test terminal did not appear on the isolated workspace")
	}
	resize := func(width string) {
		t.Helper()
		if err := client.Dispatch(ctx, "focuswindow", "address:"+address); err != nil {
			t.Fatal(err)
		}
		desktop, err := client.Desktop(ctx)
		if err != nil || desktop.ActiveWindow.Address != address {
			t.Fatalf("test terminal did not receive focus; refusing resize: %v", err)
		}
		if err := client.Dispatch(ctx, "layoutmsg", "colresize "+width); err != nil {
			t.Fatal(err)
		}
	}
	resize("0.371239")
	dir := t.TempDir()
	m := &Manager{Compositor: client, StatePath: filepath.Join(dir, "last.json"), ConfigPath: filepath.Join(dir, "config.json")}
	cfg := DefaultConfig()
	cfg.CaptureUnconfigured = false
	cfg.Applications = []Application{{ID: "qa", Command: []string{"false"}, Match: Match{InitialClass: "^" + appID + "$"}}}
	if err := m.SaveConfig(cfg); err != nil {
		t.Fatal(err)
	}
	snapshot, err := m.Capture(ctx)
	if err != nil || len(snapshot.Windows) != 1 || snapshot.Windows[0].Layout == nil {
		t.Fatalf("capture isolated scrolling window: %v", err)
	}
	// Do not restore the user's other active workspaces as part of this test.
	snapshot.Monitors = nil
	data, err := json.Marshal(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(m.StatePath, data, 0o600); err != nil {
		t.Fatal(err)
	}
	resize("0.62")
	if _, err := m.Restore(ctx, false, true); err != nil {
		t.Fatal(err)
	}
	restored, err := client.Desktop(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, window := range restored.Windows {
		if window.Address != address {
			continue
		}
		want := snapshot.Windows[0].Layout.ColumnWidth
		if math.Abs(window.ColumnWidth-want) > 0.000001 || window.Workspace.ID != workspace {
			t.Fatalf("restore got width=%v workspace=%d; want width=%v workspace=%d", window.ColumnWidth, window.Workspace.ID, want, workspace)
		}
		t.Logf("restored custom column width %.6f after resizing to 0.62", window.ColumnWidth)
		if abs(window.Size[0]-snapshot.Windows[0].Size[0]) > 1 {
			t.Fatalf("pixel width got %d, want %d", window.Size[0], snapshot.Windows[0].Size[0])
		}
	}
	// Add a second window to exercise both topology and unequal row heights.
	if out, err := exec.CommandContext(ctx, "hyprctl", "dispatch", "hl.dsp.exec_cmd("+strconv.Quote(command)+")").CombinedOutput(); err != nil {
		t.Fatalf("create second terminal: %v: %s", err, out)
	}
	for secondAddress == "" && ctx.Err() == nil {
		desktop, err := client.Desktop(ctx)
		if err != nil {
			t.Fatal(err)
		}
		for _, window := range desktop.Windows {
			if window.InitialClass == appID && window.Address != address {
				secondAddress = window.Address
			}
		}
		if secondAddress == "" {
			time.Sleep(25 * time.Millisecond)
		}
	}
	if secondAddress == "" {
		t.Fatal("second terminal did not appear")
	}
	if err := client.Dispatch(ctx, "focuswindow", "address:"+secondAddress); err != nil {
		t.Fatal(err)
	}
	if err := client.Dispatch(ctx, "layoutmsg", "consume_or_expel prev"); err != nil {
		t.Fatal(err)
	}
	resize("0.371239")
	if err := client.Dispatch(ctx, "resizewindowpixel", "exact 940 400,address:"+address); err != nil {
		t.Fatal(err)
	}
	time.Sleep(100 * time.Millisecond)
	snapshot, err = m.Capture(ctx)
	if err != nil || len(snapshot.Windows) != 2 {
		t.Fatalf("capture stacked windows: %v", err)
	}
	if snapshot.Windows[0].Layout.Column != snapshot.Windows[1].Layout.Column {
		t.Fatal("test windows did not form a stack")
	}
	snapshot.Monitors = nil
	data, err = json.Marshal(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(m.StatePath, data, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := client.Dispatch(ctx, "resizewindowpixel", "exact 1200 650,address:"+address); err != nil {
		t.Fatal(err)
	}
	if _, err := m.Restore(ctx, false, true); err != nil {
		t.Fatal(err)
	}
	restored, err = client.Desktop(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, saved := range snapshot.Windows {
		found := false
		for _, window := range restored.Windows {
			if window.InitialClass != appID || abs(window.At[1]-saved.At[1]) > 2 {
				continue
			}
			found = true
			if abs(window.Size[0]-saved.Size[0]) > 2 || abs(window.Size[1]-saved.Size[1]) > 2 {
				t.Fatalf("stacked size got %v, want %v", window.Size, saved.Size)
			}
		}
		if !found {
			t.Fatalf("stacked row at y=%d not restored", saved.At[1])
		}
	}
	t.Log("restored unequal stacked heights and custom widths")
}
