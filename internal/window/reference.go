package window

import (
	"strings"

	"hyprorbit/internal/regex"
)

// Reference captures a scoped regex selector for matching windows.
type Reference struct {
	Scope    Scope
	Selector regex.Selector
}

// ParseReference resolves scoped regex window references such as
// "orbit:class:firefox" or "global:regex:vim". The returned boolean indicates
// whether the input was recognised as a regex-style selector.
func ParseReference(input string) (Reference, bool, error) {
	trimmed := strings.TrimSpace(input)
	if trimmed == "" {
		return Reference{}, false, regex.ErrEmptySelector
	}

	scope := ScopeWorkspace
	working := trimmed
	if idx := strings.IndexRune(working, ':'); idx > 0 {
		if sc, ok := ParseScope(working[:idx]); ok {
			scope = sc
			working = strings.TrimSpace(working[idx+1:])
		}
	}

	selector, isRegex, err := regex.ParseWindowSelector(working)
	if !isRegex {
		return Reference{}, false, err
	}
	if err != nil {
		return Reference{}, true, err
	}

	return Reference{Scope: scope, Selector: selector}, true, nil
}
