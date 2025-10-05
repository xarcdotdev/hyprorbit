<a id="readme-top"></a>

<!-- PROJECT LOGO -->
<div align="center">
    <img src="docs/images/logo.webp" alt="Logo" width="150" height="150">
    <!-- <h1>**Ø**</h1> -->

<h2 align="center">hyprørbit v0.1</h2>

  <p align="center">
Lightweight workspace orchestration for <a href="https://github.com/hyprwm/Hyprland">Hyprland Power-Users</a>.
    <br>

**hyprorbit** is a stateful daemon + client system for Hyprland workspace management, written in Go.

<br>

<a href="https://github.com/xarcdotdev/hyprorbit/issues/new?labels=bug&template=bug-report---.md">Report Bug</a>
&middot;
<a href="https://github.com/xarcdotdev/hyprorbit/issues/new?labels=enhancement&template=feature-request---.md">Request Feature</a>
  </p>

[![Contributors][contributors-shield]][contributors-url]
[![Go][Go-shield]][Go.dev]
[![Stargazers][stars-shield]][stars-url]
[![Issues][issues-shield]][issues-url]
[![License][license-shield]][license-url]

</div>

<!-- DISCLAIMER -->
## Disclaimer

⚠️ **This is experimental!** ⚠️ 

**Its probably better done via native Hyprland plugin. I just wanted to see how working with multiple sets of workspaces feels. (its nice for me so far)**

**Current limitiantions:**

- Window rules: Auto-assigning apps to workspaces via Hyprland windowrules are not orbit-aware.

- Waybar Integration: Default Hyprland workspace indicator for waybar works, but is not orbit-aware. <a href="docs/waybar_configuration.md">You can create a custom module</a>)  

**What’s good:**

- It’s lightweight and fast -> Good responsiveness & low system load.

- Works.


<!-- ABOUT THE PROJECT -->
## What is hyprorbit?

**The Problem**: You've organized your Hyprland workspaces: SUPER+1 for coding, SUPER+2 for communication, etc. But what happens when you need to switch contexts? Work on a different project? Take a break? You either clutter your organized workspace or lose your setup/workspace-hotkeys. Without having designated workspaces for applications I often tend to only work on a small portion of my screen or have to search for applications scattered across workspaces & manually reorganizing.

**The Solution**: hyprorbit gives you multiple "orbits" (contexts) with identical workspace layouts. Switch between a work orbit, personal orbit, or emergency-fix orbit while keeping your muscle memory intact.

- Clean workspace separation by project/mood
- Same keybindings across all contexts
- Instant context switching & smaller number of relevant workspaces

➡️ Use `hyprorbit module jump code` to instantly switch to your code workspace.

➡️ Use `hyprorbit orbit next` to cycle through your configured orbits.

<!-- <div align="center">
![Product Demo][product-screenshot]
</div> -->

### Key Features

- **Orbit contexts** - separate workspace sets for different projects
- **Stable hotkeys** - same workspace-keybindings across all orbits
- **Focus-or-launch** - focus windows in current orbit instead of launching
- **Sub-5ms response times** via persistent daemon

<p align="right">(<a href="#readme-top">back to top</a>)</p>

## Quickstart

### 1. Prerequisites

