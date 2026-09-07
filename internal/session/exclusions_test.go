package session

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"reflect"
	"strings"
	"sync"
	"testing"
)

func TestExclusionCRUDPreservesConfigAndSnapshot(t *testing.T) {
	m, f := savedManager(t)
	cfg := DefaultConfig()
	cfg.Applications = []Application{{ID: "browser", Command: []string{"browser", "--profile", "QA profile"}, Match: Match{InitialClass: "^zen$"}}}
	cfg.CaptureInterval = 42
	if err := m.SaveConfig(cfg); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(m.StatePath)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	settings, err := m.ExclusionSettings(ctx)
	if err != nil || len(settings.Rules) != 0 || settings.Revision == "" {
		t.Fatalf("initial settings: %+v, %v", settings, err)
	}
	settings, err = m.UpdateExclusion(ctx, ExclusionUpdate{Action: "add", Revision: settings.Revision, Match: Match{InitialClass: "^zen$"}})
	if err != nil || len(settings.Rules) != 1 || settings.Rules[0].Index != 0 {
		t.Fatalf("add: %+v, %v", settings, err)
	}
	zero := 0
	settings, err = m.UpdateExclusion(ctx, ExclusionUpdate{Action: "update", Revision: settings.Revision, Index: &zero, Match: Match{InitialClass: "^zen$", Disabled: true}})
	if err != nil || !settings.Rules[0].Disabled {
		t.Fatalf("disable: %+v, %v", settings, err)
	}
	settings, err = m.UpdateExclusion(ctx, ExclusionUpdate{Action: "remove", Revision: settings.Revision, Index: &zero})
	if err != nil || len(settings.Rules) != 0 {
		t.Fatalf("remove: %+v, %v", settings, err)
	}
	loaded, err := m.LoadConfig()
	if err != nil || !reflect.DeepEqual(loaded.Applications, cfg.Applications) || loaded.CaptureInterval != cfg.CaptureInterval {
		t.Fatalf("unrelated settings changed: %+v, %v", loaded, err)
	}
	info, err := os.Stat(m.ConfigPath)
	if err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("config not private: %v, %v", info, err)
	}
	after, _ := os.ReadFile(m.StatePath)
	if string(before) != string(after) || len(f.dispatches) != 0 {
		t.Fatal("editing exclusions changed the snapshot or desktop")
	}
}

func TestExclusionApplicationsAndPreviewDoNotExposeTitles(t *testing.T) {
	m, f := savedManager(t)
	second := f.desktop.Windows[1]
	second.Workspace.ID, second.Workspace.Name = 12, "12"
	third := second
	third.Workspace.ID, third.Workspace.Name = 2, "2"
	unmapped := third
	unmapped.Mapped = false
	f.desktop.Windows = append(f.desktop.Windows, second, third, unmapped)
	ctx := context.Background()
	settings, err := m.ExclusionSettings(ctx)
	if err != nil || len(settings.Applications) != 2 {
		t.Fatalf("settings: %+v, %v", settings, err)
	}
	preview, err := m.PreviewExclusion(ctx, Match{InitialClass: "^zen$", Disabled: true})
	if err != nil || preview.Windows != 3 || len(preview.Applications) != 1 {
		t.Fatalf("preview: %+v, %v", preview, err)
	}
	if !reflect.DeepEqual(preview.Applications[0].Workspaces, []string{"2", "5", "12"}) {
		t.Fatalf("incorrect workspace grouping: %+v", preview.Applications)
	}
	data, _ := json.Marshal(settings)
	previewData, _ := json.Marshal(preview)
	if strings.Contains(string(data)+string(previewData), "private page title") || strings.Contains(string(previewData), `"title"`) {
		t.Fatal("open application list leaked titles")
	}
	combined, err := m.PreviewExclusion(ctx, Match{InitialClass: "^zen$", Class: "^thunderbird$"})
	if err != nil || combined.Windows != 0 || combined.Applications == nil {
		t.Fatalf("matcher fields not ANDed or empty list null: %+v, %v", combined, err)
	}
	if _, err := os.Stat(m.ConfigPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("read-only preview wrote configuration")
	}
	if len(f.dispatches) != 0 {
		t.Fatal("read-only exclusion query dispatched desktop changes")
	}
}

