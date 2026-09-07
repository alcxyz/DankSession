package hyprland

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

type Point [2]int

type WorkspaceRef struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

type Window struct {
	Address          string       `json:"address"`
	Mapped           bool         `json:"mapped"`
	Hidden           bool         `json:"hidden"`
	At               Point        `json:"at"`
	Size             Point        `json:"size"`
	Workspace        WorkspaceRef `json:"workspace"`
	Floating         bool         `json:"floating"`
	Monitor          int          `json:"monitor"`
	Class            string       `json:"class"`
	Title            string       `json:"title"`
	InitialClass     string       `json:"initialClass"`
	InitialTitle     string       `json:"initialTitle"`
	PID              int          `json:"pid"`
	Pinned           bool         `json:"pinned"`
	Fullscreen       int          `json:"fullscreen"`
	FullscreenClient int          `json:"fullscreenClient"`
	FocusHistory     int          `json:"focusHistoryID"`
	Tags             []string     `json:"tags"`
	StableID         string       `json:"stableId"`
}

type Monitor struct {
	ID              int          `json:"id"`
	Name            string       `json:"name"`
	X               int          `json:"x"`
	Y               int          `json:"y"`
	Width           int          `json:"width"`
	Height          int          `json:"height"`
	Scale           float64      `json:"scale"`
	Transform       int          `json:"transform"`
	Focused         bool         `json:"focused"`
	ActiveWorkspace WorkspaceRef `json:"activeWorkspace"`
}

func (m Monitor) LogicalWidth() int {
	width := m.Width
	if m.Transform%2 != 0 {
		width = m.Height
	}
	scale := m.Scale
	if scale <= 0 {
		scale = 1
	}
	return max(1, int(float64(width)/scale))
}

type Desktop struct {
	Windows      []Window
	Monitors     []Monitor
	Workspaces   []Workspace
	ActiveWindow Window
}

type Workspace struct {
	ID          int    `json:"id"`
	TiledLayout string `json:"tiledLayout"`
}

type Client struct {
	Hyprctl string

	mu       sync.Mutex
	instance string
}

func NewClient() *Client {
	return &Client{Hyprctl: "hyprctl"}
}

func (c *Client) Instance(ctx context.Context) (string, error) {
	return c.liveInstance(ctx)
}

func (c *Client) query(ctx context.Context, command string, target any) error {
	args, err := c.args(ctx, command, "-j")
	if err != nil {
		return err
	}
	out, err := exec.CommandContext(ctx, c.Hyprctl, args...).Output()
	if err != nil {
		return fmt.Errorf("hyprctl %s: %w", command, err)
	}
	if err := json.Unmarshal(out, target); err != nil {
		return fmt.Errorf("decode hyprctl %s: %w", command, err)
	}
	return nil
}

func (c *Client) Desktop(ctx context.Context) (Desktop, error) {
	var desktop Desktop
	if err := c.query(ctx, "clients", &desktop.Windows); err != nil {
		return Desktop{}, err
	}
	if err := c.query(ctx, "monitors", &desktop.Monitors); err != nil {
		return Desktop{}, err
	}
	if err := c.query(ctx, "workspaces", &desktop.Workspaces); err != nil {
		return Desktop{}, err
	}
	if err := c.query(ctx, "activewindow", &desktop.ActiveWindow); err != nil {
		return Desktop{}, err
	}
	return desktop, nil
}

func (c *Client) Dispatch(ctx context.Context, dispatcher, argument string) error {
	expression, err := dispatchExpression(dispatcher, argument)
	if err != nil {
		return err
	}
	args, err := c.args(ctx, "dispatch", expression)
	if err != nil {
		return err
	}
	out, err := exec.CommandContext(ctx, c.Hyprctl, args...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("hyprctl %s: %w: %s", dispatcher, err, strings.TrimSpace(string(out)))
	}
	if response := strings.TrimSpace(string(out)); response != "" && response != "ok" {
		return fmt.Errorf("hyprctl %s: %s", dispatcher, response)
	}
	return nil
}