- [Hyprland](https://hyprland.org/) compositor
- Go 1.21+

### 2. Install

```sh
# Clone and build
git clone https://github.com/xarcdotdev/hyprorbit.git
cd hyprorbit
make

# Install to PATH (recommended for Hyprland exec-once)
sudo cp hyprorbit hyprorbitd /usr/local/bin/
```

### 3. Start Daemon

```sh
# Recommended: via Hyprland Config (order is important here)
exec-once = hyprorbitd # start daemon
exec-once = hyprorbit --autostart # for workspace initialization

# Or manually for testing
./hyprorbitd

# Or with custom config
./hyprorbitd --config ~/.config/hyprorbit/config.yaml
```

### 4. Configure Hyprland Keybinds

Add to your `~/.config/hypr/hyprland.conf`:

```bash
# Quick workspace jumping (stable across orbits)
bind = SUPER, 1, exec, hyprorbit module jump code
bind = SUPER, 2, exec, hyprorbit module focus comm
bind = SUPER, 3, exec, hyprorbit module focus gfx

# Orbit switching
bind = SUPER, comma, exec, hyprorbit orbit prev
bind = SUPER, period, exec, hyprorbit orbit next
bind = SUPER ALT, 1, exec, hyprorbit orbit set alpha
bind = SUPER ALT, 2, exec, hyprorbit orbit set beta

# Focus Applications
bind = SUPER, C, exec, hyprorbit module focus coding
bind = SUPER, G, exec, hyprorbit module focus email
```

### 5. Basic Usage

```sh
# Check current orbit
hyprorbit orbit get

# Switch to beta orbit
hyprorbit orbit set beta

# Focus or launch VScode
hyprorbit module focus email --match "class=.*thunderbird"
```

See `hyprorbit --help` for full options.

<p align="right">(<a href="#readme-top">back to top</a>)</p>

## Usage

### Core Concepts

**Orbits**: Independent workspace contexts (e.g., `alpha`, `beta`, `gamma`)
- Separate environments for different projects/contexts
- Default labels: α, β, γ
- Each orbit maintains its own window instances

**Modules**: Workspace categories bound to consistent hotkeys (e.g., `code`, `comm`, `gfx`)
- `SUPER+1` → "code" module in whichever orbit you're in
- Generate orbit-specific workspaces: `code-alpha`, `comm-beta`, etc.
- Windows bind to modules via pattern matching

### Commands

| Command            | Description                                     |
| ------------------ | ----------------------------------------------- |
| `hyprorbit orbit get`    | Show current active orbit                      |
| `hyprorbit orbit set <name>` | Switch to specific orbit                   |
| `hyprorbit orbit next/prev` | Cycle through configured orbits             |
| `hyprorbit module focus <name>` | Smart focus-or-launch for module        |
| `hyprorbit module jump <name>` | Simple workspace switching               |
| `hyprorbit window move <window> <target>` | Move/focus windows across modules |

### Orbit Commands

**Get current orbit:**
```sh
hyprorbit orbit get
# Output: alpha	α	#BC83F9
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
```

**Module Focus Mechanism**

Instead of Hyprland's windowrules, hyprorbit uses `module focus <name>` to assign windows to workspaces. You define each module in `config.yaml` with a matcher pattern and fallback launch command.

**How it works::**
1. Switch to module workspace in current orbit (`comm-alpha`)
2. Search for matching windows (`class:^signal$|^telegram$`)
3. Then:
   - **Focus** if match exists in workspace
   - **Move** if match exists elsewhere in orbit
   - **Spawn** fallback command if no match found

Result: One window per module/orbit, automatically placed. You can still manually open applications elsewhere when needed.

**Overrides:** `--match`, `--cmd`, `--float`, `--no-move`

**Advanced:** A window can belong to multiple modules. Your email client could live in both `comm` and `outreach`, accessible from either module's workspace.


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
```

**Supported module targets:**
- `module:<name>` – explicit module name (e.g., `module:code`)
- `module:next` / `module:prev` – cycle through configured modules
- `module:index:<n>` – zero-free index (1-based) into the module list
- `module:regex:<pattern>` – first module matching the given regex
- `module:create` – spawn a temporary workspace (`<n>-<orbit>`) before moving

By default moves focus to the destination workspace; pass `--silent` to keep focus on the current workspace after the move.

**Window selectors:**
- `current` – focused window (default)
- `workspace` – every window currently on the active workspace

- `orbit:<selector>` – look across the entire active orbit (e.g. `orbit:class:foot`)
- `global:<selector>` – look across every workspace (e.g. `global:title:"Media"`)

- `class:<pattern>` – regex applied to the window class (e.g. `class:"(?i)firefox"`)
- `title:<pattern>` – regex applied to the window title
- `initialClass:<pattern>` / `initialTitle:<pattern>` – regex applied to the initial values Hyprland recorded
- `tag:<pattern>` – regex applied to Hyprland window tags (opt-in)
- `regex:<pattern>` – applies regex to all fields (class/title/initial)

Selectors default to the active workspace; prefix with `orbit:` or `global:` to expand the search scope.

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

<p align="right">(<a href="#readme-top">back to top</a>)</p>

## Configuration

### File Location
- **Default**: `~/.config/hyprorbit/config.yaml`
- **Override**: `--config <path>` flag
- **Environment**: `HYPR_ORBITS_SOCKET` for custom socket path

### Configuration Schema

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
  code:
    focus:
      match: "class=.*code"              # Window matcher
      cmd: ["ghostty", "-T", "Code"]       # Spawn command
  comm:
    focus:
      match: "title=.*Slack"
      cmd: ["flatpak", "run", "com.slack.Slack"]
  gfx:
    focus:
      match: "class=.*GIMP"
      cmd: ["gimp"]

defaults:
  float: false          # Default floating behavior
  move: true           # Default move behavior
```

### Window Matching

**Supported fields:**
- `class`: Window class name
- `title`: Window title
- `initialClass`: Class at window creation
- `initialTitle`: Title at window creation

**Match format:** `field:regex` (legacy `field=regex` remains supported)

```sh
# Match by class
hyprorbit module focus code --match "class:^.*Code$"

# Match by title
hyprorbit module focus browser --match "title:.*Firefox"
```

### Daemon Configuration

**Socket location:**
- Primary: `$XDG_RUNTIME_DIR/hyprorbit.sock`
- Fallback: `/tmp/hyprorbit-$UID.sock`

**Logging:**
```sh
# Start with debug logging
hyprorbitd --log-level debug

# JSON format logging
hyprorbitd --log-format json
```

**Manual config reload:**
```sh
# Send HUP signal to reload config
kill -HUP $(pgrep hyprorbitd)
```

<p align="right">(<a href="#readme-top">back to top</a>)</p>

## Advanced Usage

### Status Bar Integration

For a more performant and configurable waybar integration check out docs/WAYBAR.md

**Waybar example:**
```json
{
  "custom/hyprorbit": {
    "exec": "hyprorbit orbit get",
    "interval": 1,
    "format": "🌌 {}"
  }
}
```

<p align="right">(<a href="#readme-top">back to top</a>)</p>

## Troubleshooting

### Common Issues

**Daemon not starting:**
```sh
# Check if already running
pgrep hyprorbitd

# Check logs
hyprorbitd --log-level debug
```

**Socket connection failed:**
```sh
# Check socket permissions
ls -la $XDG_RUNTIME_DIR/hyprorbit.sock

# Use custom socket path
hyprorbit --socket /tmp/my-orbits.sock orbit get
```

**Window matching not working:**
```sh
# Debug window properties
hyprctl clients -j | grep -A 10 -B 10 "YourApp"

# Test matchers
hyprorbit module focus code --match "class:.*" --no-move
```

<p align="right">(<a href="#readme-top">back to top</a>)</p>

## Roadmap

### Current Status
- ✅ Initial Configuration system
- ✅ Orbit management
- ✅ Module focus/jump commands
- ✅ Window matching system
- ✅ Shell Completion Script
- ✅ Waybar/status support
- ✅ Multi Monitor Support
- ✅ Client-server architecture (IPC)
- ✅ Native Hyprland socket integration

### Planned Features
- [ ] Configurable notifications
- [ ] Module seeding (populate workspace with multiple apps)
- [ ] Hyprland tag support for adressing windows
- [ ] Making window targeting global instead only of only  in focused workspace  

<p align="right">(<a href="#readme-top">back to top</a>)</p>

## Contributing

Issues and PRs welcome! Please check existing issues before creating new ones.

### Development
```sh
# Clone repository
git clone https://github.com/xarcdotdev/hyprorbit.git
cd hyprorbit

# Build both binaries
make

# Run tests
make test
```

<p align="right">(<a href="#readme-top">back to top</a>)</p>

## License

This project is licensed under the MIT License.
See `LICENSE` for details.

<p align="right">(<a href="#readme-top">back to top</a>)</p>

## Contact

Project Link: [https://github.com/xarcdotdev/hyprorbit](https://github.com/xarcdotdev/hyprorbit)

<p align="right">(<a href="#readme-top">back to top</a>)</p>

<!-- MARKDOWN LINKS & IMAGES -->
[contributors-shield]: https://img.shields.io/github/contributors/xarcdotdev/hyprorbit.svg?style=for-the-badge
[contributors-url]: https://github.com/xarcdotdev/hyprorbit/graphs/contributors
[forks-shield]: https://img.shields.io/github/forks/xarcdotdev/hyprorbit.svg?style=for-the-badge
[forks-url]: https://github.com/xarcdotdev/hyprorbit/network/members
[stars-shield]: https://img.shields.io/github/stars/xarcdotdev/hyprorbit.svg?style=for-the-badge
[stars-url]: https://github.com/xarcdotdev/hyprorbit/stargazers
[issues-shield]: https://img.shields.io/github/issues/xarcdotdev/hyprorbit.svg?style=for-the-badge
[issues-url]: https://github.com/xarcdotdev/hyprorbit/issues
[license-shield]: https://img.shields.io/github/license/xarcdotdev/hyprorbit.svg?style=for-the-badge
[license-url]: https://github.com/xarcdotdev/hyprorbit/blob/master/LICENSE
[product-screenshot]: docs/images/demo.gif

[Go.dev]: https://go.dev/
[Go-shield]: https://img.shields.io/badge/Go-00ADD8?style=for-the-badge&logo=go&logoColor=white