func TestExclusionValidationAndRevisionProtection(t *testing.T) {
	m, _ := savedManager(t)
	ctx := context.Background()
	settings, _ := m.ExclusionSettings(ctx)
	zero, negative, large := 0, -1, 50
	requests := []ExclusionUpdate{
		{Action: "add", Match: Match{Class: "zen"}},
		{Action: "other", Revision: settings.Revision},
		{Action: "add", Revision: settings.Revision},
		{Action: "add", Revision: settings.Revision, Match: Match{Class: "["}},
		{Action: "add", Revision: settings.Revision, Match: Match{Class: "zen"}, Index: &zero},
		{Action: "remove", Revision: settings.Revision},
		{Action: "remove", Revision: settings.Revision, Index: &negative},
		{Action: "remove", Revision: settings.Revision, Index: &large},
		{Action: "update", Revision: settings.Revision, Match: Match{Class: "zen"}},
	}
	for _, request := range requests {
		if _, err := m.UpdateExclusion(ctx, request); err == nil {
			t.Fatalf("accepted invalid request: %+v", request)
		}
	}
	oldRevision := settings.Revision
	settings, err := m.UpdateExclusion(ctx, ExclusionUpdate{Action: "add", Revision: settings.Revision, Match: Match{Class: "zen"}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := m.UpdateExclusion(ctx, ExclusionUpdate{Action: "remove", Revision: oldRevision, Index: &zero}); !errors.Is(err, ErrStaleExclusions) {
		t.Fatalf("stale update accepted: %v", err)
	}
	if _, err := m.UpdateExclusion(ctx, ExclusionUpdate{Action: "add", Revision: settings.Revision, Match: Match{Class: "zen", Disabled: true}}); err == nil {
		t.Fatal("duplicate accepted despite same matcher")
	}
	for _, match := range []Match{{}, {Disabled: true}, {InitialClass: "["}, {Title: "("}} {
		if _, err := m.PreviewExclusion(ctx, match); err == nil {
			t.Fatalf("invalid preview accepted: %+v", match)
		}
	}
	if err := m.UpdateConfig(func(cfg *Config) error { cfg.CaptureInterval = 37; return nil }); err != nil {
		t.Fatal(err)
	}
	if _, err := m.UpdateExclusion(ctx, ExclusionUpdate{Action: "remove", Revision: settings.Revision, Index: &zero}); err != nil {
		t.Fatalf("unrelated preference incorrectly invalidated revision: %v", err)
	}
}

func TestDisabledExclusionRetainsApplicationMatching(t *testing.T) {
	window := testDesktop().Windows[2]
	match := Match{InitialClass: "^zen$", Disabled: true}
	if excluded(window, []Match{match}) || !matches(match, window) {
		t.Fatal("disabled must affect exclusions only")
	}
	if got := applicationFor(window, []Application{{ID: "browser", Match: match}}); got != "browser" {
		t.Fatalf("disabled unexpectedly affected launch rule: %s", got)
	}
	match.Disabled = false
	if !excluded(window, []Match{match}) {
		t.Fatal("enabled exclusion did not match")
	}
}

func TestManagedExclusionAppliesToOldSnapshot(t *testing.T) {
	m, f := savedManager(t)
	ctx := context.Background()
	settings, _ := m.ExclusionSettings(ctx)
	settings, err := m.UpdateExclusion(ctx, ExclusionUpdate{Action: "add", Revision: settings.Revision, Match: Match{Class: ".*"}})
	if err != nil {
		t.Fatal(err)
	}
	result, err := m.Restore(ctx, false, false)
	if err != nil || result.Matched != 0 || result.Launched != 0 || len(f.dispatches) != 0 {
		t.Fatalf("new exclusion did not protect old snapshot: %+v, %v", result, err)
	}
	zero := 0
	if _, err := m.UpdateExclusion(ctx, ExclusionUpdate{Action: "update", Revision: settings.Revision, Index: &zero, Match: Match{Class: ".*", Disabled: true}}); err != nil {
		t.Fatal(err)
	}
	result, err = m.Restore(ctx, true, true)
	if err != nil || result.Matched != 2 {
		t.Fatalf("disabled exclusion still applied: %+v, %v", result, err)
	}
}

func TestUpdateConfigLockAndValidation(t *testing.T) {
	m, _ := savedManager(t)
	other := *m
	entered, release := make(chan struct{}), make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(1)
	var updateErr error
	go func() {
		defer wg.Done()
		updateErr = m.UpdateConfig(func(cfg *Config) error {
			close(entered)
			<-release
			cfg.CaptureInterval = 48
			return nil
		})
	}()
	<-entered
	err := other.UpdateConfig(func(cfg *Config) error { cfg.Exclude = []Match{{Class: "zen"}}; return nil })
	close(release)
	wg.Wait()
	if !errors.Is(err, ErrBusy) || updateErr != nil {
		t.Fatalf("concurrent config updates not protected: %v / %v", err, updateErr)
	}
	if err := other.UpdateConfig(func(cfg *Config) error { cfg.Exclude = []Match{{Class: "zen"}}; return nil }); err != nil {
		t.Fatal(err)
	}
	loaded, _ := m.LoadConfig()
	if loaded.CaptureInterval != 48 || len(loaded.Exclude) != 1 {
		t.Fatalf("retry lost settings: %+v", loaded)
	}
	before, _ := os.ReadFile(m.ConfigPath)
	if err := m.UpdateConfig(func(cfg *Config) error { cfg.Exclude = []Match{{}}; return nil }); err == nil {
		t.Fatal("invalid configuration written")
	}
	if err := m.UpdateConfig(func(cfg *Config) error { cfg.CaptureInterval = 99; return errors.New("cancel") }); err == nil {
		t.Fatal("callback error swallowed")
	}
	after, _ := os.ReadFile(m.ConfigPath)
	if string(before) != string(after) {
		t.Fatal("failed update changed configuration")
	}
}

func TestExclusionCancelledMutationDoesNotWrite(t *testing.T) {
	m, _ := savedManager(t)
	settings, _ := m.ExclusionSettings(context.Background())
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := m.UpdateExclusion(ctx, ExclusionUpdate{Action: "add", Revision: settings.Revision, Match: Match{Class: "zen"}})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled request: %v", err)
	}
	if _, err := os.Stat(m.ConfigPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("cancelled request wrote config")
	}
}

func TestHistoricalTitleExclusionsAreConservativeWithoutTitles(t *testing.T) {
	saved := Window{Class: "zen", InitialClass: "zen"}
	cases := []struct {
		name string
		rule Match
		want bool
	}{
		{"class and unknown title", Match{Class: "^zen$", Title: "private"}, true},
		{"initial class and unknown title", Match{InitialClass: "^zen$", Title: "private"}, true},
		{"other application", Match{InitialClass: "^thunderbird$", Title: "private"}, false},
		{"all fields required", Match{Class: "^zen$", InitialClass: "^thunderbird$", Title: "private"}, false},
		{"title only", Match{Title: "private"}, true},
		{"disabled title rule", Match{Title: "private", Disabled: true}, false},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			if got := excludedSaved(saved, []Match{tt.rule}); got != tt.want {
				t.Fatalf("excluded=%v, want %v", got, tt.want)
			}
		})
	}
	saved.Title = "public"
	if excludedSaved(saved, []Match{{Class: "^zen$", Title: "private"}}) {
		t.Fatal("known nonmatching title was excluded")
	}
}

