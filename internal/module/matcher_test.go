package module

import "testing"

func TestParseMatcherSupportsClassifierSyntax(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name      string
		input     string
		field     string
		expr      string
		wantedErr bool
	}{
		{name: "class colon", input: "class:^foo$", field: "class", expr: "^foo$"},
		{name: "title uppercase", input: "TITLE:bar", field: "title", expr: "bar"},
		{name: "initial title underscore", input: "initial_title:.*", field: "initialTitle", expr: ".*"},
		{name: "tags plural", input: "tags:prod", field: "tag", expr: "prod"},
		{name: "equals normalization", input: "initial-class=^vim$", field: "initialClass", expr: "^vim$"},
		{name: "unknown classifier passes through", input: "unknown:expr", field: "class", expr: "unknown:expr"},
		{name: "missing expression", input: "class:", wantedErr: true},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			matcher, err := ParseMatcher(tc.input)
			if tc.wantedErr {
				if err == nil {
					t.Fatalf("expected error for input %q", tc.input)
				}
				return
			}
			if err != nil {
				t.Fatalf("ParseMatcher(%q) returned error: %v", tc.input, err)
			}
			if matcher.Field != tc.field {
				t.Fatalf("expected field %q, got %q", tc.field, matcher.Field)
			}
			if matcher.Expr != tc.expr {
				t.Fatalf("expected expr %q, got %q", tc.expr, matcher.Expr)
			}
		})
	}
}

func TestParseMatcherRetainsRawValue(t *testing.T) {
	input := "title:^Firefox$"
	matcher, err := ParseMatcher(input)
	if err != nil {
		t.Fatalf("ParseMatcher returned error: %v", err)
	}
	if matcher.Raw != input {
		t.Fatalf("expected raw %q, got %q", input, matcher.Raw)
	}
	if matcher.Field != "title" || matcher.Expr != "^Firefox$" {
		t.Fatalf("unexpected matcher contents: %+v", matcher)
	}
}
