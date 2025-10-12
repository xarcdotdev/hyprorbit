package config

// DefaultConfig returns the built-in configuration used when no file is present.
func DefaultConfig() *Config {
	move := true
	float := false

	return &Config{
		Orbits: []Orbit{
			{Name: "alpha", Label: "α", Color: "#BC83F9"},
			{Name: "beta", Label: "β", Color: "#83C6F9"},
			{Name: "gamma", Label: "γ", Color: "#83F9BC"},
		},
		Modules: map[string]Module{
			"code": {
				Hotkey: "SUPER+1",
				Focus: ModuleFocus{
					Match: "class=.*Code",
					Cmd:   []string{"kitty", "-T", "Code"},
				},
			},
			"gfx": {
				Hotkey: "SUPER+2",
				Focus: ModuleFocus{
					Match: "class=.*Blender",
					Cmd:   []string{"blender"},
				},
			},
			"comm": {
				Hotkey: "SUPER+3",
				Focus: ModuleFocus{
					Match: "title=.*Slack",
					Cmd:   []string{"flatpak", "run", "com.slack.Slack"},
				},
			},
		},
		Defaults: ModuleDefaults{
			Float: &float,
			Move:  &move,
		},
		Orbit: OrbitConfig{
			SwitchPreference: string(OrbitSwitchPreferenceLastActiveFirst),
			CycleMode:        string(OrbitCycleModeAll),
		},
	}
}
