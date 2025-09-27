# Task Plan for hypr-orbits MVP

## 1. Project Bootstrap
- Initialize Cobra-based CLI structure (`cmd/root.go`, subcommands for orbit/module).
- Wire shared dependencies (config loader, hyprctl client, orbit tracker) into Cobra persistent pre-run without global mutable state.
- Configure Go module tidy/lint targets (no extra tooling).

## 2. Configuration Layer
- Define config structs mirroring IMPLEMENTATION.md (orbits with `name`/`label`/`color`, modules with focus rules, defaults block).
- Implement YAML loader with XDG path resolution and optional `--config` flag; include default fallbacks when file missing.
- Validate orbit names (`[A-Za-z0-9]+`), ensure at least one orbit/module, and convert config into in-memory records reused per command invocation.
- Expose a lightweight provider interface so future caching/daemon modes can reuse the same contract.

## 3. State Handling
- Implement orbit state file read/write in `~/.local/state/hypr-orbits/orbit`, persisting only the orbit name.
- Provide atomic write helper (temp file + rename) and graceful fallback to the first configured orbit when state missing/corrupt.
- Ensure state helpers are invoked once per command path to minimize filesystem I/O.
- Document extension points for in-memory caching if a resident process is introduced later (no implementation yet).

## 4. Hyprctl Integration Layer
- Wrap `hyprctl` calls in a package that exposes `Dispatch(ctx, args...)` and `Clients(ctx)` helpers with JSON parsing.
- Apply per-call context timeouts to avoid hanging commands; surface stderr/stdout in rich errors.
- Reuse a single `Clients` snapshot per command to reduce repeated invocations.

## 5. Orbit Command Implementation
- Implement `orbit get` (emit current name plus optional label/color metadata).
- Implement `orbit next`/`prev` with wrap-around sequencing over configured orbit records.
- Implement `orbit set <name>` validating membership and updating state atomically.
- Ensure command outputs are concise and machine-friendly for piping to other tools.

## 6. Module Command Implementation
- Implement `module jump <module>`: resolve orbit/module workspace (`<module>-<orbitName>`), dispatch workspace jump, report errors clearly.
- Implement `module focus <module>`: resolve workspace, fetch clients once, apply matcher precedence, move/focus windows within the active orbit, spawn process if needed with inherited environment, honor `--float`/`--no-move` flags.
- Share helper functions for workspace naming, client filtering, and process spawning to keep the codebase modular.

## 7. CLI Wiring & Flags
- Expose global flags (`--config`, `--verbose`, future-friendly `--json`) on the root command.
- Register orbit/module subcommands, including flag bindings for `module focus` (`--match`, `--cmd`, `--float`, `--no-move`).
- Ensure flag parsing composes with Cobra’s persistent pre-run so shared setup happens once per invocation.

## 8. Performance Pass
- Audit commands to confirm single-pass config/state reading and client retrieval.
- Confirm all `hyprctl` calls reuse prepared arguments and avoid unnecessary string allocations.
- Add context timeouts and consistent error handling to keep execution snappy.
- Note TODO hooks for future in-memory caches when running as a daemon (leave unimplemented).

## 9. Final Polish
- Run `go fmt` and `go vet` to ensure clean build.
- Update documentation references if interfaces/flags differ from the spec.
- Skip automated tests per scope; perform manual smoke checks for orbit/module commands.
