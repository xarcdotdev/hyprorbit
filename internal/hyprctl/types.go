package hyprctl

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

// ClientInfo represents a Hyprland client entry returned by `hyprctl clients -j`.
type ClientInfo struct {
	Address      string          `json:"address"`
	Class        string          `json:"class"`
	Title        string          `json:"title"`
	InitialClass string          `json:"initialClass"`
	InitialTitle string          `json:"initialTitle"`
	Floating     bool            `json:"floating"`
	Workspace    WorkspaceHandle `json:"workspace"`
}

// WorkspaceHandle captures the minimal workspace metadata attached to a client.
type WorkspaceHandle struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

// Workspace describes a workspace record returned by Hyprland.
type Workspace struct {
	ID              int    `json:"id"`
	Name            string `json:"name"`
	Monitor         string `json:"monitor"`
	Windows         int    `json:"windows"`
	HasFullscreen   bool   `json:"hasfullscreen"`
	LastWindow      string `json:"lastwindow"`
	LastWindowTitle string `json:"lastwindowtitle"`
}

// Monitor describes a monitor entry returned by Hyprland.
type Monitor struct {
	ID              int       `json:"id"`
	Name            string    `json:"name"`
	Description     string    `json:"description"`
	Make            string    `json:"make"`
	Model           string    `json:"model"`
	Serial          string    `json:"serial"`
	Width           int       `json:"width"`
	Height          int       `json:"height"`
	RefreshRate     float64   `json:"refreshRate"`
	X               int       `json:"x"`
	Y               int       `json:"y"`
	ActiveWorkspace Workspace `json:"activeWorkspace"`
	Reserved        [4]int    `json:"reserved"`
	Scale           float64   `json:"scale"`
	Transform       int       `json:"transform"`
	Focused         bool      `json:"focused"`
	DpmsStatus      bool      `json:"dpmsStatus"`
}

// Window represents a detailed Hyprland window entry.
type Window struct {
	Address        string    `json:"address"`
	At             [2]int    `json:"at"`
	Size           [2]int    `json:"size"`
	Workspace      Workspace `json:"workspace"`
	Floating       HyprBool  `json:"floating"`
	Monitor        int       `json:"monitor"`
	Class          string    `json:"class"`
	Title          string    `json:"title"`
	InitialClass   string    `json:"initialClass"`
	InitialTitle   string    `json:"initialTitle"`
	Pid            int       `json:"pid"`
	Xwayland       bool      `json:"xwayland"`
	Pinned         HyprBool  `json:"pinned"`
	Fullscreen     HyprBool  `json:"fullscreen"`
	FakeFullscreen HyprBool  `json:"fakeFullscreen"`
	Grouped        []string  `json:"grouped"`
	Swallowing     string    `json:"swallowing"`
}

// HyprBool accepts boolean values encoded as booleans, numbers, or strings.
type HyprBool bool

// UnmarshalJSON normalizes HyprBool inputs from Hyprland responses.
func (b *HyprBool) UnmarshalJSON(data []byte) error {
	data = bytes.TrimSpace(data)
	if len(data) == 0 {
		*b = HyprBool(false)
		return nil
	}
	switch data[0] {
	case 't', 'T', 'f', 'F':
		var value bool
		if err := json.Unmarshal(data, &value); err != nil {
			return err
		}
		*b = HyprBool(value)
		return nil
	case '"':
		var text string
		if err := json.Unmarshal(data, &text); err != nil {
			return err
		}
		text = strings.TrimSpace(text)
		if text == "" {
			*b = HyprBool(false)
			return nil
		}
		if s, err := strconv.Atoi(text); err == nil {
			*b = HyprBool(s != 0)
			return nil
		}
		lower := strings.ToLower(text)
		if lower == "true" {
			*b = HyprBool(true)
			return nil
		}
		if lower == "false" {
			*b = HyprBool(false)
			return nil
		}
		if lower == "1" {
			*b = HyprBool(true)
			return nil
		}
		if lower == "0" {
			*b = HyprBool(false)
			return nil
		}
		return fmt.Errorf("hyprbool: unsupported string value %q", text)
	default:
		var num int
		if err := json.Unmarshal(data, &num); err == nil {
			*b = HyprBool(num != 0)
			return nil
		}
		var floatVal float64
		if err := json.Unmarshal(data, &floatVal); err == nil {
			*b = HyprBool(floatVal != 0)
			return nil
		}
		return fmt.Errorf("hyprbool: unsupported value %s", string(data))
	}
}

// WorkspaceName returns a stable workspace identifier for the client.
func (c ClientInfo) WorkspaceName() string {
	if c.Workspace.Name != "" {
		return c.Workspace.Name
	}
	if c.Workspace.ID != 0 {
		return fmt.Sprintf("%d", c.Workspace.ID)
	}
	return ""
}

// FieldValue extracts a supported metadata field for matching purposes.
func (c ClientInfo) FieldValue(field string) string {
	switch strings.ToLower(field) {
	case "class":
		return c.Class
	case "title":
		return c.Title
	case "initialclass":
		return c.InitialClass
	case "initialtitle":
		return c.InitialTitle
	default:
		return ""
	}
}

// DecodeClientsPayload normalises client JSON payload decoding.
func DecodeClientsPayload(data []byte, out any) error {
	return decodePayload(data, out, "clients")
}

// ParseClients converts a raw clients payload into typed client entries.
func ParseClients(data []byte) ([]ClientInfo, error) {
	var clients []ClientInfo
	if err := DecodeClientsPayload(data, &clients); err != nil {
		return nil, err
	}
	if clients == nil {
		return []ClientInfo{}, nil
	}
	return clients, nil
}

func decodePayload(data []byte, out any, resource string) error {
	if out == nil {
		return fmt.Errorf("hyprctl: decode %s: target is nil", resource)
	}
	if len(data) == 0 {
		return nil
	}
	if err := json.Unmarshal(data, out); err != nil {
		return fmt.Errorf("hyprctl: decode %s: %w", resource, err)
	}
	return nil
}
