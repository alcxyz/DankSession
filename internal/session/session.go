package session

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/alcxyz/DankSession/internal/hyprland"
)

const SchemaVersion = 1

type Match struct {
	Class        string `json:"class,omitempty"`
	InitialClass string `json:"initialClass,omitempty"`
	Title        string `json:"title,omitempty"`
}

type Application struct {
	ID      string   `json:"id"`
	Command []string `json:"command"`
	Match   Match    `json:"match"`
}

type Config struct {
	AutoRestore         bool          `json:"autoRestore"`
	CaptureUnconfigured bool          `json:"captureUnconfigured"`
	CaptureTitles       bool          `json:"captureTitles"`
	DebounceMS          int           `json:"debounceMs"`
	CaptureInterval     int           `json:"captureIntervalSeconds"`
	RestoreTimeout      int           `json:"restoreTimeoutSeconds"`
	Applications        []Application `json:"applications"`
	Exclude             []Match       `json:"exclude"`
}

type Layout struct {
	Name        string  `json:"name"`
	Column      int     `json:"column"`
	Row         int     `json:"row"`
	ColumnWidth float64 `json:"columnWidth"`
	Centered    bool    `json:"centered,omitempty"`
}

type Window struct {
	Slot             string                `json:"slot"`
	Application      string                `json:"application,omitempty"`
	Class            string                `json:"class"`
	InitialClass     string                `json:"initialClass"`
	Title            string                `json:"title,omitempty"`
	InitialTitle     string                `json:"initialTitle,omitempty"`
	Workspace        hyprland.WorkspaceRef `json:"workspace"`
	Monitor          string                `json:"monitor"`
	Floating         bool                  `json:"floating"`
	Pinned           bool                  `json:"pinned,omitempty"`
	Fullscreen       int                   `json:"fullscreen,omitempty"`
	FullscreenClient int                   `json:"fullscreenClient,omitempty"`
	At               hyprland.Point        `json:"at"`
	Size             hyprland.Point        `json:"size"`
	Layout           *Layout               `json:"layout,omitempty"`
	Focused          bool                  `json:"focused,omitempty"`
}

type Monitor struct {
	Name            string                `json:"name"`
	ActiveWorkspace hyprland.WorkspaceRef `json:"activeWorkspace"`
}

type Snapshot struct {
	Schema   int       `json:"schema"`
	SavedAt  time.Time `json:"savedAt"`
	Monitors []Monitor `json:"monitors"`
	Windows  []Window  `json:"windows"`
}

type Operation struct {
	Kind        string `json:"kind"`
	Slot        string `json:"slot,omitempty"`
	Application string `json:"application,omitempty"`
	Detail      string `json:"detail"`
}

type RestoreResult struct {
	DryRun     bool        `json:"dryRun"`
	Matched    int         `json:"matched"`
	Missing    int         `json:"missing"`
	Launched   int         `json:"launched"`
	Operations []Operation `json:"operations"`
}

type Status struct {
	Saved       bool      `json:"saved"`
	SavedAt     time.Time `json:"savedAt,omitempty"`
	Windows     int       `json:"windows"`
	Managed     int       `json:"managed"`
	Workspaces  int       `json:"workspaces"`
	AutoRestore bool      `json:"autoRestore"`
	StatePath   string    `json:"statePath"`
}

type Compositor interface {
	Desktop(context.Context) (hyprland.Desktop, error)
	Dispatch(context.Context, string, string) error
}

type Manager struct {
	Compositor Compositor
	StatePath  string
	ConfigPath string
}

func DefaultConfig() Config {
	return Config{
		CaptureUnconfigured: true,
		DebounceMS:          750,
		CaptureInterval:     15,
		RestoreTimeout:      20,
	}
}

func DefaultPaths() (statePath, configPath string, err error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", "", err
	}
	stateHome := os.Getenv("XDG_STATE_HOME")
	if stateHome == "" {
		stateHome = filepath.Join(home, ".local", "state")
	}
	configHome := os.Getenv("XDG_CONFIG_HOME")
	if configHome == "" {
		configHome = filepath.Join(home, ".config")
	}
	return filepath.Join(stateHome, "danksession", "last.json"), filepath.Join(configHome, "danksession", "config.json"), nil
}

