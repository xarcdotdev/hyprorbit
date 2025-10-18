package module

import (
	"fmt"
	"hyprorbit/internal/config"
	"hyprorbit/internal/orbit"
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
	Global          bool
}

// SeedStep expresses a single seed instruction for module bootstrapping.
type SeedStep struct {
	Matcher config.Matcher
	Cmd     []string
}

// Status describes the current module/orbit workspace association.
type Status struct {
	Module    string       `json:"module"`
	Workspace string       `json:"workspace"`
	Orbit     orbit.Record `json:"orbit"`
}

// ParseWorkspaceName splits a workspace identifier into module and orbit components.
func ParseWorkspaceName(workspace string) (moduleName, orbitName string, err error) {
	workspace = strings.TrimSpace(workspace)
	if workspace == "" {
		return "", "", fmt.Errorf("module: workspace name cannot be empty")
	}
	idx := strings.Index(workspace, "-")
	if idx <= 0 || idx == len(workspace)-1 {
		return "", "", fmt.Errorf("module: workspace %q does not follow <orbit>-<module>", workspace)
	}
	orbitName = workspace[:idx]
	moduleName = workspace[idx+1:]
	if moduleName == "" || orbitName == "" {
		return "", "", fmt.Errorf("module: workspace %q does not follow <orbit>-<module>", workspace)
	}
	return moduleName, orbitName, nil
}
