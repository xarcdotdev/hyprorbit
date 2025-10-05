package config

import "testing"

func TestParseMatcherSupportsClassifierSyntax(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name      string
		input     string
		expect    Matcher
		wantError bool
	}{
		{name: "title classifier", input: "title:^Firefox$", expect: Matcher{Field: "title", Expr: "^Firefox$", Raw: "title:^Firefox$"}},
		{name: "initial title with dash", input: "initial-title:foo", expect: Matcher{Field: "initialTitle", Expr: "foo", Raw: "initial-title:foo"}},
		{name: "underscore equals", input: "initial_class=bar", expect: Matcher{Field: "initialClass", Expr: "bar", Raw: "initial_class=bar"}},
		{name: "tags plural", input: "tags:prod", expect: Matcher{Field: "tag", Expr: "prod", Raw: "tags:prod"}},
		{name: "unknown classifier treated as expr", input: "something:else", expect: Matcher{Field: defaultMatcherField, Expr: "something:else", Raw: "something:else"}},
		{name: "missing expression", input: "class:", wantError: true},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			matcher, err := parseMatcher(tc.input)
			if tc.wantError {
				if err == nil {
					t.Fatalf("expected error for input %q", tc.input)
				}
				return
			}
			if err != nil {
				t.Fatalf("parseMatcher(%q) returned error: %v", tc.input, err)
			}
			if matcher.Field != tc.expect.Field {
				t.Fatalf("expected field %q, got %q", tc.expect.Field, matcher.Field)
			}
			if matcher.Expr != tc.expect.Expr {
				t.Fatalf("expected expr %q, got %q", tc.expect.Expr, matcher.Expr)
			}
			if matcher.Raw != tc.expect.Raw {
				t.Fatalf("expected raw %q, got %q", tc.expect.Raw, matcher.Raw)
			}
		})
	}
}
