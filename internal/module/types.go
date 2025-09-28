package module

import (
	"hypr-orbits/internal/config"
	"hypr-orbits/internal/orbit"
	"fmt"
	"strings"
)

// Result captures the outcome of a module operation exposed to callers.
type Result struct {
	Action    string
	Workspace string
	Orbit     string
}

// FocusOptions fine-tunes how focus operations behave for a module.
type FocusOptions struct {
	MatcherOverride string
	CmdOverride     []string
	ForceFloat      bool
	NoMove          bool
}

// SeedStep expresses a single seed instruction for module bootstrapping.
type SeedStep struct {
	Matcher config.Matcher
	Cmd     []string
}
