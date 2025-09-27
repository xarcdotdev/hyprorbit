# Implementation (MVP)

## Goals
- Provide task-centric switching that layers on top of Hyprland without replacing its workspace model.
- Keep hotkeys stable per module while allowing users to cycle through independent context "orbits".
- Offer a single binary with sane defaults so a fresh install works without a config file.
- Prevent duplicate windows by preferring "focus existing" before spawning, while ensuring launches land in the correct module workspace.

## High-Level Flow
1. Load configuration (or defaults) describing available modules, their focus/launch rules, and orbit definitions.
2. Read the active orbit name from state storage; map it to the configured display label/color, falling back to the default orbit if missing.
3. Each CLI command performs its operation by resolving the target workspace name `<module>-<orbitName>` and issuing the appropriate `hyprctl` dispatches.
4. Changes to the active orbit are persisted atomically so external status bars can observe them.

## CLI Surface (MVP)
```
hypr-orbits orbit get
hypr-orbits orbit next|prev|set <name>
hypr-orbits module jump <module>
hypr-orbits module focus <module> [--match <field=regex>] [--cmd <spawn>] [--float] [--no-move]
hypr-orbits module seed <module>
```

- `orbit get` prints the current orbit name and, if defined, its label/color.
- `orbit next|prev` cycles through the configured orbit list; wraps around.
- `orbit set <name>` validates that `<name>` exists and writes it to state.
- `module jump` focuses the workspace associated with `<module>` in the current orbit.
- `module focus` performs focus-or-launch: find a matching client in the target module workspace, else move a matching client into place, else launch a new process in that workspace.
- `module seed` (optional in config) primes a module workspace by running its configured launch chain when empty.

## Configuration
- Optional file: `~/.config/hypr-orbits/config.yaml`.
- If missing, defaults to:
  - Orbits: `{name: "alpha", label: "α"}`, `{name: "beta", label: "β"}`, `{name: "gamma", label: "γ"}` (ASCII labels `a/b/c` for non-Unicode setups).
  - Modules: `code`, `gfx`, `comm`, each with an empty launch rule and label-friendly descriptions.
- Each orbit entry requires an alphanumeric `name` (used for state files, CLI arguments, and workspace names) and may define a human-facing `label` plus a CSS-style `color` hint for status bars.
- Schema outline:
  ```yaml
  orbits:
    - name: "alpha"
      label: "α"
      color: "#BC83F9"
    - name: "beta"
      label: "β"
  modules:
    code:
      hotkey: "SUPER+1"
      focus:
        match: "class=.*Code"
        cmd: ["kitty", "-T", "Code"]
        workspace_type: "stack"
    comm:
      focus:
        match: "title=.*Slack"
        cmd: ["flatpak", "run", "com.slack.Slack"]
  defaults:
    float: false
    move: true
  ```
- Configuration parsing should be tolerant: unknown keys trigger warnings but not fatal errors.
- Provide an `--config <path>` override for advanced users; otherwise fall back to `$XDG_CONFIG_HOME` detection.

### Orbit Representation
- `name` is the canonical identifier. Accept only `[A-Za-z0-9]+` to guarantee compatibility with Hyprland workspace naming and filesystem storage.
- `label` is optional and can include UTF-8 glyphs (e.g., Greek letters) for display in bars or notifications; when omitted, render the name as-is.
- `color` is optional (hex or CSS color string) for downstream status bars; ignore it in core logic but surface it via `orbit get`.
- Defaults ship with `name` values `alpha`, `beta`, `gamma` and matching Greek labels; provide ASCII fallbacks (`A/B/C`) for setups that avoid Unicode.
- CLI commands accept and persist orbit names; user-facing output may include `label` and `color` metadata for richer integrations.

### Module Concept
- A *module* is a named slice of an orbit, e.g. `code-alpha` internally and displayed as `code-α` when the orbit exposes that label.
- Modules anchor your keybinds: `SUPER+1` can always jump to the "code" module in whichever orbit is active.
- Each orbit exposes the full grid of modules, so switching orbits feels like flipping to a parallel set of work surfaces.

## State Persistence
- Active orbit file: `~/.local/state/hypr-orbits/orbit`.
- Persist the orbit `name` string, not the label, so downstream tooling can look up the full record deterministically.
- Writes are atomic (temp file + rename) to avoid partial reads by status bars.
- When the state file is missing or corrupt, default to the first orbit in configuration and recreate the file.

## Workspace Naming & Discovery
- `workspaceName(module, orbitName) = fmt.Sprintf("%s-%s", module, orbitName)`.
- Orbit names are alphanumeric, ensuring the resulting workspace identifiers remain compositor-friendly; display labels stay in user-facing channels only.
- Query `hyprctl workspaces -j` to see if the workspace already exists; if not, switching to it with `dispatch workspace name:<name>` creates it lazily.

