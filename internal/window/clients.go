package window

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"

	"hyprorbit/internal/hyprctl"
	"hyprorbit/internal/orbit"
	"hyprorbit/internal/regex"
	"hyprorbit/internal/runtime"
	"hyprorbit/internal/workspace"
)

// SanitizeClient trims client metadata for stable comparisons.
func SanitizeClient(client hyprctl.ClientInfo) hyprctl.ClientInfo {
	client.Address = strings.TrimSpace(client.Address)
	client.Class = strings.TrimSpace(client.Class)
	client.Title = strings.TrimSpace(client.Title)
	client.InitialClass = strings.TrimSpace(client.InitialClass)
	client.InitialTitle = strings.TrimSpace(client.InitialTitle)
	client.Workspace.Name = strings.TrimSpace(client.Workspace.Name)
	client.Tags = hyprctl.HyprTags(sanitizeTags([]string(client.Tags)))
	return client
}

// FilterByScope limits clients to the requested search domain.
// For ScopeGlobal, special workspaces are excluded by default.
func FilterByScope(clients []hyprctl.ClientInfo, scope Scope, workspace string, orbit string) []hyprctl.ClientInfo {
	workspace = strings.TrimSpace(workspace)
	orbit = strings.TrimSpace(orbit)
	prefix := ""
	if orbit != "" {
		prefix = orbit + "-"
	}

	result := make([]hyprctl.ClientInfo, 0, len(clients))
	for _, client := range clients {
		sanitized := SanitizeClient(client)
		wsName := sanitized.Workspace.Name
		switch scope {
		case ScopeWorkspace:
			if workspace == "" || wsName != workspace {
				continue
			}
		case ScopeOrbit:
			if orbit == "" {
				continue
			}
			if wsName == "" || !strings.HasPrefix(wsName, prefix) {
				continue
			}
		case ScopeGlobal:
			// Filter out special workspaces for global scope
			if wsName == "" || strings.HasPrefix(wsName, "special") {
				continue
			}
		}
		result = append(result, sanitized)
	}
	return result
}

// MatchClient evaluates the selector against a client record.
func MatchClient(re *regexp.Regexp, selector regex.Selector, client hyprctl.ClientInfo) bool {
	if re == nil {
		return true
	}
	switch selector.Field {
	case regex.FieldAddress:
		return matchField(re, client.Address)
	case regex.FieldClass:
		return matchField(re, client.Class)
	case regex.FieldTitle:
		return matchField(re, client.Title)
	case regex.FieldInitialClass:
		return matchField(re, client.InitialClass)
	case regex.FieldInitialTitle:
		return matchField(re, client.InitialTitle)
	case regex.FieldTag:
		return matchTags(re, []string(client.Tags))
	case regex.FieldWorkspace:
		return matchField(re, client.Workspace.Name)
	default:
		return matchField(re, client.Title) || matchField(re, client.Class) ||
			matchField(re, client.InitialTitle) || matchField(re, client.InitialClass)
	}
}

func matchField(re *regexp.Regexp, value string) bool {
	if value == "" {
		return false
	}
	return re.MatchString(value)
}

func matchTags(re *regexp.Regexp, tags []string) bool {
	if len(tags) == 0 {
		return false
	}
	for _, tag := range tags {
		if tag != "" && re.MatchString(tag) {
			return true
		}
	}
	return false
}

func sanitizeTags(tags []string) []string {
	if len(tags) == 0 {
		return nil
	}
	clean := make([]string, 0, len(tags))
	for _, tag := range tags {
		trimmed := strings.TrimSpace(tag)
		if trimmed == "" {
			continue
		}
		clean = append(clean, trimmed)
	}
	if len(clean) == 0 {
		return nil
	}
	return clean
}

// DescribeClient returns a human-readable description of a client.
// Priority: "title (class)", title, class, address, or "window" as fallback.
func DescribeClient(client hyprctl.ClientInfo) string {
	title := strings.TrimSpace(client.Title)
	class := strings.TrimSpace(client.Class)
	if title != "" && class != "" {
		return title + " (" + class + ")"
	}
	if title != "" {
		return title
	}
	if class != "" {
		return class
	}
	if addr := strings.TrimSpace(client.Address); addr != "" {
		return addr
	}
	return "window"
}

