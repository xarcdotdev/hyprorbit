MVP: what to implement
1) Config & State

Load ~/.config/hypr-orbits/config.yaml (optional; default orbits if missing).

Read/write current orbit label in ~/.local/state/hypr-orbits/orbit (atomic).

2) Orbit commands

get/next/prev/set cycling through labeled context/workspace-containers

3) Workspace naming

wsName(role, orbit) = role + "-" + orbit.

4) Hyprctl integration

Minimal helpers to:

dispatch("workspace", fmt.Sprintf("name:%s", ws))

dispatch("movetoworkspace", fmt.Sprintf("name:%s address:%s", ws, addr))

dispatch("focuswindow", fmt.Sprintf("address:%s", addr))

clients -j → parse into structs.

5) role jump

Jump to workspace name:<role>-<orbit>.

6) role focus

Parse --match into key+regex.

Search clients JSON (first by workspace, then global but within context).

Move/focus or spawn as above.

7) role seed (optional in MVP)

If target workspace has 0 clients, iterate role entries from config and call role focus for each.

8) Ergonomics

Return clear exit codes and stderr messages.
Optional: notify-send shell out (behind a flag) or skip for purity.



Roadmap (later)

Waybar module: show current orbit (α/β/γ) and active roles.

Add notify-send integration (optional flag).

Add --no-move policy for focus (just focus where it lives).

Snapshot/restore (best effort): record {role,context,cmd,title} and relaunch.

Native Hyprland socket integration (avoid hyprctl, subscribe to events).

Packaging: AUR (hypr-orbits-bin), Nix flake.

Tests for matcher + clients filter.