func TestTitleExclusionsPreventRelaunchFromPrivateHistoricalSnapshot(t *testing.T) {
	m, f := savedManager(t)
	cfg := DefaultConfig()
	cfg.Applications = []Application{{ID: "browser", Command: []string{"must-not-launch"}, Match: Match{InitialClass: "^zen$"}}}
	if err := m.SaveConfig(cfg); err != nil {
		t.Fatal(err)
	}
	if _, err := m.Capture(context.Background()); err != nil {
		t.Fatal(err)
	}
	cfg.Exclude = []Match{{InitialClass: "^zen$", Title: "private"}}
	if err := m.SaveConfig(cfg); err != nil {
		t.Fatal(err)
	}
	// The browser may still be running or completely absent after a crash.
	for _, open := range []bool{true, false} {
		if !open {
			f.desktop.Windows = f.desktop.Windows[:1]
		}
		result, err := m.Restore(context.Background(), true, false)
		if err != nil || result.Matched != 1 || result.Missing != 0 {
			t.Fatalf("browser open=%v: %+v, %v", open, result, err)
		}
		for _, operation := range result.Operations {
			if operation.Kind == "launch" || operation.Application == "browser" {
				t.Fatalf("excluded browser restored: %+v", operation)
			}
		}
	}
	if len(f.dispatches) != 0 {
		t.Fatal("dry-run dispatched changes")
	}
}

func TestLiveTitleExclusionDoesNotRelaunchExistingApplication(t *testing.T) {
	m, f := savedManager(t)
	cfg := DefaultConfig()
	cfg.CaptureTitles = true
	cfg.Applications = []Application{{ID: "browser", Command: []string{"must-not-launch"}, Match: Match{InitialClass: "^zen$"}}}
	f.desktop.Windows[1].Title = "public"
	if err := m.SaveConfig(cfg); err != nil {
		t.Fatal(err)
	}
	if _, err := m.Capture(context.Background()); err != nil {
		t.Fatal(err)
	}
	f.desktop.Windows[1].Title = "private"
	cfg.Exclude = []Match{{InitialClass: "^zen$", Title: "private"}}
	if err := m.SaveConfig(cfg); err != nil {
		t.Fatal(err)
	}
	result, err := m.Restore(context.Background(), true, false)
	if err != nil || result.Matched != 1 || result.Missing != 1 {
		t.Fatalf("unexpected restore: %+v, %v", result, err)
	}
	skipped := false
	for _, operation := range result.Operations {
		if operation.Kind == "launch" {
			t.Fatalf("excluded running browser was relaunched: %+v", operation)
		}
		if operation.Kind == "skip-launch" && operation.Application == "browser" {
			skipped = true
		}
	}
	if !skipped || len(f.dispatches) != 0 {
		t.Fatal("skipped launch missing or dry-run dispatched changes")
	}
}
