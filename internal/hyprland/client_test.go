package hyprland

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestDispatchExpressionUsesHyprlandLuaAPI(t *testing.T) {
	tests := []struct {
		dispatcher string
		argument   string
		want       string
	}{
		{"focuswindow", "address:0xabc", `hl.dsp.focus({ window = "address:0xabc" })`},
		{"workspace", "5", `hl.dsp.focus({ workspace = 5 })`},
		{"movetoworkspacesilent", "5,address:0xabc", `hl.dsp.window.move({ workspace = 5, window = "address:0xabc", follow = false })`},
		{"movetoworkspacesilent", "special:danksession-staging,address:0xabc", `hl.dsp.window.move({ workspace = "special:danksession-staging", window = "address:0xabc", follow = false })`},
		{"moveworkspacetomonitor", "5 DP-1", `hl.dsp.workspace.move({ workspace = 5, monitor = "DP-1" })`},
		{"moveworkspacetomonitor", "name:my work DP-1", `hl.dsp.workspace.move({ workspace = "name:my work", monitor = "DP-1" })`},
		{"movetoworkspacesilent", "name:work, mail,address:0xabc", `hl.dsp.window.move({ workspace = "name:work, mail", window = "address:0xabc", follow = false })`},
		{"setfloating", "address:0xabc", `hl.dsp.window.float({ action = "set", window = "address:0xabc" })`},
		{"settiled", "address:0xabc", `hl.dsp.window.float({ action = "unset", window = "address:0xabc" })`},
		{"resizewindowpixel", "exact 1200 800,address:0xabc", `hl.dsp.window.resize({ x = 1200, y = 800, relative = false, window = "address:0xabc" })`},
		{"movewindowpixel", "exact -20 30,address:0xabc", `hl.dsp.window.move({ x = -20, y = 30, relative = false, window = "address:0xabc" })`},
		{"fullscreenstate", "2,1,address:0xabc", `hl.dsp.window.fullscreen_state({ internal = 2, client = 1, action = "set", window = "address:0xabc" })`},
		{"layoutmsg", "colresize 0.500000", `hl.dsp.layout("colresize 0.500000")`},
	}

	for _, test := range tests {
		t.Run(test.dispatcher+"/"+test.argument, func(t *testing.T) {
			got, err := dispatchExpression(test.dispatcher, test.argument)
			if err != nil {
				t.Fatal(err)
			}
			if got != test.want {
				t.Fatalf("got %q, want %q", got, test.want)
			}
		})
	}
}

func TestInstanceRespectsExplicitSession(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_RUNTIME_DIR", dir)
	t.Setenv("HYPRLAND_INSTANCE_SIGNATURE", "chosen")
	client := &Client{Hyprctl: "/must-not-query-other-instances"}
	if _, err := client.Instance(context.Background()); err == nil {
		t.Fatal("accepted missing explicit session")
	}
	path := filepath.Join(dir, "hypr", "chosen", ".socket.sock")
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	instance, err := client.Instance(context.Background())
	if err != nil || instance != "chosen" {
		t.Fatalf("instance=%q err=%v", instance, err)
	}
}

func TestLogicalWidthAccountsForRotationAndScale(t *testing.T) {
	monitor := Monitor{Width: 3840, Height: 2160, Scale: 2, Transform: 1}
	if got := monitor.LogicalWidth(); got != 1080 {
		t.Fatalf("got %d", got)
	}
}

func TestDispatchExpressionRejectsLegacyOrMalformedActions(t *testing.T) {
	for _, test := range [][2]string{{"focusmonitor", ""}, {"movetoworkspacesilent", "5"}, {"fullscreenstate", "3,0,address:0xabc"}, {"legacy", "anything"}} {
		if _, err := dispatchExpression(test[0], test[1]); err == nil {
			t.Fatalf("%s %q unexpectedly succeeded", test[0], test[1])
		}
	}
}
