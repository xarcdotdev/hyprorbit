package hyprctl

import (
	"context"
	"fmt"
	"strings"
)

type configParser bool

const (
	legacyParser configParser = false
	luaParser    configParser = true
)

// Probe each mutation: the config manager can change while this client lives.
// Never infer compatibility from a failed mutation or retry a mutation.
func (c *Client) detectParser(ctx context.Context) (configParser, error) {
	out, err := c.runCombined(ctx, "eval", "return nil")
	if err != nil {
		return legacyParser, fmt.Errorf("detect config parser: %w", err)
	}
	switch strings.TrimSpace(string(out)) {
	case "ok":
		return luaParser, nil
	case "eval is only supported with the lua config manager", "unknown request":
		return legacyParser, nil
	default:
		return legacyParser, fmt.Errorf("detect config parser: unexpected response %q", out)
	}
}

// luaString uses fixed-width decimal escapes (not Go's incompatible escapes).
// Escape batch metacharacters even inside quotes: Hyprland's batch splitter
// does not understand Lua strings.
func luaString(s string) string {
	var b strings.Builder
	b.WriteByte('"')
	for i := 0; i < len(s); i++ {
		ch := s[i]
		switch {
		case ch == '"' || ch == '\\':
			b.WriteByte('\\')
			b.WriteByte(ch)
		case ch < 32 || ch == 127 || ch == ';' || ch == '[' || ch == ']':
			fmt.Fprintf(&b, "\\%03d", ch)
		default:
			b.WriteByte(ch)
		}
	}
	b.WriteByte('"')
	return b.String()
}

func serializeDispatch(parser configParser, args []string) ([]string, error) {
	if len(args) == 0 || strings.TrimSpace(args[0]) == "" {
		return nil, fmt.Errorf("dispatch: dispatcher missing")
	}
	if strings.ContainsAny(args[0], " \t\r\n\x00") {
		return nil, fmt.Errorf("dispatch: invalid dispatcher %q", args[0])
	}
	if parser == legacyParser {
		for _, arg := range args {
			if strings.ContainsRune(arg, 0) {
				return nil, fmt.Errorf("dispatch: NUL in argument")
			}
		}
		return append([]string{"dispatch"}, args...), nil
	}
	name := args[0]
	switch name {
	case "focusmonitor", "workspace", "focuswindow", "movetoworkspace", "movetoworkspacesilent", "togglefloating":
	default:
		return nil, fmt.Errorf("dispatch: unsupported Lua dispatcher %q", name)
	}
	if len(args) != 2 || strings.TrimSpace(args[1]) == "" {
		return nil, fmt.Errorf("dispatch %s: expected one nonempty selector", name)
	}
	selector := luaString(args[1])
	var expr string
	switch name {
	case "focusmonitor":
		expr = "hl.dsp.focus({monitor=" + selector + "})"
	case "workspace":
		expr = "hl.dsp.focus({workspace=" + selector + "})"
	case "focuswindow":
		expr = "hl.dsp.focus({window=" + selector + "})"
	case "movetoworkspace", "movetoworkspacesilent":
		// The legacy optional ',window' syntax is not a workspace selector.
		if strings.Contains(args[1], ",") {
			return nil, fmt.Errorf("dispatch %s: compound move selectors unsupported", name)
		}
		expr = fmt.Sprintf("hl.dsp.window.move({workspace=%s,follow=%t})", selector, name == "movetoworkspace")
	case "togglefloating":
		expr = "hl.dsp.window.float({action=\"toggle\",window=" + selector + "})"
	}
	return []string{"dispatch", expr}, nil
}

func dispatchResult(out []byte) error {
	if strings.TrimSpace(string(out)) != "ok" {
		return fmt.Errorf("dispatch: unexpected response %q", out)
	}
	return nil
}

func batchQuery(name string) bool {
	switch name {
	case "clients", "monitors", "workspaces", "activeworkspace", "activewindow", "version", "devices", "layers", "getoption", "binds", "animations", "cursorpos", "configerrors", "rollinglog", "layouts", "decorations", "descriptions", "systeminfo", "instances", "globalshortcuts", "submap":
		return true
	}
	return false
}

func safeBatchCommand(cmd []string) error {
	if cmd[0] == "" || strings.ContainsAny(cmd[0], " \t\r\n") {
		return fmt.Errorf("batch: invalid command %q", cmd[0])
	}
	if flags, name, flagged := strings.Cut(cmd[0], "/"); flagged {
		// Preserve Hyprland's raw JSON query syntax without permitting flagged
		// dispatch/keyword requests to bypass compatibility serialization.
		if flags != "j" || !batchQuery(name) {
			return fmt.Errorf("batch: unsupported flagged command %q", cmd[0])
		}
	}
	for _, arg := range cmd {
		if strings.ContainsAny(arg, ";[]\r\n\x00") {
			return fmt.Errorf("batch: unsafe delimiter in %q", arg)
		}
	}
	return nil
}
