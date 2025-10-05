package regex

import "strings"

// Field identifies a Hyprland client attribute that can be targeted by regex selectors.
type Field int

const (
	FieldAny Field = iota
	FieldAddress
	FieldClass
	FieldTitle
	FieldInitialClass
	FieldInitialTitle
	FieldTag
	FieldWorkspace
)

// CanonicalName returns the normalized string identifier for the field.
func (f Field) CanonicalName() string {
	switch f {
	case FieldAddress:
		return "address"
	case FieldClass:
		return "class"
	case FieldTitle:
		return "title"
	case FieldInitialClass:
		return "initialClass"
	case FieldInitialTitle:
		return "initialTitle"
	case FieldTag:
		return "tag"
	case FieldWorkspace:
		return "workspace"
	default:
		return ""
	}
}

// FieldFromName resolves a classifier into its Field enumeration.
func FieldFromName(name string) (Field, bool) {
	trimmed := strings.TrimSpace(name)
	if trimmed == "" {
		return FieldAny, false
	}

	normalized := strings.ToLower(trimmed)
	normalized = strings.ReplaceAll(normalized, "-", "")
	normalized = strings.ReplaceAll(normalized, "_", "")

	switch normalized {
	case "addr", "address", "window", "win":
		return FieldAddress, true
	case "class":
		return FieldClass, true
	case "title":
		return FieldTitle, true
	case "initialclass":
		return FieldInitialClass, true
	case "initialtitle":
		return FieldInitialTitle, true
	case "tag", "tags":
		return FieldTag, true
	case "workspace", "ws":
		return FieldWorkspace, true
	default:
		return FieldAny, false
	}
}
