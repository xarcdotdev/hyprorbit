# Configuration

Default location: ~/.config/hyprorbit/config.yaml

```yaml

# Orbit definitions with names and labels (colors have no effect by default waybar only takes name(text) and label(alt) )
orbits:
  - name: "alpha"       # Required: alphanumeric identifier
    label: "α"          # Optional: display text
    color: "#BC83F9"  # Optional: isnt used yet
  - name: "beta"
    label: "β"
    color: "#F97583"
  - name: "gamma"
    label: "γ"
    color: "#85E89D"
  - name: "delta"
    label: "δ"
    color: "#FFAB70"
  - name: "epsilon"
    label: "ε"
    color: "#FFAB70"
  - name: "zeta"
    label: "ζ"
    color: "#FFAB70"
  - name: "theta"
    label: "η"
    color: "#FFAB70"
  - name: "iota"
    label: "ι"
    color: "#FFAB70"

# Module definitions with focus rules
modules:
  flex:
    
  code:
    focus:
      # try all matchers before focusing/executing 
      logic: try-all
      rules:
        - match: "class:^(code)$"
          cmd: ["code"]
        - match: "class:^(ghostty|Alacritty|kitty)$"
          cmd: ["ghostty", "new-window"]
        - match: "title:Hyprland Wiki"
          options: [move, float]

  surf:
    focus:
      rules:
        - match: "class:^(Firefox|zen)$"
          cmd: ["firefox"]
  
  comm:
    focus:
      rules:
        - match: "class:^thunderbird$"
          cmd: ["thunderbird"]

  media:
    focus:
      rules:
        - match: "class:^(mpv)$"
          cmd: ["mpv"]

defaults:
  float: false
  move: true

orbit:
  switch_preference: last-active-first
  orbit_cycle_mode: not-empty  # Optional: cycle populated orbits plus one empty fallback (default)

# Debug logging configuration
debug:
  enabled: false                       # Enable debug logging
  log_file: "/tmp/hyprorbit-debug.log" # Optional: defaults to /tmp/hyprorbit-debug.log
  dispatcher: false                    # Enable dispatcher-specific debug logs
  hyprctl: false                       # Enable hyprctl-specific debug logs
```

- `logic` defaults to `first-match-wins`; set `try-all` to chain through every rule even after an earlier action succeeds.
- `rules` is an ordered list of matcher/command pairs. Rules without a `cmd` simply attempt to focus or move matching windows.

## Window Matching

**Supported fields:**
- `class`: Window class name
- `title`: Window title
- `initialClass`: Class at window creation
- `initialTitle`: Title at window creation

**Match format:** `field:regex` (legacy `field=regex` remains supported)


## Daemon Configuration

**Socket location:**
- Primary: `$XDG_RUNTIME_DIR/hyprorbit.sock`
- Fallback: `/tmp/hyprorbit-$UID.sock`

```sh
hyprorbit daemon status
hyprorbit daemon reload # Reloads config
```