func (m *Manager) LoadConfig() (Config, error) {
	cfg := DefaultConfig()
	data, err := os.ReadFile(m.ConfigPath)
	if errors.Is(err, os.ErrNotExist) {
		return cfg, nil
	}
	if err != nil {
		return Config{}, err
	}
	if err := json.Unmarshal(data, &cfg); err != nil {
		return Config{}, fmt.Errorf("decode config: %w", err)
	}
	if cfg.DebounceMS < 100 {
		cfg.DebounceMS = 100
	}
	if cfg.CaptureInterval < 1 {
		cfg.CaptureInterval = 15
	}
	if cfg.RestoreTimeout < 1 {
		cfg.RestoreTimeout = 20
	}
	for _, app := range cfg.Applications {
		if app.ID == "" || len(app.Command) == 0 {
			return Config{}, errors.New("every application requires an id and command")
		}
		if err := validateMatch(app.Match); err != nil {
			return Config{}, fmt.Errorf("application %q: %w", app.ID, err)
		}
	}
	for _, match := range cfg.Exclude {
		if err := validateMatch(match); err != nil {
			return Config{}, fmt.Errorf("exclude: %w", err)
		}
	}
	return cfg, nil
}

func (m *Manager) SaveConfig(cfg Config) error {
	return writeJSONAtomic(m.ConfigPath, cfg)
}

func (m *Manager) Capture(ctx context.Context) (Snapshot, error) {
	cfg, err := m.LoadConfig()
	if err != nil {
		return Snapshot{}, err
	}
	desktop, err := m.Compositor.Desktop(ctx)
	if err != nil {
		return Snapshot{}, err
	}
	snapshot := BuildSnapshot(desktop, cfg, time.Now().UTC())
	if err := writeJSONAtomic(m.StatePath, snapshot); err != nil {
		return Snapshot{}, err
	}
	return snapshot, nil
}

func BuildSnapshot(desktop hyprland.Desktop, cfg Config, now time.Time) Snapshot {
	monitorByID := make(map[int]hyprland.Monitor, len(desktop.Monitors))
	snapshot := Snapshot{Schema: SchemaVersion, SavedAt: now}
	for _, monitor := range desktop.Monitors {
		monitorByID[monitor.ID] = monitor
		snapshot.Monitors = append(snapshot.Monitors, Monitor{Name: monitor.Name, ActiveWorkspace: monitor.ActiveWorkspace})
	}

	type candidate struct {
		window hyprland.Window
		app    string
	}
	byWorkspace := map[int][]candidate{}
	for _, window := range desktop.Windows {
		if !window.Mapped || window.Hidden || window.Workspace.ID < 0 || excluded(window, cfg.Exclude) {
			continue
		}
		app := applicationFor(window, cfg.Applications)
		if app == "" && !cfg.CaptureUnconfigured {
			continue
		}
		byWorkspace[window.Workspace.ID] = append(byWorkspace[window.Workspace.ID], candidate{window: window, app: app})
	}

	ordinals := map[string]int{}
	workspaceIDs := make([]int, 0, len(byWorkspace))
	for id := range byWorkspace {
		workspaceIDs = append(workspaceIDs, id)
	}
	sort.Ints(workspaceIDs)
	for _, workspaceID := range workspaceIDs {
		candidates := byWorkspace[workspaceID]
		sort.SliceStable(candidates, func(i, j int) bool {
			if candidates[i].window.Floating != candidates[j].window.Floating {
				return !candidates[i].window.Floating
			}
			if candidates[i].window.At[0] != candidates[j].window.At[0] {
				return candidates[i].window.At[0] < candidates[j].window.At[0]
			}
			return candidates[i].window.At[1] < candidates[j].window.At[1]
		})

		column := -1
		columnX := math.MinInt
		row := 0
		for _, item := range candidates {
			window := item.window
			identity := item.app
			if identity == "" {
				identity = firstNonEmpty(window.InitialClass, window.Class, "window")
			}
			ordinals[identity]++
			saved := Window{
				Slot:             fmt.Sprintf("%s#%d", identity, ordinals[identity]),
				Application:      item.app,
				Class:            window.Class,
				InitialClass:     window.InitialClass,
				Workspace:        window.Workspace,
				Floating:         window.Floating,
				Pinned:           window.Pinned,
				Fullscreen:       window.Fullscreen,
				FullscreenClient: window.FullscreenClient,
				At:               window.At,
				Size:             window.Size,
				Focused:          window.Address != "" && window.Address == desktop.ActiveWindow.Address,
			}
			if cfg.CaptureTitles {
				saved.Title = window.Title
				saved.InitialTitle = window.InitialTitle
			}
			monitor, ok := monitorByID[window.Monitor]
			if ok {
				saved.Monitor = monitor.Name
			}
			if !window.Floating && ok {
				if columnX == math.MinInt || abs(window.At[0]-columnX) > 16 {
					column++
					columnX = window.At[0]
					row = 0
				} else {
					row++
				}
				width := snapWidth(float64(window.Size[0]+8) / float64(max(1, monitor.Width)))
				centered := abs((window.At[0]+window.Size[0]/2)-(monitor.X+monitor.Width/2)) <= 16
				saved.Layout = &Layout{Name: "scrolling", Column: column, Row: row, ColumnWidth: width, Centered: centered}
			}
			snapshot.Windows = append(snapshot.Windows, saved)
		}
	}
	return snapshot
}

