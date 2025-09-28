package service

import (
	"context"
	"fmt"

	"hypr-orbits/internal/ipc"
)

// Dispatcher routes IPC requests to domain handlers.
type Dispatcher struct {
	state *DaemonState
}

// NewDispatcher constructs a dispatcher bound to the daemon state.
func NewDispatcher(state *DaemonState) *Dispatcher {
	return &Dispatcher{state: state}
}

// Handle executes the request, returning a response suitable for IPC clients.
func (d *Dispatcher) Handle(ctx context.Context, req ipc.Request) (ipc.Response, error) {
	resp := ipc.NewResponse(false)

	if req.Version != ipc.Version {
		resp.Error = fmt.Sprintf("unsupported protocol version %d", req.Version)
		resp.ExitCode = 1
		return resp, nil
	}

	resp.Error = "dispatcher: not implemented"
	resp.ExitCode = 1
	return resp, nil
}
