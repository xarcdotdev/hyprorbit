package presenter

import (
	"context"
	"encoding/json"
	"strconv"
	"strings"

	"hyprorbit/internal/config"
	"hyprorbit/internal/daemon"
)

type ModuleWatchFormatterOptions struct {
	Waybar           bool
	ConfigPath       string
	WaybarConfigPath string
	Config           *config.EffectiveConfig
}

type ModuleWatchFormatter struct {
	mode         moduleWatchMode
	waybarConfig config.WaybarModuleWatchSettings
}

type moduleWatchMode int

const (
	moduleWatchModeGeneral moduleWatchMode = iota
	moduleWatchModeWaybar
)

func NewModuleWatchFormatter(ctx context.Context, opts ModuleWatchFormatterOptions) (*ModuleWatchFormatter, error) {
	mode := moduleWatchModeGeneral
	formatter := &ModuleWatchFormatter{mode: mode}
	if !opts.Waybar {
		return formatter, nil
	}

	var cfg *config.EffectiveConfig
	var err error
	if opts.Config != nil {
		cfg = opts.Config
	} else {
		loader := config.NewLoader(config.LoaderOptions{
			OverridePath:       opts.ConfigPath,
			WaybarOverridePath: opts.WaybarConfigPath,
		})
		cfg, err = loader.Load(ctx)
		if err != nil {
			return nil, err
		}
	}
	formatter.mode = moduleWatchModeWaybar
	formatter.waybarConfig = cfg.Waybar.ModuleWatch
	return formatter, nil
}

func (f *ModuleWatchFormatter) Format(snapshot daemon.StatusSnapshot) ([]byte, error) {
	switch f.mode {
	case moduleWatchModeWaybar:
		return f.formatWaybar(snapshot)
	default:
		return f.formatGeneral(snapshot)
	}
}

func (f *ModuleWatchFormatter) formatGeneral(snapshot daemon.StatusSnapshot) ([]byte, error) {
	text := snapshot.Module
	if strings.TrimSpace(text) == "" {
		text = snapshot.Workspace
	}

	payload := map[string]any{
		"text":      text,
		"workspace": snapshot.Workspace,
		"module":    snapshot.Module,
	}

	tooltip := snapshot.Workspace
	if tooltip == "" && snapshot.Orbit != nil && snapshot.Orbit.Name != "" {
		tooltip = snapshot.Orbit.Name
	}
	if snapshot.Orbit != nil && snapshot.Orbit.Label != "" {
		tooltip = snapshot.Orbit.Label
	}
	if tooltip != "" {
		payload["tooltip"] = tooltip
	}

	if snapshot.Workspace != "" {
		payload["alt"] = snapshot.Workspace
	}

	classes := make([]string, 0, 3)
	if snapshot.Module != "" {
		classes = append(classes, snapshot.Module)
	}
	if snapshot.Orbit != nil && snapshot.Orbit.Name != "" {
		classes = append(classes, snapshot.Orbit.Name)
		payload["orbit"] = snapshot.Orbit.Name
	}
	if len(classes) > 0 {
		payload["class"] = strings.Join(classes, " ")
	}

	if snapshot.Orbit != nil {
		payload["orbit_record"] = snapshot.Orbit
		if snapshot.Orbit.Label != "" {
			payload["orbit_label"] = snapshot.Orbit.Label
		}
		if snapshot.Orbit.Color != "" {
			payload["color"] = snapshot.Orbit.Color
		}
	}

	if snapshot.Windows > 0 {
		payload["windows"] = snapshot.Windows
	}
	if snapshot.Monitor != "" {
		payload["monitor"] = snapshot.Monitor
	}

	return json.Marshal(payload)
}

