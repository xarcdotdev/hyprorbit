package regex

import (
	"errors"
	"strings"
)

// Selector captures the outcome of parsing a field-qualified pattern.
type Selector struct {
	Field     Field
	Pattern   string
	Qualified bool
}

var (
	ErrEmptySelector  = errors.New("regex: selector cannot be empty")
	ErrEmptyQualifier = errors.New("regex: selector field cannot be empty")
	ErrEmptyPattern   = errors.New("regex: selector pattern cannot be empty")
)

// ParseMatcher normalises matcher input using the provided default field.
// It accepts both `field:pattern` and legacy `field=pattern` forms, defaulting to
// the provided field when no qualifier is supplied.
func ParseMatcher(input string, defaultField Field) (Selector, error) {
	trimmed := strings.TrimSpace(input)
	if trimmed == "" {
		return Selector{}, ErrEmptySelector
	}

	qualifier, pattern, hasDelimiter, err := split(trimmed, true)
	if err != nil {
		return Selector{}, err
	}

	field := defaultField
	qualified := false
	if hasDelimiter {
		if f, ok := FieldFromName(qualifier); ok {
			field = f
			qualified = true
		} else {
			// Unknown classifier: treat entire input as the pattern using the default field.
			pattern = trimmed
		}
	}

	return Selector{Field: field, Pattern: pattern, Qualified: qualified}, nil
}

// ParseWindowSelector interprets window selection references used by the dispatcher.
// The boolean indicates whether the input was recognised as a regex selector.
func ParseWindowSelector(input string) (Selector, bool, error) {
	trimmed := strings.TrimSpace(input)
	if trimmed == "" {
		return Selector{}, false, ErrEmptySelector
	}

	working := trimmed
	usedPrefix := false
	if len(working) >= 6 && strings.HasPrefix(strings.ToLower(working), "regex:") {
		working = strings.TrimSpace(working[6:])
		if working == "" {
			return Selector{}, true, ErrEmptyPattern
		}
		usedPrefix = true
	}

	qualifier, pattern, hasDelimiter, err := split(working, true)
	if err != nil {
		return Selector{}, true, err
	}

	if hasDelimiter {
		if f, ok := FieldFromName(qualifier); ok {
			return Selector{Field: f, Pattern: pattern, Qualified: true}, true, nil
		}
		return Selector{Field: FieldAny, Pattern: pattern, Qualified: false}, true, nil
	}

	if usedPrefix {
		return Selector{Field: FieldAny, Pattern: pattern, Qualified: false}, true, nil
	}

	return Selector{}, false, nil
}

func split(input string, allowEquals bool) (qualifier, pattern string, hasDelimiter bool, err error) {
	trimmed := strings.TrimSpace(input)
	if trimmed == "" {
		return "", "", false, ErrEmptySelector
	}

	delimIdx := strings.IndexRune(trimmed, ':')
	if allowEquals {
		if eqIdx := strings.IndexRune(trimmed, '='); eqIdx >= 0 && (delimIdx < 0 || eqIdx < delimIdx) {
			delimIdx = eqIdx
		}
	}
	if delimIdx <= 0 {
		return "", trimmed, false, nil
	}

	qualifier = strings.TrimSpace(trimmed[:delimIdx])
	pattern = strings.TrimSpace(trimmed[delimIdx+1:])
	if qualifier == "" {
		return "", "", true, ErrEmptyQualifier
	}
	if pattern == "" {
		return "", "", true, ErrEmptyPattern
	}
	return qualifier, pattern, true, nil
}
