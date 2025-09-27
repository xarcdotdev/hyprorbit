# hypr-orbits

**Task-centric context switching for Hyprland.**  
Stable hotkeys per *role* (code, gfx, comm, …). Multiple *contexts* (α, β, γ …) you can cycle through.  
Each role+context maps to a named workspace like `code-α`, `gfx-α`, `comm-α`.

- `orbit next/prev/get/set` — switch the active **context** (α → β → γ …).
- `role jump <role>` — jump to the role’s workspace in the current context.
- `role focus <role> --match <pcre> --cmd <spawn>` — focus existing window for this role, or launch it **in** the role’s workspace.
- `role seed <role>` — optional: seed the role workspace with its default apps if empty.

No compositor plugin. Just a small Go CLI that talks to `hyprctl`.

---

## Why

- Keep **hotkeys stable** per task role (e.g., `SUPER+1` is always “code primary”), while switching **contexts** when you need parallel threads of work.
- Prevent duplicate windows and entropy: “focus existing → or launch → and place it correctly”.
- Don’t fight Hyprland with hardcoded window rules; drive placement from your intent.

---

## Concepts

- **Role**: a task slot, e.g. `code`, `gfx`, `comm`, `write`, `research`…
- **Context**: a family label (default **Greek**): `α`, `β`, `γ`, …  
- **Workspace name**: `<role>-<context>` → e.g. `code-α`, `gfx-β`.

You bind your keys to roles; cycling contexts re-targets those keys to a different family.

---

## Defaults

- Context labels (orbit): `α β γ δ ε …`
- Roles: you declare in config; example includes `code`, `gfx`, `comm`.

---

## Install (MVP)

```bash
# Build
go build -o hypr-orbits ./cmd/hypr-orbits

# Install
install -Dm755 hypr-orbits "$HOME/.local/bin/hypr-orbits"

