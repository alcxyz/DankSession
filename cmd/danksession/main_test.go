package main

import "testing"

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