func (m *Manager) LoadSnapshot() (Snapshot, error) {
	data, err := os.ReadFile(m.StatePath)
	if err != nil {
		return Snapshot{}, err
	}
	var snapshot Snapshot
	if err := json.Unmarshal(data, &snapshot); err != nil {
		return Snapshot{}, fmt.Errorf("decode snapshot: %w", err)
	}
	if snapshot.Schema != SchemaVersion {
		return Snapshot{}, fmt.Errorf("unsupported snapshot schema %d", snapshot.Schema)
	}
	return snapshot, nil
}

func (m *Manager) Status() (Status, error) {
	cfg, err := m.LoadConfig()
	if err != nil {
		return Status{}, err
	}
	status := Status{AutoRestore: cfg.AutoRestore, StatePath: m.StatePath}
	snapshot, err := m.LoadSnapshot()
	if errors.Is(err, os.ErrNotExist) {
		return status, nil
	}
	if err != nil {
		return Status{}, err
	}
	status.Saved = true
	status.SavedAt = snapshot.SavedAt
	status.Windows = len(snapshot.Windows)
	workspaces := map[int]bool{}
	for _, window := range snapshot.Windows {
		workspaces[window.Workspace.ID] = true
		if window.Application != "" {
			status.Managed++
		}
	}
	status.Workspaces = len(workspaces)
	return status, nil
}

