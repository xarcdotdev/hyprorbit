package events

import (
	"errors"
	"fmt"
	"strings"
)

// EventType identifies the Hyprland event emitted on the socket stream.
type EventType string

const (
	// TypeUnknown captures events that do not map to a known Hyprland category.
	TypeUnknown EventType = ""
	// TypeWorkspace is emitted when the active workspace changes (legacy format).
	TypeWorkspace EventType = "workspace"
	// TypeWorkspaceV2 carries workspace updates with both ID and name.
	TypeWorkspaceV2 EventType = "workspacev2"
	// TypeActiveWorkspace mirrors TypeWorkspace but is emitted for different hooks.
	TypeActiveWorkspace EventType = "activeworkspace"
	// TypeFocusedMonitor is sent when focus moves between monitors.
	TypeFocusedMonitor EventType = "focusedmon"
	// TypeActiveWindow is produced when the focused window changes.
	TypeActiveWindow EventType = "activewindow"
)

// Event represents a single Hyprland socket notification.
type Event struct {
	Type    EventType
	Raw     string
	Payload string

	Workspace *Workspace
	Monitor   *Monitor
	Window    *Window
}

// Workspace captures workspace-centric event metadata.
type Workspace struct {
	ID      string
	Name    string
	Monitor string
}

// Monitor describes which monitor is referenced by an event.
type Monitor struct {
	Name string
}

// Window carries information about the active window event payload.
type Window struct {
	Address string
	Fields  []string
}

var (
	// ErrEmptyEvent indicates an empty line was encountered on the socket stream.
	ErrEmptyEvent = errors.New("hyprland event: empty payload")
	// ErrMalformedEvent indicates the event line could not be parsed due to formatting issues.
	ErrMalformedEvent = errors.New("hyprland event: malformed payload")
)

// ParseEvent converts a raw Hyprland event line into a structured Event.
func ParseEvent(line string) (Event, error) {
	trimmed := strings.TrimSpace(line)
	if trimmed == "" {
		return Event{}, ErrEmptyEvent
	}

	parts := strings.SplitN(trimmed, ">>", 2)
	if len(parts) != 2 {
		return Event{}, fmt.Errorf("%w: %q", ErrMalformedEvent, trimmed)
	}

	rawType := strings.TrimSpace(parts[0])
	payload := strings.TrimSpace(parts[1])

	ev := Event{
		Type:    EventType(rawType),
		Raw:     trimmed,
		Payload: payload,
	}

	switch ev.Type {
	case TypeWorkspace, TypeActiveWorkspace:
		if ws := parseWorkspaceSimple(payload); ws != nil {
			ev.Workspace = ws
		}
	case TypeWorkspaceV2:
		if ws := parseWorkspaceV2(payload); ws != nil {
			ev.Workspace = ws
		}
	case TypeFocusedMonitor:
		mon, ws := parseFocusedMonitor(payload)
		if mon != nil {
			ev.Monitor = mon
		}
		if ws != nil {
			ev.Workspace = ws
		}
	case TypeActiveWindow:
		if win := parseActiveWindow(payload); win != nil {
			ev.Window = win
		}
	default:
		if rawType == "" {
			ev.Type = TypeUnknown
		}
	}

	return ev, nil
}

func parseWorkspaceSimple(payload string) *Workspace {
	id := strings.TrimSpace(payload)
	if id == "" {
		return nil
	}
	return &Workspace{ID: id, Name: id}
}

func parseWorkspaceV2(payload string) *Workspace {
	if payload == "" {
		return nil
	}
	parts := strings.SplitN(payload, ",", 2)
	id := strings.TrimSpace(parts[0])
	if id == "" {
		return nil
	}
	ws := &Workspace{ID: id, Name: id}
	if len(parts) > 1 {
		name := strings.TrimSpace(parts[1])
		if name != "" {
			ws.Name = name
		}
	}
	return ws
}

func parseFocusedMonitor(payload string) (*Monitor, *Workspace) {
	if payload == "" {
		return nil, nil
	}
	parts := strings.SplitN(payload, ",", 2)
	monitorName := strings.TrimSpace(parts[0])
	var workspace *Workspace
	if len(parts) > 1 {
		wsID := strings.TrimSpace(parts[1])
		if wsID != "" {
			workspace = &Workspace{ID: wsID, Name: wsID, Monitor: monitorName}
		}
	}

	if monitorName == "" {
		return nil, workspace
	}
	mon := &Monitor{Name: monitorName}
	if workspace != nil && workspace.Monitor == "" {
		workspace.Monitor = monitorName
	}
	return mon, workspace
}

func parseActiveWindow(payload string) *Window {
	if payload == "" {
		return nil
	}
	fields := splitAndTrim(payload, ",")
	if len(fields) == 0 {
		return nil
	}
	return &Window{Address: fields[0], Fields: fields}
}

func splitAndTrim(payload, sep string) []string {
	raw := strings.Split(payload, sep)
	out := make([]string, 0, len(raw))
	for _, part := range raw {
		val := strings.TrimSpace(part)
		if val == "" {
			continue
		}
		out = append(out, val)
	}
	return out
}
