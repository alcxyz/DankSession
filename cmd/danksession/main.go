package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/alcxyz/DankSession/internal/hyprland"
	"github.com/alcxyz/DankSession/internal/session"
)

var version = "dev"

func main() {
	if err := run(os.Args[1:]); err != nil {
		writeJSON(map[string]any{"error": err.Error()})
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) == 0 || args[0] == "help" || args[0] == "--help" || args[0] == "-h" {
		fmt.Print(help())
		return nil
	}
	if args[0] == "--version" || args[0] == "version" {
		fmt.Println(version)
		return nil
	}

	statePath, configPath, err := session.DefaultPaths()
	if err != nil {
		return err
	}
	if value := os.Getenv("DANKSESSION_STATE"); value != "" {
		statePath = value
	}
	if value := os.Getenv("DANKSESSION_CONFIG"); value != "" {
		configPath = value
	}
	manager := &session.Manager{Compositor: hyprland.NewClient(), StatePath: statePath, ConfigPath: configPath}

	switch args[0] {
	case "capture", "save":
		snapshot, err := manager.Capture(context.Background())
		if err != nil {
			return err
		}
		writeJSON(map[string]any{"saved": true, "savedAt": snapshot.SavedAt, "windows": len(snapshot.Windows), "statePath": statePath})
		return nil
	case "status":
		status, err := manager.Status()
		if err != nil {
			return err
		}
		writeJSON(status)
		return nil
	case "restore":
		flags := flag.NewFlagSet("restore", flag.ContinueOnError)
		dryRun := flags.Bool("dry-run", false, "print operations without changing the desktop")
		noLaunch := flags.Bool("no-launch", false, "restore only applications that are already running")
		if err := flags.Parse(args[1:]); err != nil {
			return err
		}
		result, err := manager.Restore(context.Background(), *dryRun, *noLaunch)
		if err != nil {
			return err
		}
		writeJSON(result)
		return nil
	case "configure":
		cfg, err := manager.LoadConfig()
		if err != nil {
			return err
		}
		flags := flag.NewFlagSet("configure", flag.ContinueOnError)
		autoRestore := flags.Bool("auto-restore", cfg.AutoRestore, "restore the last session when the daemon starts")
		captureUnconfigured := flags.Bool("capture-unconfigured", cfg.CaptureUnconfigured, "capture windows without a launch rule")
		captureTitles := flags.Bool("capture-titles", cfg.CaptureTitles, "store window titles in the local snapshot")
		debounceMS := flags.Int("debounce-ms", cfg.DebounceMS, "event capture debounce in milliseconds")
		captureInterval := flags.Int("capture-interval", cfg.CaptureInterval, "periodic capture interval in seconds")
		restoreTimeout := flags.Int("restore-timeout", cfg.RestoreTimeout, "application restore timeout in seconds")
		if err := flags.Parse(args[1:]); err != nil {
			return err
		}
		cfg.AutoRestore = *autoRestore
		cfg.CaptureUnconfigured = *captureUnconfigured
		cfg.CaptureTitles = *captureTitles
		cfg.DebounceMS = *debounceMS
		cfg.CaptureInterval = *captureInterval
		cfg.RestoreTimeout = *restoreTimeout
		if err := manager.SaveConfig(cfg); err != nil {
			return err
		}
		writeJSON(cfg)
		return nil
	case "daemon":
		return runDaemon(manager)
	default:
		return fmt.Errorf("unknown command %q", args[0])
	}
}

func runDaemon(manager *session.Manager) error {
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	cfg, err := manager.LoadConfig()
	if err != nil {
		return err
	}
	if cfg.AutoRestore {
		result, restoreErr := manager.Restore(ctx, false, false)
		if restoreErr != nil && !errors.Is(restoreErr, os.ErrNotExist) {
			writeJSON(map[string]any{"event": "restore-error", "error": restoreErr.Error()})
		} else if restoreErr == nil {
			writeJSON(map[string]any{"event": "restored", "matched": result.Matched, "missing": result.Missing, "launched": result.Launched})
		}
	}
	if _, err := manager.Capture(ctx); err != nil {
		return err
	}

	client, ok := manager.Compositor.(*hyprland.Client)
	if !ok {
		return errors.New("daemon requires the Hyprland compositor client")
	}
	events, eventErrors := client.Events(ctx)
	interval := time.NewTicker(time.Duration(cfg.CaptureInterval) * time.Second)
	defer interval.Stop()
	var debounce *time.Timer
	var debounceC <-chan time.Time

	schedule := func() {
		if debounce != nil {
			debounce.Stop()
		}
		debounce = time.NewTimer(time.Duration(cfg.DebounceMS) * time.Millisecond)
		debounceC = debounce.C
	}
	capture := func() {
		if _, err := manager.Capture(context.Background()); err != nil {
			writeJSON(map[string]any{"event": "capture-error", "error": err.Error()})
		}
	}

	for {
		select {
		case <-ctx.Done():
			capture()
			return nil
		case event, open := <-events:
			if !open {
				events = nil
				continue
			}
			if captureEvent(event) {
				schedule()
			}
		case err, open := <-eventErrors:
			if open && err != nil {
				return err
			}
			eventErrors = nil
		case <-interval.C:
			capture()
		case <-debounceC:
			debounceC = nil
			capture()
		}
	}
}

func captureEvent(event string) bool {
	name, _, _ := strings.Cut(event, ">>")
	switch name {
	case "openwindow", "closewindow", "movewindow", "moveworkspace", "changefloatingmode", "fullscreen",
		"workspace", "workspacev2", "focusedmon", "activewindow", "activewindowv2",
		"monitoradded", "monitorremoved":
		return true
	default:
		return false
	}
}

func writeJSON(value any) {
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetEscapeHTML(false)
	_ = encoder.Encode(value)
}

func help() string {
	return `DankSession restores application windows to their previous desktop state.

Usage:
  danksession capture
  danksession restore [--dry-run] [--no-launch]
  danksession status
  danksession configure [options]
  danksession daemon
  danksession --version
`
}
