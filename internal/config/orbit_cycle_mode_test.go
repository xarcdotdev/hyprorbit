package config

import "testing"

func TestParseOrbitCycleMode(t *testing.T) {
	tests := map[string]struct {
		input   string
		want    OrbitCycleMode
		wantErr bool
	}{
		"default":     {input: "", want: OrbitCycleModeAll},
		"all":         {input: "all", want: OrbitCycleModeAll},
		"All spacing": {input: "  ALL  ", want: OrbitCycleModeAll},
		"not-empty":   {input: "not-empty", want: OrbitCycleModeNotEmpty},
		"invalid":     {input: "windows", wantErr: true},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			got, err := ParseOrbitCycleMode(tc.input)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error for input %q", tc.input)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tc.want {
				t.Fatalf("unexpected result for %q: got %q want %q", tc.input, got, tc.want)
			}
		})
	}
}
