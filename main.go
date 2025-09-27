package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Orbits []string               `yaml:"orbits"`
	Roles  map[string][]RoleEntry `yaml:"roles"`
}
type RoleEntry struct {
	Match string `yaml:"match"` // e.g. class:^Code$
	Cmd   string `yaml:"cmd"`
}

type Client struct {
	Address string `json:"address"`
	Class   string `json:"class"`
	Title   string `json:"title"`
	// Wayland natives often have appid in initialClass
	InitialClass string `json:"initialClass"`
	Workspace    struct {
		Name string `json:"name"`
	} `json:"workspace"`
}

func configPath() string {
	dir := os.Getenv("XDG_CONFIG_HOME")
	if dir == "" {
		dir = filepath.Join(os.Getenv("HOME"), ".config")
	}
	return filepath.Join(dir, "hypr-orbits", "config.yaml")
}
func statePath() string {
	dir := os.Getenv("XDG_STATE_HOME")
	if dir == "" {
		dir = filepath.Join(os.Getenv("HOME"), ".local", "state")
	}
	return filepath.Join(dir, "hypr-orbits", "orbit")
}
func ensureDir(p string) error { return os.MkdirAll(filepath.Dir(p), 0o755) }

func loadConfig() (*Config, error) {
	path := configPath()
	b, err := os.ReadFile(path)
	if err != nil {
		// default config
		return &Config{
			Orbits: []string{"α", "β", "γ"},
			Roles:  map[string][]RoleEntry{},
		}, nil
	}
	var c Config
	if err := yaml.Unmarshal(b, &c); err != nil {
		return nil, err
	}
	if len(c.Orbits) == 0 {
		c.Orbits = []string{"α", "β", "γ"}
	}
	return &c, nil
}
func getOrbit(c *Config) (string, error) {
	p := statePath()
	b, err := os.ReadFile(p)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			if err := ensureDir(p); err != nil {
				return "", err
			}
			if err := os.WriteFile(p, []byte(c.Orbits[0]), 0o644); err != nil {
				return "", err
			}
			return c.Orbits[0], nil
		}
		return "", err
	}
	return strings.TrimSpace(string(b)), nil
}
func setOrbit(label string) error {
	p := statePath()
	if err := ensureDir(p); err != nil {
		return err
	}
	return os.WriteFile(p, []byte(label), 0o644)
}
func orbitIndex(c *Config, cur string) int {
	for i, o := range c.Orbits {
		if o == cur {
			return i
		}
	}
	return 0
}

// hypr helpers
func hyprctl(args ...string) (string, error) {
	cmd := exec.Command("hyprctl", args...)
	var out, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("hyprctl %v: %v (%s)", args, err, strings.TrimSpace(errb.String()))
	}
	return out.String(), nil
}
func dispatch(cmd, arg string) error {
	_, err := hyprctl("dispatch", cmd, arg)
	return err
}
func clients() ([]Client, error) {
	out, err := hyprctl("clients", "-j")
	if err != nil {
		return nil, err
	}
	var cs []Client
	if err := json.Unmarshal([]byte(out), &cs); err != nil {
		return nil, err
	}
	return cs, nil
}

// matching
type matcher struct {
	key string // class|title|appid
	re  *regexp.Regexp
}

func parseMatcher(s string) (*matcher, error) {
	parts := strings.SplitN(s, ":", 2)
	if len(parts) != 2 {
		return nil, fmt.Errorf("invalid matcher: %s", s)
	}
	key := strings.ToLower(strings.TrimSpace(parts[0]))
	pat := strings.TrimSpace(parts[1])
	re, err := regexp.Compile(pat)
	if err != nil {
		return nil, err
	}
	return &matcher{key: key, re: re}, nil
}
func (m *matcher) match(c Client) bool {
	switch m.key {
	case "class":
		return m.re.MatchString(c.Class)
	case "title":
		return m.re.MatchString(c.Title)
	case "appid":
		return m.re.MatchString(c.InitialClass)
	default:
		return false
	}
}

func wsName(role, orbit string) string { return role + "-" + orbit }