func dispatchExpression(dispatcher, argument string) (string, error) {
	window := func(value string) string { return strconv.Quote(value) }
	workspace := func(value string) string {
		if id, err := strconv.Atoi(value); err == nil && id > 0 {
			return strconv.Itoa(id)
		}
		return strconv.Quote(value)
	}

	switch dispatcher {
	case "focuswindow":
		if argument == "" {
			return "", errors.New("focus window requires a selector")
		}
		return fmt.Sprintf("hl.dsp.focus({ window = %s })", window(argument)), nil
	case "focusmonitor":
		if argument == "" {
			return "", errors.New("focus monitor requires a name")
		}
		return fmt.Sprintf("hl.dsp.focus({ monitor = %s })", strconv.Quote(argument)), nil
	case "workspace":
		if argument == "" {
			return "", errors.New("focus workspace requires a selector")
		}
		return fmt.Sprintf("hl.dsp.focus({ workspace = %s })", workspace(argument)), nil
	case "movetoworkspacesilent":
		separator := strings.LastIndex(argument, ",address:")
		if separator <= 0 {
			return "", fmt.Errorf("invalid window workspace move %q", argument)
		}
		target, selector := argument[:separator], argument[separator+1:]
		return fmt.Sprintf("hl.dsp.window.move({ workspace = %s, window = %s, follow = false })", workspace(target), window(selector)), nil
	case "moveworkspacetomonitor":
		separator := strings.LastIndex(argument, " ")
		if separator <= 0 || separator == len(argument)-1 {
			return "", fmt.Errorf("invalid workspace monitor move %q", argument)
		}
		target, monitor := argument[:separator], argument[separator+1:]
		return fmt.Sprintf("hl.dsp.workspace.move({ workspace = %s, monitor = %s })", workspace(target), strconv.Quote(monitor)), nil
	case "setfloating", "settiled":
		action := "set"
		if dispatcher == "settiled" {
			action = "unset"
		}
		return fmt.Sprintf("hl.dsp.window.float({ action = %q, window = %s })", action, window(argument)), nil
	case "pin", "unpin":
		action := "set"
		if dispatcher == "unpin" {
			action = "unset"
		}
		return fmt.Sprintf("hl.dsp.window.pin({ action = %q, window = %s })", action, window(argument)), nil
	case "fullscreenstate":
		parts := strings.Split(argument, ",")
		if len(parts) != 3 {
			return "", fmt.Errorf("invalid fullscreen state %q", argument)
		}
		internal, err := strconv.Atoi(parts[0])
		if err != nil || internal < 0 || internal > 2 {
			return "", fmt.Errorf("invalid internal fullscreen state %q", parts[0])
		}
		client, err := strconv.Atoi(parts[1])
		if err != nil || client < 0 || client > 2 {
			return "", fmt.Errorf("invalid client fullscreen state %q", parts[1])
		}
		return fmt.Sprintf("hl.dsp.window.fullscreen_state({ internal = %d, client = %d, action = \"set\", window = %s })", internal, client, window(parts[2])), nil
	case "resizewindowpixel", "movewindowpixel":
		geometry, selector, ok := strings.Cut(argument, ",")
		if !ok || selector == "" {
			return "", fmt.Errorf("invalid window geometry %q", argument)
		}
		fields := strings.Fields(geometry)
		if len(fields) != 3 || fields[0] != "exact" {
			return "", fmt.Errorf("invalid exact geometry %q", geometry)
		}
		x, err := strconv.Atoi(fields[1])
		if err != nil {
			return "", fmt.Errorf("invalid geometry x %q", fields[1])
		}
		y, err := strconv.Atoi(fields[2])
		if err != nil {
			return "", fmt.Errorf("invalid geometry y %q", fields[2])
		}
		method := "resize"
		if dispatcher == "movewindowpixel" {
			method = "move"
		}
		return fmt.Sprintf("hl.dsp.window.%s({ x = %d, y = %d, relative = false, window = %s })", method, x, y, window(selector)), nil
	case "layoutmsg":
		return fmt.Sprintf("hl.dsp.layout(%s)", strconv.Quote(argument)), nil
	default:
		return "", fmt.Errorf("unsupported Hyprland dispatcher %q", dispatcher)
	}
}

