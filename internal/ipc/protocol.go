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
	Version   uint16          `json:"version"`
	Success   bool            `json:"success"`
	Data      json.RawMessage `json:"data,omitempty"`
	Error     string          `json:"error,omitempty"`
	ExitCode  int             `json:"exit_code"`
	Streaming bool            `json:"streaming,omitempty"`
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

// AssignData marshals a value to JSON and assigns it to the response data field.
// If value is nil, the response is marked successful with nil data.
func AssignData(resp *Response, value any) error {
	if value == nil {
		resp.Success = true
		resp.Data = nil
		return nil
	}
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}
	resp.Data = data
	resp.Success = true
	return nil
}