## Hyprland Integration
- Shell out to `hyprctl` for dispatches and queries; no long-lived socket connection in the MVP.
- Commands used:
  - `hyprctl dispatch workspace name:<ws>` to jump to a workspace.
  - `hyprctl dispatch movetoworkspace name:<ws> address:<addr>` to move a client.
  - `hyprctl dispatch focuswindow address:<addr>` to focus an existing client.
  - `hyprctl clients -j` to inspect window metadata (class, title, workspace ID, floating state, etc.).
- Always render `<ws>` as `<module>-<orbitName>` so Hyprland receives stable ASCII-friendly identifiers irrespective of the display label.
- All hyprctl errors are surfaced with actionable messages and non-zero exit codes.

## Module Workflows

### module jump
- Resolve target workspace via the current orbit and the requested module.
- Call `dispatch workspace name:<ws>`; if Hyprland returns success, exit 0.
- If the module is undefined, report the available modules and exit with code 2.

### module focus
1. Determine the target workspace from module + orbit name.
2. Load clients and filter by module configuration:
   - If `--match field=regex` is provided on the CLI, prefer it.
   - Else use the module’s configured matcher (supports `class`, `title`, `initialClass`, `initialTitle`).
3. Search order (always constrained to the active orbit):
   - Clients already in the target module workspace for the current orbit → focus the first match.
   - Matching clients in other workspaces → move the client into the target module workspace within the same orbit, then focus. When moving, always dispatch to `<module>-<orbitName>` so the window lands in the orbit that initiated the command.
   - No match → if a `cmd` is available (CLI or config), spawn it with `HYPRLAND_INSTANCE_SIGNATURE` preserved; record the PID for optional wait.
4. Optional flags:
   - `--float` sets the spawned window to float by emitting `dispatch togglefloating`; obey module defaults if flag absent.
   - `--no-move` only focuses existing clients in place; skips move/launch.
5. Output: minimal status text so wrappers (Waybar, keybind scripts) can react.

### module seed
- Triggered manually to populate an empty module workspace.
- Before spawning, check clients in the target workspace; if non-zero, noop.
- For each entry in the module’s `seed` array, call `module focus` with its specific `--match`/`--cmd` overrides.

## Orbit Management
- Maintains an ordered slice of orbit records (`name`, optional `label`, optional `color`).
- `next`/`prev` wrap around; pointer math happens on the record list while persisted state carries only the orbit name.
- After updating the orbit file, print the orbit name on stdout and include label/color in structured output when relevant (e.g., `--json`).
- Provide a `--notify` flag guarded behind `notify-send` detection to announce orbit changes; notifications should use the orbit label when available and fall back to the name.
- Treat labels/colors as opaque display metadata; avoid byte-length assumptions so multi-glyph labels remain intact.

## Error Handling and Observability
- Return exit code 0 on success, 1 for runtime/Hyprland errors, 2 for user/config errors.
- Log to stderr with concise messages prefixed by the command (e.g., `orbit: failed to read state`).
- Offer `--verbose` to dump JSON payloads when diagnosing `hyprctl` issues; the flag can be global via persistent pre-run.
- Do not spam notifications; respect Hyprland’s philosophy of minimal intrusive UI.

## Performance
- Cache the parsed configuration and active orbit record in memory per process invocation; avoid repeated file I/O on a single command path.
- Prefer a single `hyprctl clients -j` call per command and reuse its results instead of multiple invocations when focusing or seeding modules.
- Defer expensive JSON parsing until needed (e.g., avoid fetching clients when `module jump` simply dispatches a workspace switch).
- Keep spawned processes short-lived; rely on shelling out to `hyprctl` synchronously and exit immediately after command completion.
- Use Go’s context with tight timeouts for `hyprctl` to prevent hung calls from blocking workspace switches.
- Consider a lightweight in-memory cache for recent client lookups if a daemon mode is introduced later, but keep the MVP stateless to minimize overhead.

## Testing Approach
- Provide unit tests for:
  - Orbit parsing and sequencing (name validation, `next/prev/set` wrapping, Unicode labels).
  - Client matching filters (`field=regex` parsing, precedence rules).
  - Workspace name generation.
- Enable integration smoke tests behind a build tag to mock `hyprctl` responses.

## LATER
- Waybar/AGStatus module exposing current orbit and focused module.
- Native Hyprland socket subscriber to avoid repeated `hyprctl` calls.
- Snapshot & restore of module state across reboots.
- Packaging targets (AUR, Nix, Debian) and release automation.
- Notification helpers (libnotify) and richer telemetry.
- Persistent module descriptors (e.g., `workspace_type` layout hints) synced with Hyprland keywords.
