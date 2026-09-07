package session

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"sort"
	"strconv"

	"github.com/alcxyz/DankSession/internal/hyprland"
)

var ErrStaleExclusions = errors.New("application exclusions changed; reload and try again")

type ExclusionRule struct {
	Index int `json:"index"`
	Match
}

// OpenApplication deliberately exposes no titles, addresses, PIDs, or commands.
type OpenApplication struct {
	Class        string   `json:"class"`
	InitialClass string   `json:"initialClass"`
	Windows      int      `json:"windows"`
	Workspaces   []string `json:"workspaces"`
}

type ExclusionSettings struct {
	Revision     string            `json:"revision"`
	Rules        []ExclusionRule   `json:"rules"`
	Applications []OpenApplication `json:"applications"`
}

type ExclusionPreview struct {
	Windows      int               `json:"windows"`
	Applications []OpenApplication `json:"applications"`
}

type ExclusionUpdate struct {
	Action   string `json:"action"`
	Revision string `json:"revision"`
	Index    *int   `json:"index,omitempty"`
	Match    Match  `json:"match"`
}

// UpdateConfig coordinates all read-modify-write configuration changes across
// CLI and widget processes. Callers must not use SaveConfig for such updates.
func (m *Manager) UpdateConfig(update func(*Config) error) error {
	unlock, err := lockFile(m.ConfigPath + ".lock")
	if err != nil {
		return err
	}
	defer unlock()
	cfg, err := m.LoadConfig()
	if err != nil {
		return err
	}
	if err := update(&cfg); err != nil {
		return err
	}
	if err := validateConfig(cfg); err != nil {
		return err
	}
	return m.SaveConfig(cfg)
}

func (m *Manager) ExclusionSettings(ctx context.Context) (ExclusionSettings, error) {
	cfg, err := m.LoadConfig()
	if err != nil {
		return ExclusionSettings{}, err
	}
	desktop, err := m.Compositor.Desktop(ctx)
	if err != nil {
		return ExclusionSettings{}, err
	}
	if err := ctx.Err(); err != nil {
		return ExclusionSettings{}, err
	}
	return exclusionSettings(cfg, desktop), nil
}

func (m *Manager) PreviewExclusion(ctx context.Context, match Match) (ExclusionPreview, error) {
	if err := validateMatch(match); err != nil {
		return ExclusionPreview{}, err
	}
	desktop, err := m.Compositor.Desktop(ctx)
	if err != nil {
		return ExclusionPreview{}, err
	}
	if err := ctx.Err(); err != nil {
		return ExclusionPreview{}, err
	}
	windows := make([]hyprland.Window, 0)
	for _, window := range desktop.Windows {
		if window.Mapped && matches(match, window) {
			windows = append(windows, window)
		}
	}
	return ExclusionPreview{Windows: len(windows), Applications: openApplications(windows)}, nil
}

func (m *Manager) UpdateExclusion(ctx context.Context, request ExclusionUpdate) (ExclusionSettings, error) {
	if request.Revision == "" {
		return ExclusionSettings{}, errors.New("exclusion revision is required; reload and try again")
	}
	switch request.Action {
	case "add", "update":
		if err := validateMatch(request.Match); err != nil {
			return ExclusionSettings{}, err
		}
	case "remove":
	default:
		return ExclusionSettings{}, errors.New("exclusion action must be add, update, or remove")
	}
	if request.Action == "add" && request.Index != nil {
		return ExclusionSettings{}, errors.New("adding an exclusion must not include an index")
	}
	// Query first: a disconnected compositor must not turn a successful write
	// into an ambiguous error when returning the refreshed application list.
	desktop, err := m.Compositor.Desktop(ctx)
	if err != nil {
		return ExclusionSettings{}, err
	}
	var result ExclusionSettings
	err = m.UpdateConfig(func(cfg *Config) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		if request.Revision != exclusionRevision(cfg.Exclude) {
			return ErrStaleExclusions
		}
		index := -1
		if request.Action != "add" {
			if request.Index == nil || *request.Index < 0 || *request.Index >= len(cfg.Exclude) {
				return errors.New("exclusion index is invalid; reload and try again")
			}
			index = *request.Index
		}
		if request.Action != "remove" {
			for i, existing := range cfg.Exclude {
				if i != index && existing.Class == request.Match.Class && existing.InitialClass == request.Match.InitialClass && existing.Title == request.Match.Title {
					return errors.New("an exclusion with this match already exists")
				}
			}
		}
		switch request.Action {
		case "add":
			cfg.Exclude = append(cfg.Exclude, request.Match)
		case "update":
			cfg.Exclude[index] = request.Match
		case "remove":
			cfg.Exclude = append(cfg.Exclude[:index], cfg.Exclude[index+1:]...)
		}
		result = exclusionSettings(*cfg, desktop)
		return nil
	})
	if err != nil {
		return ExclusionSettings{}, err
	}
	return result, nil
}