func (m *Manager) Restore(ctx context.Context, dryRun, noLaunch bool) (RestoreResult, error) {
	snapshot, err := m.LoadSnapshot()
	if err != nil {
		return RestoreResult{}, err
	}
	cfg, err := m.LoadConfig()
	if err != nil {
		return RestoreResult{}, err
	}
	desktop, err := m.Compositor.Desktop(ctx)
	if err != nil {
		return RestoreResult{}, err
	}

	result := RestoreResult{DryRun: dryRun}
	matches, missing := matchWindows(snapshot.Windows, desktop.Windows, cfg)
	result.Matched = len(matches)
	result.Missing = len(missing)

	launched := map[string]bool{}
	if !noLaunch {
		for _, saved := range missing {
			if saved.Application == "" || launched[saved.Application] {
				continue
			}
			app, ok := configuredApplication(saved.Application, cfg.Applications)
			if !ok {
				continue
			}
			result.Operations = append(result.Operations, Operation{Kind: "launch", Application: app.ID, Detail: strings.Join(app.Command, " ")})
			launched[app.ID] = true
			if !dryRun {
				if err := launch(app.Command); err != nil {
					return result, fmt.Errorf("launch %s: %w", app.ID, err)
				}
				result.Launched++
			}
		}
	}

	if !dryRun && len(launched) > 0 {
		deadline := time.Now().Add(time.Duration(cfg.RestoreTimeout) * time.Second)
		for time.Now().Before(deadline) {
			time.Sleep(250 * time.Millisecond)
			desktop, err = m.Compositor.Desktop(ctx)
			if err != nil {
				return result, err
			}
			matches, missing = matchWindows(snapshot.Windows, desktop.Windows, cfg)
			if len(missing) == 0 {
				break
			}
		}
		result.Matched = len(matches)
		result.Missing = len(missing)
	}

	for _, saved := range snapshot.Windows {
		current, ok := matches[saved.Slot]
		if !ok {
			continue
		}
		if current.Workspace.ID != saved.Workspace.ID {
			detail := fmt.Sprintf("workspace %d -> %d", current.Workspace.ID, saved.Workspace.ID)
			result.Operations = append(result.Operations, Operation{Kind: "move-workspace", Slot: saved.Slot, Detail: detail})
			if !dryRun {
				arg := fmt.Sprintf("%d,address:%s", saved.Workspace.ID, current.Address)
				if err := m.Compositor.Dispatch(ctx, "movetoworkspacesilent", arg); err != nil {
					return result, err
				}
			}
		}
		if saved.Floating != current.Floating {
			dispatcher := "setfloating"
			detail := "restore floating state"
			if !saved.Floating {
				dispatcher = "settiled"
				detail = "restore tiled state"
			}
			result.Operations = append(result.Operations, Operation{Kind: "float", Slot: saved.Slot, Detail: "restore floating state"})
			if !dryRun {
				if err := m.Compositor.Dispatch(ctx, dispatcher, "address:"+current.Address); err != nil {
					return result, err
				}
			}
			result.Operations[len(result.Operations)-1].Detail = detail
		}
		if saved.Floating {
			result.Operations = append(result.Operations, Operation{Kind: "geometry", Slot: saved.Slot, Detail: fmt.Sprintf("%dx%d at %d,%d", saved.Size[0], saved.Size[1], saved.At[0], saved.At[1])})
			if !dryRun {
				if err := m.Compositor.Dispatch(ctx, "resizewindowpixel", fmt.Sprintf("exact %d %d,address:%s", saved.Size[0], saved.Size[1], current.Address)); err != nil {
					return result, err
				}
				if err := m.Compositor.Dispatch(ctx, "movewindowpixel", fmt.Sprintf("exact %d %d,address:%s", saved.At[0], saved.At[1], current.Address)); err != nil {
					return result, err
				}
			}
		}
		if saved.Pinned != current.Pinned {
			dispatcher := "pin"
			detail := "restore pinned state"
			if !saved.Pinned {
				dispatcher = "unpin"
				detail = "restore unpinned state"
			}
			result.Operations = append(result.Operations, Operation{Kind: "pin", Slot: saved.Slot, Detail: detail})
			if !dryRun {
				if err := m.Compositor.Dispatch(ctx, dispatcher, "address:"+current.Address); err != nil {
					return result, err
				}
			}
		}
		if saved.Fullscreen != current.Fullscreen || saved.FullscreenClient != current.FullscreenClient {
			result.Operations = append(result.Operations, Operation{Kind: "fullscreen", Slot: saved.Slot, Detail: fmt.Sprintf("internal %d, client %d", saved.Fullscreen, saved.FullscreenClient)})
			if !dryRun {
				argument := fmt.Sprintf("%d,%d,address:%s", saved.Fullscreen, saved.FullscreenClient, current.Address)
				if err := m.Compositor.Dispatch(ctx, "fullscreenstate", argument); err != nil {
					return result, err
				}
			}
		}
	}

	workspaceMonitor := map[int]string{}
	for _, saved := range snapshot.Windows {
		if saved.Workspace.ID > 0 && saved.Monitor != "" {
			workspaceMonitor[saved.Workspace.ID] = saved.Monitor
		}
	}
	workspaceMonitorIDs := make([]int, 0, len(workspaceMonitor))
	for workspaceID := range workspaceMonitor {
		workspaceMonitorIDs = append(workspaceMonitorIDs, workspaceID)
	}
	sort.Ints(workspaceMonitorIDs)
	for _, workspaceID := range workspaceMonitorIDs {
		monitor := workspaceMonitor[workspaceID]
		result.Operations = append(result.Operations, Operation{Kind: "workspace-monitor", Detail: fmt.Sprintf("%d -> %s", workspaceID, monitor)})
		if !dryRun {
			if err := m.Compositor.Dispatch(ctx, "moveworkspacetomonitor", fmt.Sprintf("%d %s", workspaceID, monitor)); err != nil {
				return result, err
			}
		}
	}

	workspaceWindows := map[int][]Window{}
	for _, saved := range snapshot.Windows {
		if saved.Layout != nil && !saved.Floating {
			workspaceWindows[saved.Workspace.ID] = append(workspaceWindows[saved.Workspace.ID], saved)
		}
	}
	workspaceIDs := make([]int, 0, len(workspaceWindows))
	for workspaceID := range workspaceWindows {
		workspaceIDs = append(workspaceIDs, workspaceID)
	}
	sort.Ints(workspaceIDs)
	for _, workspaceID := range workspaceIDs {
		savedWindows := workspaceWindows[workspaceID]
		complete := true
		for _, saved := range savedWindows {
			if _, ok := matches[saved.Slot]; !ok {
				complete = false
				break
			}
		}
		if !complete || !needsTopologyRestore(savedWindows) {
			continue
		}
		sort.SliceStable(savedWindows, func(i, j int) bool {
			if savedWindows[i].Layout.Column != savedWindows[j].Layout.Column {
				return savedWindows[i].Layout.Column < savedWindows[j].Layout.Column
			}
			return savedWindows[i].Layout.Row < savedWindows[j].Layout.Row
		})
		for _, saved := range savedWindows {
			current := matches[saved.Slot]
			result.Operations = append(result.Operations, Operation{Kind: "stage", Slot: saved.Slot, Detail: "special:danksession-staging"})
			if !dryRun {
				if err := m.Compositor.Dispatch(ctx, "movetoworkspacesilent", "special:danksession-staging,address:"+current.Address); err != nil {
					return result, err
				}
			}
		}
		for _, saved := range savedWindows {
			current := matches[saved.Slot]
			result.Operations = append(result.Operations, Operation{Kind: "place-column", Slot: saved.Slot, Detail: fmt.Sprintf("workspace %d column %d row %d", workspaceID, saved.Layout.Column, saved.Layout.Row)})
			if dryRun {
				continue
			}
			if err := m.Compositor.Dispatch(ctx, "movetoworkspacesilent", fmt.Sprintf("%d,address:%s", workspaceID, current.Address)); err != nil {
				return result, err
			}
			if saved.Layout.Row > 0 {
				if err := m.Compositor.Dispatch(ctx, "focuswindow", "address:"+current.Address); err != nil {
					return result, err
				}
				if err := m.Compositor.Dispatch(ctx, "layoutmsg", "movewindowto l"); err != nil {
					return result, err
				}
			}
		}
	}

	for _, saved := range snapshot.Windows {
		current, ok := matches[saved.Slot]
		if !ok || saved.Layout == nil || saved.Floating {
			continue
		}
		result.Operations = append(result.Operations, Operation{Kind: "column-width", Slot: saved.Slot, Detail: strconv.FormatFloat(saved.Layout.ColumnWidth, 'f', 3, 64)})
		if !dryRun && saved.Layout.Row == 0 {
			if err := m.Compositor.Dispatch(ctx, "focuswindow", "address:"+current.Address); err != nil {
				return result, err
			}
			if err := m.Compositor.Dispatch(ctx, "layoutmsg", fmt.Sprintf("colresize %.6f", saved.Layout.ColumnWidth)); err != nil {
				return result, err
			}
			if saved.Layout.Centered {
				if err := m.Compositor.Dispatch(ctx, "layoutmsg", "center"); err != nil {
					return result, err
				}
			}
		}
	}

	for _, monitor := range snapshot.Monitors {
		if monitor.Name == "" || monitor.ActiveWorkspace.ID <= 0 {
			continue
		}
		result.Operations = append(result.Operations, Operation{Kind: "active-workspace", Detail: fmt.Sprintf("%s -> %d", monitor.Name, monitor.ActiveWorkspace.ID)})
		if !dryRun {
			if err := m.Compositor.Dispatch(ctx, "focusmonitor", monitor.Name); err != nil {
				return result, err
			}
			if err := m.Compositor.Dispatch(ctx, "workspace", strconv.Itoa(monitor.ActiveWorkspace.ID)); err != nil {
				return result, err
			}
		}
	}

	for _, saved := range snapshot.Windows {
		if !saved.Focused {
			continue
		}
		if current, ok := matches[saved.Slot]; ok && !dryRun {
			if err := m.Compositor.Dispatch(ctx, "focuswindow", "address:"+current.Address); err != nil {
				return result, err
			}
		}
		break
	}
	return result, nil
}