func (f *ModuleWatchFormatter) formatWaybar(snapshot daemon.StatusSnapshot) ([]byte, error) {
	cfg := f.waybarConfig

	payload := make(map[string]any)

	text := f.firstNonEmpty(cfg.Text, snapshot)
	if text == "" {
		text = snapshot.Module
		if text == "" {
			text = snapshot.Workspace
		}
	}
	payload["text"] = text

	if tooltip := f.firstNonEmpty(cfg.Tooltip, snapshot); tooltip != "" {
		payload["tooltip"] = tooltip
	}
	if alt := f.firstNonEmpty(cfg.Alt, snapshot); alt != "" {
		payload["alt"] = alt
	}

	classes := f.buildWaybarClasses(cfg.Class, snapshot)
	if len(classes) > 0 {
		payload["class"] = classes
	}

	if cfg.Percentage != nil {
		if pct, ok := f.computePercentage(*cfg.Percentage, snapshot); ok {
			payload["percentage"] = pct
		}
	}

	return json.Marshal(payload)
}

func (f *ModuleWatchFormatter) firstNonEmpty(sources []config.WaybarValueSource, snapshot daemon.StatusSnapshot) string {
	for _, src := range sources {
		value := strings.TrimSpace(f.valueFromSource(src, snapshot))
		if value != "" {
			return value
		}
	}
	return ""
}

func (f *ModuleWatchFormatter) valueFromSource(src config.WaybarValueSource, snapshot daemon.StatusSnapshot) string {
	switch src {
	case config.WaybarValueModule:
		return snapshot.Module
	case config.WaybarValueWorkspace:
		return snapshot.Workspace
	case config.WaybarValueOrbit:
		if snapshot.Orbit != nil {
			return snapshot.Orbit.Name
		}
	case config.WaybarValueOrbitLabel:
		if snapshot.Orbit != nil {
			return snapshot.Orbit.Label
		}
	case config.WaybarValueOrbitColor:
		if snapshot.Orbit != nil {
			return snapshot.Orbit.Color
		}
	case config.WaybarValueModuleOrbit:
		if snapshot.Module != "" && snapshot.Orbit != nil && snapshot.Orbit.Name != "" {
			return snapshot.Module + "-" + snapshot.Orbit.Name
		}
	case config.WaybarValueWindows:
		return strconv.Itoa(snapshot.Windows)
	}
	return ""
}

func (f *ModuleWatchFormatter) buildWaybarClasses(cfg config.WaybarClassSettings, snapshot daemon.StatusSnapshot) []string {
	classes := make([]string, 0, len(cfg.Sources))
	seen := make(map[string]struct{})
	add := func(values ...string) {
		for _, v := range values {
			v = strings.TrimSpace(v)
			if v == "" {
				continue
			}
			if _, ok := seen[v]; ok {
				continue
			}
			seen[v] = struct{}{}
			classes = append(classes, v)
		}
	}

	for _, src := range cfg.Sources {
		switch src {
		case config.WaybarClassModule:
			add(snapshot.Module)
		case config.WaybarClassOrbit:
			if snapshot.Orbit != nil {
				add(snapshot.Orbit.Name)
			}
		case config.WaybarClassModuleOrbit:
			if snapshot.Module != "" && snapshot.Orbit != nil && snapshot.Orbit.Name != "" {
				add(snapshot.Module + "-" + snapshot.Orbit.Name)
			}
		case config.WaybarClassWorkspace:
			add(snapshot.Workspace)
		case config.WaybarClassWindows:
			add("windows-" + strconv.Itoa(snapshot.Windows))
		}
	}

	for _, rule := range cfg.Rules {
		fieldValue := f.valueFromSource(rule.Field, snapshot)
		if fieldValue == "" {
			continue
		}
		match := false
		if rule.Regex != nil {
			match = rule.Regex.MatchString(fieldValue)
		} else if rule.Equals != "" {
			match = fieldValue == rule.Equals
		}
		if !match {
			continue
		}
		if rule.Append {
			add(rule.Value...)
			continue
		}
		classes = nil
		seen = make(map[string]struct{})
		add(rule.Value...)
		break
	}

	return classes
}

func (f *ModuleWatchFormatter) computePercentage(cfg config.WaybarPercentageSetting, snapshot daemon.StatusSnapshot) (int, bool) {
	if cfg.Max <= 0 {
		return 0, false
	}
	switch cfg.Source {
	case config.WaybarMetricWindows:
		if snapshot.Windows < 0 {
			return 0, false
		}
		value := snapshot.Windows * 100 / cfg.Max
		if value > 100 {
			value = 100
		}
		if value < 0 {
			value = 0
		}
		return value, true
	default:
		return 0, false
	}
}