func (c *Client) Events(ctx context.Context) (<-chan string, <-chan error) {
	events := make(chan string)
	errorsOut := make(chan error, 1)

	go func() {
		defer close(events)
		defer close(errorsOut)

		instance, err := c.liveInstance(ctx)
		if err != nil {
			errorsOut <- err
			return
		}
		socket, err := eventSocket(instance)
		if err != nil {
			errorsOut <- err
			return
		}

		conn, err := net.DialTimeout("unix", socket, 5*time.Second)
		if err != nil {
			errorsOut <- fmt.Errorf("connect Hyprland event socket: %w", err)
			return
		}
		defer conn.Close()
		done := make(chan struct{})
		defer close(done)

		go func() {
			select {
			case <-ctx.Done():
				_ = conn.Close()
			case <-done:
			}
		}()

		scanner := bufio.NewScanner(conn)
		for scanner.Scan() {
			select {
			case events <- scanner.Text():
			case <-ctx.Done():
				return
			}
		}
		if err := scanner.Err(); err != nil && !errors.Is(err, net.ErrClosed) && ctx.Err() == nil {
			errorsOut <- fmt.Errorf("read Hyprland event socket: %w", err)
		}
	}()

	return events, errorsOut
}

func (c *Client) args(ctx context.Context, args ...string) ([]string, error) {
	instance, err := c.liveInstance(ctx)
	if err != nil {
		return nil, err
	}
	return append([]string{"-i", instance}, args...), nil
}

func (c *Client) liveInstance(ctx context.Context) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.instance != "" && controlSocketExists(c.instance) {
		return c.instance, nil
	}
	// Respect the caller's session; never choose a nested compositor merely
	// because it was started more recently.
	if requested := os.Getenv("HYPRLAND_INSTANCE_SIGNATURE"); requested != "" {
		if !controlSocketExists(requested) {
			return "", errors.New("the requested Hyprland instance is not available")
		}
		c.instance = requested
		return c.instance, nil
	}

	type instance struct {
		Signature string `json:"instance"`
		Time      int64  `json:"time"`
	}
	out, err := exec.CommandContext(ctx, c.Hyprctl, "instances", "-j").Output()
	if err != nil {
		return "", fmt.Errorf("list Hyprland instances: %w", err)
	}
	var instances []instance
	if err := json.Unmarshal(out, &instances); err != nil {
		return "", fmt.Errorf("decode Hyprland instances: %w", err)
	}
	sort.Slice(instances, func(i, j int) bool { return instances[i].Time > instances[j].Time })
	for _, candidate := range instances {
		if candidate.Signature != "" && controlSocketExists(candidate.Signature) {
			c.instance = candidate.Signature
			return c.instance, nil
		}
	}
	return "", errors.New("no live Hyprland instance found")
}

func controlSocketExists(instance string) bool {
	runtimeDir := os.Getenv("XDG_RUNTIME_DIR")
	if runtimeDir == "" {
		return false
	}
	_, err := os.Stat(filepath.Join(runtimeDir, "hypr", instance, ".socket.sock"))
	return err == nil
}

func eventSocket(instance string) (string, error) {
	runtimeDir := os.Getenv("XDG_RUNTIME_DIR")
	if runtimeDir == "" || instance == "" {
		return "", errors.New("XDG_RUNTIME_DIR and a Hyprland instance are required")
	}
	return filepath.Join(runtimeDir, "hypr", instance, ".socket2.sock"), nil
}