func exclusionRevision(exclusions []Match) string {
	// Normalize nil and [] so formatting or unrelated preference updates do not
	// invalidate an exclusion editor. Exclusion order and disabled state matter.
	if exclusions == nil {
		exclusions = []Match{}
	}
	data, _ := json.Marshal(exclusions)
	digest := sha256.Sum256(data)
	return hex.EncodeToString(digest[:])
}

func exclusionSettings(cfg Config, desktop hyprland.Desktop) ExclusionSettings {
	rules := make([]ExclusionRule, 0, len(cfg.Exclude))
	for i, match := range cfg.Exclude {
		rules = append(rules, ExclusionRule{Index: i, Match: match})
	}
	return ExclusionSettings{Revision: exclusionRevision(cfg.Exclude), Rules: rules, Applications: openApplications(desktop.Windows)}
}

func excludedSaved(saved Window, exclusions []Match) bool {
	window := hyprland.Window{Class: saved.Class, InitialClass: saved.InitialClass, Title: saved.Title}
	if excluded(window, exclusions) {
		return true
	}
	if saved.Title != "" {
		return false
	}
	// Titles are private and not captured by default. An unknown historical
	// title must not let a new exclusion relaunch or reposition its application.
	// A title-only exclusion necessarily covers every unknown-title entry.
	for _, rule := range exclusions {
		if !rule.Disabled && rule.Title != "" && regexpMatches(rule.Class, saved.Class) && regexpMatches(rule.InitialClass, saved.InitialClass) {
			return true
		}
	}
	return false
}

func excludedLiveMatch(saved Window, windows []hyprland.Window, cfg Config) bool {
	// A title may have changed since capture. Filtering the live window must
	// not make a running, excluded application look absent and launch it again.
	for _, window := range windows {
		if window.Mapped && excluded(window, cfg.Exclude) && matchScore(saved, window, cfg) >= 50 {
			return true
		}
	}
	return false
}

func openApplications(windows []hyprland.Window) []OpenApplication {
	type identity struct{ class, initialClass string }
	groups := map[identity]*OpenApplication{}
	for _, window := range windows {
		if !window.Mapped {
			continue
		}
		key := identity{window.Class, window.InitialClass}
		app, exists := groups[key]
		if !exists {
			app = &OpenApplication{Class: key.class, InitialClass: key.initialClass, Workspaces: []string{}}
			groups[key] = app
		}
		app.Windows++
		workspace := window.Workspace.Name
		if workspace == "" {
			workspace = strconv.Itoa(window.Workspace.ID)
		}
		seen := false
		for _, existing := range app.Workspaces {
			if workspace == existing {
				seen = true
				break
			}
		}
		if !seen {
			app.Workspaces = append(app.Workspaces, workspace)
		}
	}
	apps := make([]OpenApplication, 0, len(groups))
	for _, app := range groups {
		sort.Slice(app.Workspaces, func(i, j int) bool {
			a, aerr := strconv.Atoi(app.Workspaces[i])
			b, berr := strconv.Atoi(app.Workspaces[j])
			if aerr == nil && berr == nil {
				return a < b
			}
			if (aerr == nil) != (berr == nil) {
				return aerr == nil
			}
			return app.Workspaces[i] < app.Workspaces[j]
		})
		apps = append(apps, *app)
	}
	sort.Slice(apps, func(i, j int) bool {
		if apps[i].InitialClass != apps[j].InitialClass {
			return apps[i].InitialClass < apps[j].InitialClass
		}
		return apps[i].Class < apps[j].Class
	})
	return apps
}