func needsTopologyRestore(windows []Window) bool {
	if len(windows) > 1 {
		return true
	}
	return len(windows) == 1 && windows[0].Layout != nil && windows[0].Layout.Row > 0
}

func matchWindows(saved []Window, current []hyprland.Window, cfg Config) (map[string]hyprland.Window, []Window) {
	matches := map[string]hyprland.Window{}
	used := map[string]bool{}
	var missing []Window
	for _, slot := range saved {
		bestIndex, bestScore := -1, -1
		for i, candidate := range current {
			if used[candidate.Address] || !candidate.Mapped || candidate.Hidden {
				continue
			}
			score := matchScore(slot, candidate, cfg)
			if score > bestScore {
				bestIndex, bestScore = i, score
			}
		}
		if bestIndex < 0 || bestScore < 50 {
			missing = append(missing, slot)
			continue
		}
		matches[slot.Slot] = current[bestIndex]
		used[current[bestIndex].Address] = true
	}
	return matches, missing
}

func matchScore(saved Window, current hyprland.Window, cfg Config) int {
	score := 0
	if saved.Application != "" {
		app, ok := configuredApplication(saved.Application, cfg.Applications)
		if !ok || !matches(app.Match, current) {
			return -1
		}
		score += 100
	}
	if saved.InitialClass != "" && saved.InitialClass == current.InitialClass {
		score += 60
	}
	if saved.Class != "" && saved.Class == current.Class {
		score += 40
	}
	if saved.InitialTitle != "" && saved.InitialTitle == current.InitialTitle {
		score += 15
	}
	if saved.Title != "" && saved.Title == current.Title {
		score += 10
	}
	if saved.Workspace.ID == current.Workspace.ID {
		score += 5
	}
	return score
}