func cmdOrbit(args []string, cfg *Config) error {
	if len(args) == 0 || args[0] == "get" {
		o, err := getOrbit(cfg)
		if err != nil {
			return err
		}
		fmt.Println(o)
		return nil
	}
	switch args[0] {
	case "set":
		if len(args) < 2 {
			return fmt.Errorf("usage: orbit set <label>")
		}
		return setOrbit(args[1])
	case "next":
		cur, _ := getOrbit(cfg)
		i := orbitIndex(cfg, cur)
		i = (i + 1) % len(cfg.Orbits)
		return setOrbit(cfg.Orbits[i])
	case "prev":
		cur, _ := getOrbit(cfg)
		i := orbitIndex(cfg, cur)
		i = (i - 1 + len(cfg.Orbits)) % len(cfg.Orbits)
		return setOrbit(cfg.Orbits[i])
	default:
		return fmt.Errorf("usage: orbit [get|set <label>|next|prev]")
	}
}

func cmdRole(args []string, cfg *Config) error {
	if len(args) < 2 {
		return fmt.Errorf("usage: role <jump|focus|seed> <role> [flags]")
	}
	sub, role := args[0], args[1]
	orbit, err := getOrbit(cfg)
	if err != nil {
		return err
	}
	ws := wsName(role, orbit)

	switch sub {
	case "jump":
		return dispatch("workspace", fmt.Sprintf("name:%s", ws))

	case "focus":
		// flags: --match X --cmd "Y"
		var match, spawn string
		for i := 2; i < len(args); i++ {
			if args[i] == "--match" && i+1 < len(args) {
				match = args[i+1]
				i++
			}
			if args[i] == "--cmd" && i+1 < len(args) {
				spawn = args[i+1]
				i++
			}
		}
		if match == "" {
			return fmt.Errorf("--match required")
		}
		m, err := parseMatcher(match)
		if err != nil {
			return err
		}

		cs, err := clients()
		if err != nil {
			return err
		}
		// 1) in target ws
		for _, c := range cs {
			if c.Workspace.Name == ws && m.match(c) {
				return dispatch("focuswindow", "address:"+c.Address)
			}
		}
		// 2) anywhere -> move + focus
		for _, c := range cs {
			if m.match(c) {
				if err := dispatch("movetoworkspace", fmt.Sprintf("name:%s address:%s", ws, c.Address)); err != nil {
					return err
				}
				return dispatch("focuswindow", "address:"+c.Address)
			}
		}
		// 3) spawn in ws
		if spawn != "" {
			if err := dispatch("workspace", "name:"+ws); err != nil {
				return err
			}
			return dispatch("exec", spawn)
		}
		// no spawn: just jump
		return dispatch("workspace", "name:"+ws)

	case "seed":
		// if target ws empty, iterate configured entries for role
		cs, err := clients()
		if err != nil {
			return err
		}
		count := 0
		for _, c := range cs {
			if c.Workspace.Name == ws {
				count++
			}
		}
		if count > 0 {
			return nil
		}
		entries := cfg.Roles[role]
		for _, e := range entries {
			if e.Match == "" {
				continue
			}
			args := []string{"role", "focus", role, "--match", e.Match}
			if e.Cmd != "" {
				args = append(args, "--cmd", e.Cmd)
			}
			if err := cmdRole(args, cfg); err != nil {
				return err
			}
		}
		return nil

	default:
		return fmt.Errorf("usage: role <jump|focus|seed> <role> [--match ... --cmd ...]")
	}
}

func main() {
	cfg, err := loadConfig()
	if err != nil {
		fmt.Fprintln(os.Stderr, "config:", err)
		os.Exit(1)
	}

	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: hypr-orbits <orbit|role> ...")
		os.Exit(2)
	}
	switch os.Args[1] {
	case "orbit":
		if err := cmdOrbit(os.Args[2:], cfg); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
	case "role":
		if err := cmdRole(os.Args[2:], cfg); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
	default:
		fmt.Fprintln(os.Stderr, "usage: hypr-orbits <orbit|role> ...")
		os.Exit(2)
	}
}
