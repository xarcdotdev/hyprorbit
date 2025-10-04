<a id="readme-top"></a>

<!-- PROJECT LOGO -->
<div align="center">
    <img src="docs/images/logo.webp" alt="Logo" width="150" height="150">
    <!-- <h1>**Ø**</h1> -->

<h2 align="center">hyprørbit</h2>

  <p align="center">
Lightweight workspace orchestration for Hyprland with sub-5ms responsiveness.
    <br>

**hyprorbit** is a stateful daemon + lightweight client system for Hyprland workspace management, written in Go.

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

<!-- ABOUT THE PROJECT -->
## What is hyprorbit?

**The Problem**: You've organized your Hyprland workspaces perfectly: SUPER+1 for coding, SUPER+2 for communication, etc. But what happens when you need to switch contexts? Work on a different project? Take a break? You either clutter your organized workspace or lose your setup entirely.

**The Solution**: hyprorbit gives you multiple "orbits" (contexts) with identical workspace layouts. Switch between a work orbit, personal orbit, or emergency-fix orbit while keeping your muscle memory intact.

- Same keybindings across all contexts
- Clean workspace separation by project/mood
- Instant context switching without losing your setup

Use `hyprorbit module focus code` to instantly switch to your code workspace.
Use `hyprorbit orbit next` to cycle through your configured orbits.

<!-- <div align="center">
![Product Demo][product-screenshot]
</div> -->

### Key Features

- **Sub-5ms response times** via persistent daemon
- **Orbit contexts** - separate workspace sets for different projects
- **Stable hotkeys** - same keybindings across all orbits
- **Focus-or-launch** - smart window management prevents duplicates
- **Regex window matching** by class, title, or initial properties

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
go build ./cmd/hyprorbitd
go build ./cmd/hyprorbit
```

### 3. Start Daemon

```sh
# Start the daemon
./hyprorbitd

# Or with custom config
./hyprorbitd --config ~/.config/hyprorbit/config.yaml
```

### 4. Configure Hyprland Keybinds

Add to your `~/.config/hypr/hyprland.conf`:

```bash
# Module hotkeys (stable across orbits)
bind = SUPER, 1, exec, hyprorbit module focus code
bind = SUPER, 2, exec, hyprorbit module focus comm
bind = SUPER, 3, exec, hyprorbit module focus gfx

# Orbit switching
bind = SUPER, comma, exec, hyprorbit orbit prev
bind = SUPER, period, exec, hyprorbit orbit next

# Quick workspace jumping
bind = SUPER SHIFT, 1, exec, hyprorbit module jump code
```

### 5. Basic Usage

```sh
# Check current orbit
hyprorbit orbit get

# Switch to beta orbit
hyprorbit orbit set beta

# Focus or launch code module
hyprorbit module focus code --match "class=.*Code"
```

See `hyprorbit --help` for full options.

<p align="right">(<a href="#readme-top">back to top</a>)</p>

## Usage

### Core Concepts

**Orbits**: Independent workspace contexts (e.g., `alpha`, `beta`, `gamma`)
- Switch between complete workspace sets
- Maintain separate contexts for different projects/moods
- Default orbits with Greek letter labels: α, β, γ

**Modules**: Workspace categories that anchor your hotkeys (e.g., `code`, `comm`, `gfx`)
- Consistent across all orbits
- `SUPER+1` always goes to "code" regardless of active orbit
- Modules create workspaces like `code-alpha`, `comm-beta`, etc.

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

**Focus or launch:**
```sh
# Focus existing window, move if needed, or spawn new
hyprorbit module focus code

# With custom matcher
hyprorbit module focus code --match "class=.*VSCode"

# With spawn command override
hyprorbit module focus code --cmd "code"

# Prevent window moving
hyprorbit module focus code --no-move
```

Modules bundle the matching and launch behaviour in `config.yaml`, so `module focus` always takes a **module name** (e.g., `thunar`). Each definition provides the matcher and fallback command, which lets hyprorbit:
1. Resolve the active orbit and target workspace (`<module>-<orbit>`)
2. Focus a matching window already in that workspace
3. Otherwise move a matching window from elsewhere in the orbit
4. Otherwise spawn the configured command, guaranteeing a single window per module/orbit

Flags like `--match`, `--cmd`, `--float`, and `--no-move` temporarily override the config when you need custom behaviour.

**Jump to workspace:**
```sh
# Simple workspace switch (no window management)
hyprorbit module jump code
```


### Window Commands

**Move current window to another module:**
```sh
# Move focused window to next module workspace and focus it
hyprorbit window move current module:next

# Move silently (do not change focus)
hyprorbit window move current module:comm --silent

# Create a temporary workspace in the active orbit and move window there
hyprorbit window move current module:create
```

**Supported module targets:**
- `module:<name>` – explicit module name (e.g., `module:code`)
- `module:next` / `module:prev` – cycle through configured modules
- `module:index:<n>` – zero-free index (1-based) into the module list
- `module:regex:<pattern>` – first module matching the given regex
- `module:create` – spawn a temporary workspace (`<n>-<orbit>`) before moving

By default moves focus to the destination workspace; pass `--silent` to keep focus on the current workspace after the move.


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
  - name: "beta"
    label: "β"
  - name: "gamma"
    label: "γ"

modules:
  code:
    focus:
      match: "class=.*Code"              # Window matcher
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

**Match format:** `field=regex`

```sh
# Match by class
hyprorbit module focus code --match "class=.*Code"

# Match by title
hyprorbit module focus browser --match "title=.*Firefox"
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

### Systemd Service

```ini
[Unit]
Description=Hypr Orbits Daemon
After=graphical-session.target

[Service]
Type=simple
ExecStart=/usr/bin/hyprorbitd
Restart=on-failure

[Install]
WantedBy=default.target
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
hyprorbit module focus code --match "class=.*" --no-move
```

<p align="right">(<a href="#readme-top">back to top</a>)</p>

## Roadmap

### Current Status
- ✅ Basic orbit management
- ✅ Module focus/jump commands
- ✅ Window matching system
- ✅ Configuration system
- ✅ Daemon implementation
- ✅ IPC protocol
- ✅ Client-server architecture
- ✅ Waybar/status bar module
- ✅ Native Hyprland socket integration

### Planned Features
- 📋 Module seeding (populate workspace with multiple apps)
- 💾 State snapshots across reboots
- 🔔 Desktop notifications
- 📦 Package distribution (AUR, Nix)

<p align="right">(<a href="#readme-top">back to top</a>)</p>

## Contributing

Issues and PRs welcome! Please check existing issues before creating new ones.

### Development
```sh
# Clone repository
git clone https://github.com/xarcdotdev/hyprorbit.git
cd hyprorbit

# Build both binaries
make build

# Run tests
make test

# Development with live reload
make dev
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
