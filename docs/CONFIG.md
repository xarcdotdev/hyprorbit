# Configuration

Default location: ~/.config/hyprorbit/conig.yaml

```yaml
orbits:
  - name: "alpha"       # Required: alphanumeric identifier
    label: "α"          # Optional: display text
    color: "#BC83F9"  # Optional: isnt used yet
  - name: "beta"
    label: "β"
  - name: "gamma"
    label: "γ"

modules:
  flex:
  code:
    focus:
      logic: try-all
      rules:
        # - match: "class:(ghostty|Alacritty|kitty)$"
        #   cmd: ["ghostty", "+new-window"]
        - match: "class:^(code)$"
          cmd: ["code"]
  surf:
    focus:
      rules:
        - match: "class:^(zen)"
          cmd: ["zen-browser"]
  comm:
    focus:
      rules:
        - match: "class:^(thunderbird)$"
          cmd: ["thunderbird"]
  media:
    focus:
      rules:
        - match: "class:^(mpv)$"
          cmd: ["mpv"]
  note:
    focus:
      rules:
        - match: "class:^(obisidan)$"
        - cmd: ['obsidian']

defaults:
  float: false          # Default floating behavior
  move: true           # Default move behavior
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
