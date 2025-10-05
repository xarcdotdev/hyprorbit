package config

import "testing"

func TestBuildEffectiveFocusLegacyRule(t *testing.T) {
	t.Parallel()

	cfg := &Config{
		Orbits: []Orbit{{Name: "alpha"}},
		Modules: map[string]Module{
			"app": {
				Focus: ModuleFocus{Match: "class:^App$", Cmd: []string{"app"}},
			},
		},
	}

	eff, err := BuildEffective("<legacy>", cfg)
	if err != nil {
		t.Fatalf("BuildEffective returned error: %v", err)
	}

	mod, ok := eff.Modules["app"]
	if !ok {
		t.Fatalf("expected module app to be present")
	}
	if len(mod.Focus.Rules) != 1 {
		t.Fatalf("expected one focus rule, got %d", len(mod.Focus.Rules))
	}
	rule := mod.Focus.Rules[0]
	if rule.Matcher.Expr != "^App$" {
		t.Fatalf("expected matcher expr ^App$, got %q", rule.Matcher.Expr)
	}
	if rule.Matcher.Field != "class" {
		t.Fatalf("expected matcher field class, got %q", rule.Matcher.Field)
	}
	if mod.Focus.Logic != ModuleFocusLogicFirstMatchWins {
		t.Fatalf("expected default logic first-match-wins, got %q", mod.Focus.Logic)
	}
	if len(rule.Cmd) != 1 || rule.Cmd[0] != "app" {
		t.Fatalf("expected command [app], got %v", rule.Cmd)
	}
}

func TestBuildEffectiveFocusRulesWithLogic(t *testing.T) {
	t.Parallel()

	cfg := &Config{
		Orbits: []Orbit{{Name: "alpha"}},
		Modules: map[string]Module{
			"suite": {
				Focus: ModuleFocus{
					Logic: string(ModuleFocusLogicTryAll),
					Rules: []ModuleFocusRule{
						{Match: "class:^One$", Cmd: []string{"one"}},
						{Match: "class:^Two$"},
					},
				},
			},
		},
	}

	eff, err := BuildEffective("<rules>", cfg)
	if err != nil {
		t.Fatalf("BuildEffective returned error: %v", err)
	}

	mod := eff.Modules["suite"]
	if mod.Focus.Logic != ModuleFocusLogicTryAll {
		t.Fatalf("expected logic try-all, got %q", mod.Focus.Logic)
	}
	if len(mod.Focus.Rules) != 2 {
		t.Fatalf("expected two focus rules, got %d", len(mod.Focus.Rules))
	}
	if mod.Focus.Rules[0].Matcher.Expr != "^One$" {
		t.Fatalf("expected first matcher expr ^One$, got %q", mod.Focus.Rules[0].Matcher.Expr)
	}
	if len(mod.Focus.Rules[1].Cmd) != 0 {
		t.Fatalf("expected second rule with no command, got %v", mod.Focus.Rules[1].Cmd)
	}
}

func TestBuildEffectiveFocusInvalidLogic(t *testing.T) {
	t.Parallel()

	cfg := &Config{
		Orbits: []Orbit{{Name: "alpha"}},
		Modules: map[string]Module{
			"bad": {
				Focus: ModuleFocus{
					Logic: "invalid-mode",
				},
			},
		},
	}

	_, err := BuildEffective("<invalid>", cfg)
	if err == nil {
		t.Fatalf("expected error for invalid focus logic")
	}
}
