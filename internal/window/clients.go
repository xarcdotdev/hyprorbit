package window

import (
	"regexp"
	"strings"

	"hyprorbit/internal/hyprctl"
	"hyprorbit/internal/regex"
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
func FilterByScope(clients []hyprctl.ClientInfo, scope Scope, workspace string, orbit string) []hyprctl.ClientInfo {
	workspace = strings.TrimSpace(workspace)
	orbit = strings.TrimSpace(orbit)
	suffix := ""
	if orbit != "" {
		suffix = "-" + orbit
	}

	result := make([]hyprctl.ClientInfo, 0, len(clients))
	for _, client := range clients {
		sanitized := SanitizeClient(client)
		switch scope {
		case ScopeWorkspace:
			if workspace == "" || sanitized.Workspace.Name != workspace {
				continue
			}
		case ScopeOrbit:
			name := sanitized.Workspace.Name
			if orbit == "" {
				continue
			}
			if name == "" || !strings.HasSuffix(name, suffix) {
				continue
			}
		case ScopeGlobal:
			// no workspace filtering
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
