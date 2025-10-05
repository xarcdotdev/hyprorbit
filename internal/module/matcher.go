package module

import (
	"fmt"
	"regexp"
	"strings"

	"hyprorbit/internal/config"
)

// ParseMatcher converts a CLI-style matcher override into the structured form.
func ParseMatcher(input string) (config.Matcher, error) {
	input = strings.TrimSpace(input)
	if input == "" {
		return config.Matcher{}, fmt.Errorf("matcher override cannot be empty")
	}

	matcher, err := config.ParseMatcherString(input)
	if err != nil {
		return config.Matcher{}, err
	}
	if matcher.Raw == "" {
		matcher.Raw = input
	}
	return matcher, nil
}

func matcherToString(m config.Matcher) string {
	if m.Raw != "" {
		return m.Raw
	}
	if m.Expr == "" {
		return ""
	}
	return fmt.Sprintf("%s:%s", m.Field, m.Expr)
}

func compileMatcher(m config.Matcher) (*regexp.Regexp, error) {
	if m.Expr == "" {
		return nil, nil
	}
	return regexp.Compile(m.Expr)
}

func matches(re *regexp.Regexp, expr string, value string) bool {
	if expr == "" {
		return true
	}
	if re == nil {
		return false
	}
	return re.MatchString(value)
}
