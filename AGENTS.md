# AGENTS.md — Project Operating Guide

> **Purpose.** This file sets expectations for contributors (human or LLM) and stays in lock-step with the codebase. It outlines the project’s goals, features, hotkeys, requirements, and house rules for implementation and maintenance.

## 1) Project overview

**siftail** is a Go + Bubble Tea TUI for tailing and exploring logs from three sources:

* **File mode:** `siftail <path/to/file>` — tails a file and survives rotation/truncation.
* **Docker mode:** `siftail docker` — streams from all running containers; containers can be toggled on/off and saved as presets.
* **Streaming stdin:** `… | siftail` — reads piped input as a live stream.

### Core behavior

* Live, scrollable viewport with **nano-style** toolbar/hints.
* **Highlight (no scroll), Find (jump), Filter-in, Filter-out**.
* **Severity/level detection** (JSON, logfmt, common patterns, case insensitive) with **dynamic levels**: defaults map to `DEBUG, INFO, WARN, ERROR` (keys `1..4`) and new levels are assigned to slots `5..9`; overflow groups into **OTHER**.
* In Docker mode: **container list** (`l`) with per-container toggles, **All** toggle, and **named presets**.

## 2) Feature list (functional requirements)

* **Inputs:** file tail (rotation aware), stdin stream, Docker containers (stdout+stderr demux).
* **Find (**\`\`**):** highlight matches and navigate **Up/Down** across occurrences.
* **Highlight (**\`\`**):** visually mark text without scrolling to matches.
* **Filter-in (**\`\`**):** show only lines matching one or more terms/regexes.
* **Filter-out (**\`\`**):** hide lines matching terms/regexes.
* **Severity filters (**\`\`**):** toggle level buckets on/off; dynamic discovery for custom levels.
* **Docker controls:** list running containers, toggle individually or **All**, manage visibility **presets** (save/apply/delete).
* **Performance:** coalesced rendering; configurable ring buffer; handles long lines with soft wrapping; remains responsive under bursty input.

## 3) Hotkeys (default)

* **Global:** `Ctrl+Q` or `Ctrl+C` quit; `Esc` cancels current prompt.
* **Help:** `?` or `F1` opens help; `Esc`/`?` closes.
* **Highlight:** `h` → text box → **Enter** to add highlight (no scroll).
* **Find:** `Ctrl+F` → text box → **Enter** to activate; **Up/Down** jumps prev/next hit.
* **Filter-in:** `I` (capital i) → text box → **Enter** to apply.
* **Filter-out:** `O` (capital o) → text box → **Enter** to apply.
* **Severity:** `1..9` toggles corresponding severity buckets shown in the toolbar; `Shift+1..9` focuses a single bucket; `0` enables all.
* **Docker list:** `Ctrl+D` opens container list; inside list: `Space` toggle, `a` toggle All, `Enter`/`Esc` close.
* **Docker presets:** `p` opens presets manager (apply, save current, delete).
* **Theme:** `t` cycles theme.

## 4) CLI usage

```
# File mode
siftail /var/log/app.log

# Docker mode
siftail docker

# Streaming stdin
journalctl -f -u my.service | siftail

```

## 5) Severity/level system

* Detectors look for `level/lvl/severity` (JSON/logfmt) or common tokens like `INFO`, `WARN`, `ERROR`, etc.
* Default mapping: `1=DEBUG`, `2=INFO`, `3=WARN`, `4=ERROR`.
* As new levels appear (e.g., `TRACE`, `NOTICE`, `ALERT`, `CRITICAL`), they occupy slots `5..9` in order of first sight.
* If there are more than 9 distinct levels, remaining values are grouped into **9\:OTHER**.

## 6) Non-functional requirements

* **Responsiveness:** UI remains interactive under heavy input (e.g., thousands of lines/sec).
* **Stability:** no goroutine leaks; graceful shutdown on `SIGINT`.
* **Portability:** Linux/macOS primary; Windows best-effort (fsnotify). No root required (Docker socket permissions apply).
* **Resource bounds:** ring buffer size is configurable (default \~10k lines); long lines are soft‑wrapped to the viewport; any hard length cap is applied without adding ellipses.

## 7) Tooling & dependencies

* **Language:** Go ≥ 1.22
* **UI:** Bubble Tea, Bubbles, Lip Gloss
* **File watch:** fsnotify
* **Docker:** docker client SDK (`github.com/docker/docker`), `stdcopy` for demux
* **Lint/format:** `staticcheck` + `gofmt`
* **CI:** run `go test -race ./...`, lint, and build on every PR

## 8) Working agreements (house rules)

* **Keep this doc authoritative.** The AGENTS.md document must be updated regularly and kept in sync with the project.
* **Scoped AGENTS.md files.** Create multiple AGENTS.md files as needed—placing them in subdirectories is encouraged to keep scope isolated. These must also be updated regularly.
* **Engineering practices.** Follow best practices: keep things DRY, each unit should have a single responsibility and be reusable, and code must be readable. Use comments sparingly—only when they convey information that can’t be inferred from the code below them.
* **Testing policy.** Tests are required. No task is done without tests **passing**. Do not remove existing tests without explicit permission. Do not modify existing tests that are unrelated to your task.
* **Quality gate for commits.** Always format/lint before committing. Only commit if there are no lint, build, or test errors.

## 9) Definition of Done (per task/PR)

1. Feature/bugfix implemented according to spec and reflected here if behavior/UX changed.
2. Unit tests (and integration tests where appropriate) added/updated and passing locally with `-race`.
3. Lint/format clean; no TODOs;
4. User-visible behavior (hotkeys, prompts, options) documented and consistent with the toolbar/help.

## 10) Minimal repository layout (reference)

```
cmd/siftail/         # main entrypoint
internal/cli/        # flag parsing & mode dispatch
internal/tui/        # Bubble Tea model, view, styles
internal/core/       # domain types, ring buffer, matchers, severity
internal/input/      # stdin, file tail, docker readers, fan-in
internal/dockerx/    # docker client wrapper (interface + impl + fakes)
internal/persist/    # presets/config (XDG paths)
testdata/            # sample logs & rotation fixtures
```

## 11) Presets/config paths

* Linux/macOS (XDG): `~/.config/siftail/config.json`
* Windows: `%AppData%/siftail/config.json`
