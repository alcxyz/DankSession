package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/alcxyz/DankSession/internal/hyprland"
	"github.com/alcxyz/DankSession/internal/session"
	"testing"
)

func TestConfigurePreservesUnrelatedSettingsAndExclusions(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("DANKSESSION_CONFIG", filepath.Join(dir, "config.json"))
	t.Setenv("DANKSESSION_STATE", filepath.Join(dir, "last.json"))
	m := &session.Manager{ConfigPath: filepath.Join(dir, "config.json")}
	cfg := session.DefaultConfig()
	cfg.Exclude = []session.Match{{InitialClass: "^private-app$"}}
	cfg.CaptureInterval = 35
	if err := m.SaveConfig(cfg); err != nil {
		t.Fatal(err)
	}
	if err := run([]string{"configure", "--auto-capture=false"}); err != nil {
		t.Fatal(err)
	}
	got, err := m.LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	if got.AutoCapture || got.CaptureInterval != 35 || len(got.Exclude) != 1 {
		t.Fatalf("configure replaced unrelated preferences: %+v", got)
	}
	if err := run([]string{"configure", "--capture-interval=0"}); err == nil {
		t.Fatal("accepted invalid capture interval")
	}
	got, _ = m.LoadConfig()
	if got.CaptureInterval != 35 {
		t.Fatal("failed update changed config")
	}
}

func TestJSONInputIsBoundedAndStrict(t *testing.T) {
	for _, value := range []string{`{"class":"^zen$"}`, `{"typo":true}`, `{} {}`, strings.Repeat(" ", 65537)} {
		t.Run(value[:min(16, len(value))], func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "request.json")
			if err := os.WriteFile(path, []byte(value), 0o600); err != nil {
				t.Fatal(err)
			}
			input, err := os.Open(path)
			if err != nil {
				t.Fatal(err)
			}
			defer input.Close()
			previous := os.Stdin
			os.Stdin = input
			defer func() { os.Stdin = previous }()
			var match session.Match
			err = readInputJSON(&match)
			if (err == nil) != (value == `{"class":"^zen$"}`) {
				t.Fatalf("unexpected decode result: %v", err)
			}
		})
	}
}

type countingCompositor struct{ captures chan struct{} }

func (c countingCompositor) Desktop(context.Context) (hyprland.Desktop, error) {
	c.captures <- struct{}{}
	return hyprland.Desktop{Monitors: []hyprland.Monitor{{ID: 1, Name: "test", Width: 1000, Height: 1000}}}, nil
}

func (c countingCompositor) Dispatch(context.Context, string, string) error {
	panic("preference updates must never dispatch")
}

func TestAutomaticSavingCanPauseAndResumeWithoutRestart(t *testing.T) {
	dir := t.TempDir()
	calls := make(chan struct{}, 10)
	m := &session.Manager{Compositor: countingCompositor{captures: calls}, ConfigPath: filepath.Join(dir, "config.json"), StatePath: filepath.Join(dir, "last.json")}
	cfg := session.DefaultConfig()
	cfg.AutoCapture = false
	cfg.DebounceMS = 100
	if err := m.SaveConfig(cfg); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	events := make(chan string, 1)
	done := make(chan error, 1)
	go func() { done <- captureLoop(ctx, m, cfg, events, make(chan error)) }()
	events <- "movewindow>>test"
	select {
	case <-calls:
		t.Fatal("automatic capture ran while paused")
	case <-time.After(250 * time.Millisecond):
	}
	if err := m.UpdateConfig(func(current *session.Config) error { current.AutoCapture = true; return nil }); err != nil {
		t.Fatal(err)
	}
	events <- "movewindow>>test"
	select {
	case <-calls:
	case <-time.After(2 * time.Second):
		t.Fatal("automatic capture did not resume")
	}
	cancel()
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}

func TestCaptureIntervalChangesWhileDesktopIsIdle(t *testing.T) {
	dir := t.TempDir()
	calls := make(chan struct{}, 10)
	m := &session.Manager{Compositor: countingCompositor{captures: calls}, ConfigPath: filepath.Join(dir, "config.json"), StatePath: filepath.Join(dir, "last.json")}
	cfg := session.DefaultConfig()
	cfg.CaptureInterval = 120
	updated := cfg
	updated.CaptureInterval = 1
	if err := m.SaveConfig(updated); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- captureLoop(ctx, m, cfg, make(chan string), make(chan error)) }()
	select {
	case <-calls:
	case <-time.After(4 * time.Second):
		t.Fatal("new interval was not applied until the old interval elapsed")
	}
	cancel()
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}

func TestCaptureLoopDoesNotCaptureDuringShutdown(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	// A capture would dereference the nil compositor: no shutdown snapshot
	// may replace the last completed desktop snapshot.
	if err := captureLoop(ctx, &session.Manager{}, session.DefaultConfig(), make(chan string), make(chan error)); err != nil {
		t.Fatal(err)
	}
}

func TestCaptureLoopStopsWhenCompositorDisconnects(t *testing.T) {
	events := make(chan string)
	close(events)
	if err := captureLoop(context.Background(), &session.Manager{}, session.DefaultConfig(), events, make(chan error)); err != nil {
		t.Fatal(err)
	}
}

func TestCaptureLoopReportsErrorWhenBothChannelsClose(t *testing.T) {
	for i := 0; i < 100; i++ {
		events := make(chan string)
		eventErrors := make(chan error, 1)
		failure := errors.New("socket connection failed")
		eventErrors <- failure
		close(events)
		close(eventErrors)
		if err := captureLoop(context.Background(), &session.Manager{}, session.DefaultConfig(), events, eventErrors); !errors.Is(err, failure) {
			t.Fatalf("connection failure lost: %v", err)
		}
	}
}

func TestCaptureEventIncludesPlacementAndFocusChanges(t *testing.T) {
	for _, event := range []string{
		"openwindow>>0xabc,5,foot,title",
		"movewindow>>0xabc,8",
		"workspacev2>>8,8",
		"focusedmon>>DP-1,8",
		"activewindowv2>>0xabc",
		"fullscreen>>0xabc,1",
	} {
		if !captureEvent(event) {
			t.Errorf("captureEvent(%q) = false", event)
		}
	}
	if captureEvent("submap>>resize") {
		t.Fatal("unrelated submap event scheduled a snapshot")
	}
}
