package ipc

import "encoding/json"

// Version identifies the IPC protocol version used by clients and daemon.
const Version uint16 = 1

// Request describes the payload sent from the control client to the daemon.
type Request struct {
	Version uint16         `json:"version"`
	Command string         `json:"command"`
	Action  string         `json:"action"`
	Args    []string       `json:"args,omitempty"`
	Flags   map[string]any `json:"flags,omitempty"`
}

// Response represents the reply emitted by the daemon for a given request.
type Response struct {
	Version  uint16          `json:"version"`
	Success  bool            `json:"success"`
	Data     json.RawMessage `json:"data,omitempty"`
	Error    string          `json:"error,omitempty"`
	ExitCode int             `json:"exit_code"`
}

// NewRequest builds a request with the current protocol version applied.
func NewRequest(command, action string) Request {
	return Request{
		Version: Version,
		Command: command,
		Action:  action,
	}
}

// NewResponse creates a response mirrored to the current protocol version.
func NewResponse(success bool) Response {
	return Response{
		Version: Version,
		Success: success,
	}
}
