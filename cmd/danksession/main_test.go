package main

import (
	"context"
	"errors"
	"github.com/alcxyz/DankSession/internal/session"
	"testing"
)

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
