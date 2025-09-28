package hyprctl

import "testing"

func TestDecodeClientsPayload(t *testing.T) {
	tests := []struct {
		name      string
		payload   string
		wantLen   int
		wantErr   bool
		wantFirst string
	}{
		{
			name:      "single client",
			payload:   `[{"address":"0xabc","workspace":{"name":"code-alpha"}}]`,
			wantLen:   1,
			wantFirst: "code-alpha",
		},
		{
			name:    "empty array",
			payload: `[]`,
			wantLen: 0,
		},
		{
			name:    "empty payload",
			payload: ``,
			wantLen: 0,
		},
		{
			name:    "invalid json",
			payload: `{]`,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var clients []ClientInfo
			err := DecodeClientsPayload([]byte(tt.payload), &clients)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(clients) != tt.wantLen {
				t.Fatalf("expected %d clients, got %d", tt.wantLen, len(clients))
			}
			if tt.wantLen > 0 && tt.wantFirst != "" {
				if got := clients[0].WorkspaceName(); got != tt.wantFirst {
					t.Fatalf("expected workspace %q, got %q", tt.wantFirst, got)
				}
			}
		})
	}
}

func TestDecodeClientsPayloadNilTarget(t *testing.T) {
	if err := DecodeClientsPayload([]byte(`[]`), nil); err == nil {
		t.Fatalf("expected error for nil target")
	}
}

func TestParseClients(t *testing.T) {
	payload := `[{"address":"0xabc","workspace":{"id":3}}]`
	clients, err := ParseClients([]byte(payload))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(clients) != 1 {
		t.Fatalf("expected 1 client, got %d", len(clients))
	}
	if got := clients[0].WorkspaceName(); got != "3" {
		t.Fatalf("expected derived workspace name 3, got %q", got)
	}
}
