package main

import "testing"

func TestParseWindowMoveTarget(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		wantOrbit   string
		wantModule  string
		expectError bool
	}{
		{name: "module only", input: "module:code", wantModule: "module:code"},
		{name: "orbit qualified", input: "orbit:beta/module:code", wantOrbit: "orbit:beta", wantModule: "module:code"},
		{name: "shorthand", input: "beta/code", wantOrbit: "orbit:beta", wantModule: "module:code"},
		{name: "relative selectors", input: "orbit:next/module:regex:dev", wantOrbit: "orbit:next", wantModule: "module:regex:dev"},
		{name: "missing module", input: "orbit:beta/", expectError: true},
	}

	for _, tt := range tests {
		tc := tt
		t.Run(tc.name, func(t *testing.T) {
			orbit, module, err := parseWindowMoveTarget(tc.input)
			if tc.expectError {
				if err == nil {
					t.Fatalf("expected error for %q, got none", tc.input)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error for %q: %v", tc.input, err)
			}
			if orbit != tc.wantOrbit {
				t.Fatalf("unexpected orbit: got %q want %q", orbit, tc.wantOrbit)
			}
			if module != tc.wantModule {
				t.Fatalf("unexpected module: got %q want %q", module, tc.wantModule)
			}
		})
	}
}