// ClientInfoFromWindow converts a hyprctl.Window to hyprctl.ClientInfo.
func ClientInfoFromWindow(win *hyprctl.Window) hyprctl.ClientInfo {
	if win == nil {
		return hyprctl.ClientInfo{}
	}
	info := hyprctl.ClientInfo{
		Address:      win.Address,
		Class:        win.Class,
		Title:        win.Title,
		InitialClass: win.InitialClass,
		InitialTitle: win.InitialTitle,
		Floating:     bool(win.Floating),
		Tags:         win.Tags,
		Workspace: hyprctl.WorkspaceHandle{
			Name: win.Workspace.Name,
		},
	}
	return SanitizeClient(info)
}

// DecodeClients retrieves all client info from hyprctl.
func DecodeClients(ctx context.Context, hypr runtime.HyprctlClient) ([]hyprctl.ClientInfo, error) {
	var clients []hyprctl.ClientInfo
	if err := hypr.DecodeClients(ctx, &clients); err != nil {
		return nil, err
	}
	return clients, nil
}

// ResolveSelection resolves window references and returns matching clients.
// Supports: "current", "workspace", "all", and scoped regex references like "orbit:class:firefox".
// When global is true, regex matches are performed across all orbits instead of the current scope.
func ResolveSelection(ctx context.Context, hypr runtime.HyprctlClient, orbitProvider orbit.Provider, ref string, global bool) ([]hyprctl.ClientInfo, error) {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return nil, fmt.Errorf("window reference cannot be empty")
	}
	lower := strings.ToLower(ref)
	switch {
	case lower == "current":
		win, err := hypr.ActiveWindow(ctx)
		if err != nil {
			return nil, err
		}
		if win == nil {
			return nil, nil
		}
		client := ClientInfoFromWindow(win)
		if client.Address == "" {
			return nil, nil
		}
		return []hyprctl.ClientInfo{client}, nil
	case lower == "workspace":
		workspaceName, err := workspace.ActiveName(ctx, hypr)
		if err != nil {
			return nil, err
		}
		clients, err := DecodeClients(ctx, hypr)
		if err != nil {
			return nil, err
		}
		return FilterByScope(clients, ScopeWorkspace, workspaceName, ""), nil
	case lower == "all":
		clients, err := DecodeClients(ctx, hypr)
		if err != nil {
			return nil, err
		}
		return FilterByScope(clients, ScopeGlobal, "", ""), nil
	default:
		reference, isRegex, err := ParseReference(ref)
		if err != nil {
			switch {
			case errors.Is(err, regex.ErrEmptyPattern):
				return nil, fmt.Errorf("window regex reference requires a pattern")
			case errors.Is(err, regex.ErrEmptyQualifier):
				return nil, fmt.Errorf("window regex reference requires a field before the pattern")
			case errors.Is(err, regex.ErrEmptySelector):
				return nil, fmt.Errorf("window regex reference requires a pattern")
			default:
				return nil, err
			}
		}
		if !isRegex {
			return nil, fmt.Errorf("window reference %q not supported", ref)
		}
		selector := reference.Selector
		if selector.Pattern == "" {
			return nil, fmt.Errorf("window regex reference requires a pattern")
		}
		re, err := regexp.Compile(selector.Pattern)
		if err != nil {
			return nil, fmt.Errorf("window regex %q invalid: %w", selector.Pattern, err)
		}

		clients, err := DecodeClients(ctx, hypr)
		if err != nil {
			return nil, err
		}

		// When global is true, use ScopeGlobal to match across all workspaces
		var scoped []hyprctl.ClientInfo
		if global {
			scoped = FilterByScope(clients, ScopeGlobal, "", "")
		} else {
			var workspaceName string
			if reference.Scope == ScopeWorkspace || reference.Scope == ScopeOrbit {
				workspaceName, err = workspace.ActiveName(ctx, hypr)
				if err != nil {
					return nil, err
				}
			}

			var orbitName string
			if reference.Scope == ScopeOrbit {
				orbitName, err = orbit.ActiveName(ctx, orbitProvider)
				if err != nil {
					return nil, err
				}
			}

			scoped = FilterByScope(clients, reference.Scope, workspaceName, orbitName)
		}

		if len(scoped) == 0 {
			return scoped, nil
		}

		matched := make([]hyprctl.ClientInfo, 0, len(scoped))
		for _, client := range scoped {
			if MatchClient(re, selector, client) {
				matched = append(matched, client)
			}
		}
		return matched, nil
	}
}
