package ctl

// Error represents a daemon-side failure annotated with an exit code.
type Error struct {
	Message string
	Code    int
}

func (e *Error) Error() string {
	if e == nil {
		return ""
	}
	return e.Message
}

// ExitCode returns the process exit code suggested by the daemon.
func (e *Error) ExitCode() int {
	if e == nil || e.Code == 0 {
		return 1
	}
	return e.Code
}
