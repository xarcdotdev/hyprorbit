# Commands


### Orbit Commands

**Get current orbit:**
```sh
hyprorbit orbit get
# Output: alpha	α
```

**Switch orbits:**
```sh
# Specific orbit
hyprorbit orbit set beta

# Cycle through orbits
hyprorbit orbit next
hyprorbit orbit prev
```

### Module Commands

**Jump to workspace:**
```sh
# Simple workspace switch in current orbit
hyprorbit module jump code
```

**Focus or launch:**
```sh
# Focus existing window, move if needed, or spawn new
hyprorbit module focus email

# With custom matcher (field:regex; legacy field=regex still works)
hyprorbit module focus email --match "title:Thunderbird"

# With spawn command override
hyprorbit module focus email --cmd "thunderbird"

# Prevent window moving
hyprorbit module focus email --no-move

# Search for matching clients across all orbits (for single-instance apps)
hyprorbit module focus email --global
```

**Module Focus Mechanism**

Instead of Hyprland's windowrules, hyprorbit uses `module focus <name>` to assign windows to workspaces. You define each module in `config.yaml` with a matcher pattern and fallback launch command.

**How it works::**
1. Switch to module workspace in current orbit (`comm-alpha`)
2. Search for matching windows (`class:^signal$|^telegram$`)
3. Then:
   - **Focus** if match exists in workspace
   - **Move** if match exists elsewhere in orbit
   - **Move from global** (with `--global`) if match exists in any other orbit
   - **Spawn** fallback command if no match found

Result: One window per module/orbit, automatically placed. You can still manually open applications elsewhere when needed.

### Window Commands

**Move current window to another module:**
```sh
# Move focused window to next module workspace and focus it
hyprorbit window move current module:next

# Move silently (do not change focus)
hyprorbit window move current module:comm --silent

# Create a temporary workspace in the active orbit and move window there
hyprorbit window move current module:create

# Move every window on the active workspace
hyprorbit window move workspace module:next

# Move matching windows (by class) on the active workspace
hyprorbit window move class:"^firefox$" module:comm

# Move all windows from all orbits (global search)
hyprorbit window move all module:code --global 

# Move matching windows from all orbits
hyprorbit window move class:"^firefox$" module:comm --global 

# Move focused window to the code module in the beta orbit
hyprorbit window move current code/beta

# Explicit orbit target syntax
hyprorbit window move current module:code/orbit:beta
```

//TODO: test module:index & module:regex & module:create
**Supported targets:**
- `module:<name>` – explicit module name (e.g., `module:code`)
- `module:next` / `module:prev` – cycle through configured modules
- `module:index:<n>` – zero-free index (1-based) into the module list
- `module:regex:<pattern>` – first module matching the given regex
- `module:create` – spawn a temporary workspace (`<n>-<orbit>`) before moving
- `module:<name>/orbit:<orbit>` – move directly into a module on a specific orbit; shorthand `<module>/<orbit>` also works

**Flags:**
- `--silent` – keep focus on the current workspace after moving (default: false)
- `--global` – search for windows across all orbits instead of current orbit only (default: false)

**Window selectors:**
- `current` – focused window (default)
- `workspace` – every window currently on the active workspace
- `all` – every window across all workspaces (excludes special workspaces)

- `orbit:<selector>` – look across the entire active orbit (e.g. `orbit:class:foot`)
- `global:<selector>` – look across every workspace (e.g. `global:title:"Media"`)

- `class:<pattern>` – regex applied to the window class (e.g. `class:"(?i)firefox"`)
- `title:<pattern>` – regex applied to the window title
- `initialClass:<pattern>` / `initialTitle:<pattern>` – regex applied to the initial values Hyprland recorded
- `tag:<pattern>` – regex applied to Hyprland window tags (opt-in)
- `regex:<pattern>` – applies regex to all fields (class/title/initial)

### Output Formats

**Human-readable (default):**
```sh
$ hyprorbit orbit get
alpha	α	#BC83F9

$ hyprorbit module focus code
focused	code-alpha	alpha
```

**JSON mode:**
```sh
$ hyprorbit orbit get --json
{"name":"alpha","label":"α","color":"#BC83F9"}

$ hyprorbit module focus code --json
{"action":"focused","workspace":"code-alpha","orbit":"alpha"}

$ hyprorbit module watch --waybar
{"alt":"comm-alpha","class":["comm","alpha"],"text":"comm","tooltip":"α"}
```

**Quiet mode:**
```sh
$ hyprorbit orbit get --quiet
# Minimal output, errors to stderr
```