func applicationFor(window hyprland.Window, applications []Application) string {
	for _, app := range applications {
		if matches(app.Match, window) {
			return app.ID
		}
	}
	return ""
}

func configuredApplication(id string, applications []Application) (Application, bool) {
	for _, app := range applications {
		if app.ID == id {
			return app, true
		}
	}
	return Application{}, false
}

func excluded(window hyprland.Window, exclusions []Match) bool {
	for _, tag := range window.Tags {
		if strings.Contains(tag, "scratchpad") {
			return true
		}
	}
	for _, exclusion := range exclusions {
		if matches(exclusion, window) {
			return true
		}
	}
	return false
}

func matches(match Match, window hyprland.Window) bool {
	if match.Class == "" && match.InitialClass == "" && match.Title == "" {
		return false
	}
	return regexpMatches(match.Class, window.Class) && regexpMatches(match.InitialClass, window.InitialClass) && regexpMatches(match.Title, window.Title)
}

func regexpMatches(pattern, value string) bool {
	if pattern == "" {
		return true
	}
	matched, err := regexp.MatchString(pattern, value)
	return err == nil && matched
}

func validateMatch(match Match) error {
	if match.Class == "" && match.InitialClass == "" && match.Title == "" {
		return errors.New("match must include class, initialClass, or title")
	}
	for _, pattern := range []string{match.Class, match.InitialClass, match.Title} {
		if pattern == "" {
			continue
		}
		if _, err := regexp.Compile(pattern); err != nil {
			return fmt.Errorf("invalid regular expression %q: %w", pattern, err)
		}
	}
	return nil
}

func writeJSONAtomic(path string, value any) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	temporary, err := os.CreateTemp(filepath.Dir(path), ".last-*.json")
	if err != nil {
		return err
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if err := temporary.Chmod(0o600); err != nil {
		temporary.Close()
		return err
	}
	if _, err := temporary.Write(append(data, '\n')); err != nil {
		temporary.Close()
		return err
	}
	if err := temporary.Sync(); err != nil {
		temporary.Close()
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	return os.Rename(temporaryPath, path)
}

func launch(command []string) error {
	cmd := exec.Command(command[0], command[1:]...)
	cmd.Stdin = nil
	cmd.Stdout = nil
	cmd.Stderr = nil
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := cmd.Start(); err != nil {
		return err
	}
	return cmd.Process.Release()
}

func snapWidth(value float64) float64 {
	for _, preset := range []float64{0.25, 0.333, 0.5, 0.666, 1} {
		if math.Abs(value-preset) <= 0.02 {
			return preset
		}
	}
	return math.Round(value*1000) / 1000
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

func abs(value int) int {
	if value < 0 {
		return -value
	}
	return value
}
