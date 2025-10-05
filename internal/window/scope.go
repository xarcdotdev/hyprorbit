package window

import "strings"

// Scope describes the search domain when selecting windows.
type Scope int

const (
	ScopeWorkspace Scope = iota
	ScopeOrbit
	ScopeGlobal
)

// ParseScope normalises scope modifiers such as "orbit" or "all".
func ParseScope(input string) (Scope, bool) {
	trimmed := strings.TrimSpace(input)
	if trimmed == "" {
		return ScopeWorkspace, false
	}

	switch strings.ToLower(trimmed) {
	case "workspace", "ws", "current":
		return ScopeWorkspace, true
	case "orbit":
		return ScopeOrbit, true
	case "global", "all", "any":
		return ScopeGlobal, true
	default:
		return ScopeWorkspace, false
	}
}
