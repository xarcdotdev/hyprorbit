package events

import (
	"errors"
	"testing"
)

func TestParseEvent(t *testing.T) {
	tests := []struct {
		name       string
		input      string
		wantType   EventType
		wantWSID   string
		wantWSName string
		wantMon    string
		wantWin    string
		wantFields int
		wantErr    error
	}{
		{
			name:       "workspace legacy",
			input:      "workspace>>1",
			wantType:   TypeWorkspace,
			wantWSID:   "1",
			wantWSName: "1",
		},
		{
			name:       "workspace v2",
			input:      "workspacev2>>2,dev",
			wantType:   TypeWorkspaceV2,
			wantWSID:   "2",
			wantWSName: "dev",
		},
		{
			name:       "active workspace",
			input:      "activeworkspace>>alpha",
			wantType:   TypeActiveWorkspace,
			wantWSID:   "alpha",
			wantWSName: "alpha",
		},
		{
			name:       "focused monitor",
			input:      "focusedmon>>HDMI-A-1,3",
			wantType:   TypeFocusedMonitor,
			wantMon:    "HDMI-A-1",
			wantWSID:   "3",
			wantWSName: "3",
		},
		{
			name:       "active window",
			input:      "activewindow>>0x123,kitty,Terminal",
			wantType:   TypeActiveWindow,
			wantWin:    "0x123",
			wantFields: 3,
		},
		{
			name:     "unknown event type",
			input:    "custom_event>>payload",
			wantType: EventType("custom_event"),
		},
		{
			name:    "missing separator",
			input:   "invalid payload",
			wantErr: ErrMalformedEvent,
		},
		{
			name:    "empty payload",
			input:   "  \t  ",
			wantErr: ErrEmptyEvent,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			ev, err := ParseEvent(tt.input)
			if tt.wantErr != nil {
				if err == nil {
					t.Fatalf("expected error %v, got nil", tt.wantErr)
				}
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("expected error %v, got %v", tt.wantErr, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if ev.Type != tt.wantType {
				t.Fatalf("expected type %q, got %q", tt.wantType, ev.Type)
			}
			if tt.wantWSID != "" {
				if ev.Workspace == nil {
					t.Fatalf("expected workspace info")
				}
				if ev.Workspace.ID != tt.wantWSID {
					t.Fatalf("expected workspace ID %q, got %q", tt.wantWSID, ev.Workspace.ID)
				}
				if ev.Workspace.Name != tt.wantWSName {
					t.Fatalf("expected workspace name %q, got %q", tt.wantWSName, ev.Workspace.Name)
				}
				if tt.wantMon != "" && ev.Workspace.Monitor != tt.wantMon {
					t.Fatalf("expected workspace monitor %q, got %q", tt.wantMon, ev.Workspace.Monitor)
				}
			}
			if tt.wantMon != "" {
				if ev.Monitor == nil {
					t.Fatalf("expected monitor info")
				}
				if ev.Monitor.Name != tt.wantMon {
					t.Fatalf("expected monitor name %q, got %q", tt.wantMon, ev.Monitor.Name)
				}
			}
			if tt.wantWin != "" {
				if ev.Window == nil {
					t.Fatalf("expected window info")
				}
				if ev.Window.Address != tt.wantWin {
					t.Fatalf("expected window address %q, got %q", tt.wantWin, ev.Window.Address)
				}
				if len(ev.Window.Fields) != tt.wantFields {
					t.Fatalf("expected %d window fields, got %d", tt.wantFields, len(ev.Window.Fields))
				}
			}
		})
	}
}

func TestParseFocusedMonitorMissingWorkspace(t *testing.T) {
	ev, err := ParseEvent("focusedmon>>HDMI-A-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ev.Monitor == nil || ev.Monitor.Name != "HDMI-A-1" {
		t.Fatalf("expected monitor HDMI-A-1, got %#v", ev.Monitor)
	}
	if ev.Workspace != nil {
		t.Fatalf("expected no workspace, got %#v", ev.Workspace)
	}
}
