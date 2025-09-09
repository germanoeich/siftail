## 0) Project scaffold & conventions

**Goal**
Create a clean repo layout, basic CLI, deterministic builds, and CI that runs tests and race detector.

**Deliverables**

* Module init: `go mod init <your.module>`
* Folder layout:

  ```
  cmd/siftail/main.go
  internal/cli/          // flags & command dispatch
  internal/tui/          // Bubble Tea models, view, keymap, styles
  internal/core/         // domain types, severity, filter/search, ring buffer
  internal/input/        // Input sources (stdin, file, docker), fan-in
  internal/dockerx/      // thin wrapper over Docker SDK (interface + impl + fakes)
  internal/persist/      // presets/config persistence (XDG paths)
  testdata/              // sample logs (plain, json, logfmt), rotation fixtures
  ```
* Makefile targets: `build`, `test`, `lint`, `race`, `bench`
* `staticcheck` config, `go test -race ./...` in CI (github actions)
* Minimal`README.md` with quick usage examples

**Success criteria**

* `go build ./cmd/siftail` succeeds
* `siftail -h` prints usage with the three modes examples
* CI runs unit tests and race detector on push

**Minimum tests**

* `TestMain_BuildsAndPrintsHelp` (exec `siftail -h`, assert usage contains “docker”, “stdin” behavior, “file”)

---

## 1) Domain model & severity system

**Goal**
Define the core types and **dynamic severity detection** with up to 9 togglable levels.

**Implement**

```go
package core

type SourceKind int
const (
  SourceStdin SourceKind = iota
  SourceFile
  SourceDocker
)

type Severity uint8
// Internal ordering; dynamic mapping maps strings -> index [1..9].
const (
  SevUnknown Severity = iota
  SevDebug
  SevInfo
  SevWarn
  SevError
  // additional can be learned dynamically; map to 5..9 as seen.
)

type LogEvent struct {
  Seq       uint64
  Time      time.Time
  Source    SourceKind
  Container string // docker only; empty otherwise
  Line      string // raw
  LevelStr  string // original parsed token, e.g. "warn", "TRACE"
  Level     Severity
}

type LevelMap struct {
  // Bi-directional maps, stable order for number keys.
  IndexToName []string           // positions 1..9 (0 unused here)
  NameToIndex map[string]int     // uppercased -> 1..9
  Enabled     map[int]bool       // current visibility by index (default true)
}

type SeverityDetector interface {
  Detect(line string) (levelStr string, level Severity, ok bool)
}
```

* Default level names/order: `1=DEBUG, 2=INFO, 3=WARN, 4=ERROR`; slots **5..9** reserved for new levels.
* New level discovery: on any line with unknown level token, **assign next free slot 5..9**. If more than 9 appear, overflow to slot 9 named `OTHER` (toggle controls all overflow).
* Parsing heuristics (try in order):

  1. **JSON**: try fast prefix `{` and `}`; if valid JSON, pick first found of keys (case‑insensitive): `level`, `lvl`, `severity`, `sev`, `log.level`, `priority`. Extract string or number mapped to names.
  2. **logfmt** (`key=value`): extract from same keys.
  3. **Bracketed/common**: regex `\b(DEBUG|INFO|WARN|WARNING|ERROR|TRACE|FATAL|CRITICAL)\b` (case‑insens.), also `[WARN]`, `<ERR>`, etc.
  4. Unknown → `OTHER` bucket.

**Success criteria**

* Unknown custom levels (e.g., “NOTICE”, “ALERT”) dynamically occupy slots 5..9 in order of first sight.
* Toggling numeric keys 1..9 filters correctly by mapped names.
* Works for JSON and plain text.

**Minimum tests**

* `TestSeverity_DefaultMapping`
* `TestSeverity_Detect_JSON_LevelField`
* `TestSeverity_Detect_Logfmt`
* `TestSeverity_Detect_Bracketed`
* `TestSeverity_DynamicLevels_AssignSlots`
* `TestSeverity_OverflowToOther`

---

## 2) Ring buffer & event store

**Goal**
Efficiently retain recent lines (e.g., default 10,000), support view windows, and filtering indices.

**Implement**

```go
type Ring struct {
  mu    sync.RWMutex
  cap   int
  buf   []LogEvent
  head  int      // next write pos
  size  int
  seq   uint64   // monotonically increasing
}

func NewRing(cap int) *Ring
func (r *Ring) Append(e LogEvent) LogEvent // assigns Seq & stores
func (r *Ring) Snapshot() []LogEvent       // stable copy (no pointers to internal slice)
func (r *Ring) GetBySeq(seq uint64) (LogEvent, bool)
```

* Maintain a **match index cache** per active filter/search/highlight (see next tasks) computed incrementally.

**Success criteria**

* Constant time append; memory capped \~O(capacity).
* Safe concurrent reads (view) with writes (sources).

**Minimum tests**

* `TestRing_AppendAndWrap`
* `TestRing_SnapshotConsistency`
* `TestRing_ConcurrentReaders` (with `-race`)

---

## 3) Matchers: filter‑in, filter‑out, highlight, find

**Goal**
Provide **fast, case‑insensitive substring** matching (optionally regex if prefixed `/.../`), and maintain incremental indices.

**Implement**

```go
type TextMatcher struct {
  raw      string   // user input
  isRegex  bool
  pattern  *regexp.Regexp
  lowered  string
}

func NewMatcher(s string) (TextMatcher, error)
func (m TextMatcher) Match(line string) bool

type Filters struct {
  Include []TextMatcher // OR over includes
  Exclude []TextMatcher // OR over excludes
  Highlights []TextMatcher // highlight only; does not affect visibility
}
```

* **Find** mode uses a single active matcher; maintain a **sorted \[]uint64** of matching `Seq` for navigation (prev/next).
* On new `LogEvent`, update indices for current matchers; keep them bounded by ring capacity.

**Success criteria**

* Filter‑in shows only lines matching any Include and not matching any Exclude.
* Highlight applies styles without auto‑scrolling.
* Find keeps selection position stable as new lines stream.

**Minimum tests**

* `TestMatcher_SubstringAndRegex`
* `TestFilters_IncludeExclude`
* `TestFindIndexing_NextPrevAcrossStream`

---

## 4) Input abstraction & fan‑in

**Goal**
Unify stdin, file tail, and docker sources behind a **non‑blocking** interface and a fan‑in multiplexer.

**Implement**

```go
package input

type Reader interface {
  Start(ctx context.Context) (<-chan core.LogEvent, <-chan error)
  // Start should return immediately; goroutine pumps events until ctx done.
}

type FanIn struct {
  readers []Reader
}
func (f *FanIn) Start(ctx context.Context) (<-chan core.LogEvent, <-chan error)
```

**Success criteria**

* Cancelling `ctx` cleanly stops all readers.
* No goroutine leaks (checked via test using `runtime.NumGoroutine` deltas or using a WaitGroup).

**Minimum tests**

* `TestFanIn_Multiplexes`
* `TestFanIn_CancelStopsAll`

---

## 5) STDIN reader

**Goal**
Stream from `os.Stdin` with arbitrarily long lines and minimal allocations.

**Implement**

* Use `bufio.Reader.ReadBytes('\n')` (not default `Scanner` limit).
* Trim trailing `\n`, keep `\r`.
* Stamp `Time: time.Now()` if no timestamp parsing.
* SourceKind = `SourceStdin`.

**Success criteria**

* Handles long lines (> 64KB).
* EOF stops gracefully (exit if stdin mode and not docker/file).

**Minimum tests**

* `TestStdinReader_LongLines`
* `TestStdinReader_StopsOnEOF`

---

## 6) File tailer

**Goal**
Tail a growing file, handle **truncate/rotation**, and minimal cross‑platform support.

**Implement**

* Open with `os.Open`, seek to end unless a `--from-start` flag is provided.
* Use `fsnotify` to watch `Write`, `Rename`, `Remove`:

  * On `Write`: read newly appended bytes.
  * On `Rename/Remove`: keep file handle if possible; also poll inode/size; if file recreated, reopen and continue.
  * On `Truncate` (size decreased), seek to 0 or to end (configurable; default follow from start of new content).
* Backoff on read errors.

**Success criteria**

* Detects writes appended to the same file.
* Recovers from log rotation (copytruncate & rename strategies).
* No duplication of lines across rotation.

**Minimum tests**

* `TestTailer_AppendsDetected`
* `TestTailer_CopyTruncateRotation`
* `TestTailer_RenameCreateRotation`

*(Use temp dir + helper that simulates writes/rotations.)*

---

## 7) Docker client wrapper

**Goal**
A small interface abstracting Docker SDK and enabling fakes in tests.

**Implement**

```go
package dockerx

type Client interface {
  ListContainers(ctx context.Context) ([]Container, error)
  StreamLogs(ctx context.Context, id string, since string) (io.ReadCloser, error)
  ContainerName(ctx context.Context, id string) (string, error) // convenience
}

type Container struct {
  ID    string
  Name  string // without leading '/'
  State string // running, etc
}
```

* Real impl uses `github.com/docker/docker/client` and `api/types`.
* Use `ContainerList` for running containers.
* For logs, use `ContainerLogs` with `Follow:true, ShowStdout:true, ShowStderr:true, Timestamps:true`.
  Decode the multiplexed stream using `stdcopy.StdCopy` into two pipes; tag lines as `stdout`/`stderr` if desired.

**Success criteria**

* Works when Docker daemon is reachable; returns clear error otherwise.
* Log lines from multiple containers interleave in arrival order.

**Minimum tests**

* `TestDockerClient_Fake_ListAndLogs` (using a fake implementation).
* `TestDockerLogReader_DemuxStdoutStderr`

---

## 8) Docker logs reader & toggle control

**Goal**
Stream logs from **all running containers**, allow **per‑container visibility toggles** and “All” toggles, plus **presets** later.

**Implement**

```go
type DockerReader struct {
  C dockerx.Client
  LevelDetect core.SeverityDetector
  // internal: map[id]readerState, selection set, etc.
}

type VisibleSet struct {
  mu sync.RWMutex
  On map[string]bool // containerID -> visible?
}
```

* On `Start`, enumerate running containers; start a goroutine per container to stream logs.
* Always read/parse, but **visibility is applied in the filter layer** (don’t stop underlying streams when toggled off to avoid backpressure).
* Provide messages to TUI model with container registry updates (`DockerContainersMsg` listing `{ID,Name,Visible}`), and a way to toggle visibility.

**Success criteria**

* Pressing `l` in TUI shows the list; toggling a container hides/shows its lines immediately.
* “All” toggles apply instantly.

**Minimum tests**

* `TestVisibleSet_ToggleAndAll`
* `TestDockerReader_StreamsAllContainers_Fake`

---

## 9) Search/highlight navigation state

**Goal**
Implement “h” **highlight** (no scrolling) and “f” **find** (highlight + arrow up/down jump) with a small prompt.

**Implement**

* State:

  ```go
  type SearchState struct {
    Active bool
    Matcher core.TextMatcher
    HitSeqs []uint64 // sorted
    Cursor  int // current index into HitSeqs
  }
  ```
* When active, arrow up/down moves Cursor; TUI scrolls to line with that `Seq`.
* Highlights: keep a separate array of `TextMatcher` used only for styling.

**Success criteria**

* “h” opens a focused textbox; Enter adds a highlight matcher; Esc cancels; no scroll on apply.
* “f” opens textbox; Enter activates search, jumps to first hit; arrow keys navigate; Esc exits search mode but keeps highlight (optional) — or clear on Esc (pick one; document).

**Minimum tests**

* `TestSearch_IndexesHitsIncrementally`
* `TestSearch_JumpPrevNext_Bounds`
* `TestHighlight_DoesNotAffectVisibility`

---

## 10) Severity toggling

**Goal**
Number keys 1..9 toggle severity visibility by current dynamic mapping.

**Implement**

* Model holds `LevelMap` from Task 1.
* Key handling: on numeric key, flip `Enabled[index]` and request view recompute.
* Status line shows `1:DEBUG [on] 2:INFO [off] …`.

**Success criteria**

* Lines hide/show immediately.
* New levels discovered appear in the toolbar mapping (up to slot 9; overflow bundles into “9\:OTHER”).

**Minimum tests**

* `TestSeverityToggle_FiltersView`
* `TestSeverity_DiscoveryUpdatesToolbar`

---

## 11) Filter‑in (Shift+F) & Filter‑out (Shift+U)

**Goal**
Maintain include and exclude matcher lists with prompts and apply to view.

**Implement**

* “Shift+F” opens textbox → on Enter, append to `Include`.
* “Shift+U” opens textbox → on Enter, append to `Exclude`.
* Provide small HUD listing active filters and a shortcut to clear them (e.g., `Ctrl+L` to clear all filters; not required by spec—document if added).

**Success criteria**

* Only lines satisfying include AND NOT exclude are shown.
* Piping or Docker continues to ingest while filtering is active.

**Minimum tests**

* `TestFilterIn_ApplyAndStack`
* `TestFilterOut_ApplyAndStack`
* `TestFilter_Composition_IncludeAndExclude`

---

## 12) TUI foundation (Bubble Tea model)

**Goal**
A responsive TUI with nano‑style toolbar, resizable viewport, text input overlay, and smooth streaming.

**Implement**

* Use `bubbles/viewport` for the log view, `bubbles/textinput` for prompts, `lipgloss` for styles.
* Model:

  ```go
  type Model struct {
    vp         viewport.Model
    input      textinput.Model
    inPrompt   bool
    promptKind enum{Find, Highlight, FilterIn, FilterOut, PresetName}
    ring       *core.Ring
    filters    core.Filters
    search     SearchState
    levels     core.LevelMap
    dockerUI   DockerUIState // container list, toggles
    mode       enum{ModeFile, ModeStdin, ModeDocker}
    followTail bool // auto-scroll when at bottom
    width, height int
    errMsg     string
  }
  ```
* Throttle UI refresh to \~30–60 FPS max (coalesce bursts) using a tick message or a small debouncer; apply only visible slice recomputation per frame.
* **Toolbar** at bottom with hotkeys and mapped severities.

**Success criteria**

* Window resizes handled (`tea.WindowSizeMsg`).
* When scrolled to bottom, new lines keep following. If scrolled up, no auto‑scroll until user hits `End` or a `g/G` pair (optional).

**Minimum tests**

* `TestModel_Update_ResizeAdjustsViewport`
* `TestModel_FollowTailBehavior`
* `TestModel_PromptFocusAndApply`

*(TUI tests call `Update` directly with messages, asserting model state.)*

---

## 13) View rendering & styling

**Goal**
Render lines efficiently with severity coloring and inline highlight spans.

**Implement**

* For visible lines in viewport range, compute once per frame:

  * Prefix with container name (Docker mode), timestamp (optional), and severity badge.
  * Apply highlight spans by **finding indices of matched substrings**; style substring ranges.
* Add a small top status line with current mode, number of lines visible, filters active.

**Success criteria**

* Smooth scroll without tearing; no full re-renders on every incoming line beyond needed.
* No panics on wide unicode; truncate long lines to viewport width (soft wrap optional).

**Minimum tests**

* `TestRender_SeverityBadges`
* `TestRender_HighlightSpansNoOverlap`
* `TestRender_ContainerPrefixing`

---

## 14) CLI wiring & modes

**Goal**
User‑facing commands exactly as you specified.

**Implement**

* **Invocation parsing** (std `flag` or `cobra`, your choice):

  * `siftail <path/to/file>` → ModeFile
  * `siftail docker` → ModeDocker
  * `tail -f file.txt | siftail` → ModeStdin (detect `!isatty(os.Stdin)`)
* Flags (minimal v1):

  * `--buffer-size=N` (default 10000)
  * `--from-start` (file mode)
  * `--no-color` (optional)
  * `--time-format` (optional)
* Detect misuse: both file and docker passed → show usage error.

**Success criteria**

* All three examples from your prompt work as described.
* Without args and with TTY stdin → show usage.

**Minimum tests**

* `TestCLI_FileMode_StartsTailer`
* `TestCLI_DockerMode_StartsDockerReader_Fake`
* `TestCLI_StdinMode_WhenPiped`

---

## 15) Docker TUI: container list & toggles & “All”

**Goal**
`l` opens a list of running containers with toggles; “All” entry toggles all; visible changes apply immediately.

**Implement**

* `dockerUI`:

  ```go
  type DockerUIState struct {
    ListOpen  bool
    Containers []dockerx.Container // cached
    Visible    map[string]bool     // id -> bool
    Cursor     int                 // list navigation
  }
  ```
* Keys inside list: `space` toggle, `a` toggle All, `Esc` close, `Enter` close.
* Optionally enable “show only selected” toggling via `S` (not required).

**Success criteria**

* Toggling a container hides/shows its lines in real time.
* Containers list refreshes when containers start/stop (poll or events; v1 can poll every N seconds).

**Minimum tests**

* `TestDockerUI_ToggleSingle`
* `TestDockerUI_ToggleAll`

---

## 16) Docker presets (visibility sets)

**Goal**
Create/apply named presets of container visibility.

**Implement**

* Persistence:

  ```go
  type Preset struct {
    Name string
    Visible map[string]bool // container name or ID (use Name for stability)
  }
  type PresetsFile struct {
    Presets []Preset
  }
  ```
* Location: XDG config dir `~/.config/siftail/presets.json` (Windows: `%AppData%/siftail/presets.json`).
* TUI:

  * Key `P` in docker mode opens presets manager: list, apply, save current as preset (prompt name), delete.
* Apply by mapping current containers by **Name** first, fallback to ID.

**Success criteria**

* Save preset from current toggles; later apply restores visibility mapping.
* Missing containers are ignored; extra containers remain as they were unless explicitly in preset.

**Minimum tests**

* `TestPresets_SaveAndLoad`
* `TestPresets_ApplyByName`
* `TestPresets_IgnoresMissingContainers`

---

## 17) Error handling & status

**Goal**
Display non-blocking errors (Docker unavailable, file missing, etc.) and recover when possible.

**Implement**

* Non-fatal errors set `errMsg` with timestamp, shown in status line for a few seconds.
* Docker unreachable: show error; allow retry key `R` (reconnect).
* File not found at launch: error & exit non-zero. If lost mid-run, attempt reopen for a grace period.

**Success criteria**

* No crashes on daemon loss or file rotation errors.
* Clear user feedback without blocking input.

**Minimum tests**

* `TestErrors_ShownInStatus`
* `TestDockerReconnect_FakeClient`

---

## 18) Performance & backpressure

**Goal**
Handle bursts without UI stall; keep CPU reasonable.

**Implement**

* **Frame coalescing**: accumulate events; repaint at most every \~33ms.
* **Max line length** cap (configurable): truncate and mark with “…”.
* **Ring size** configurable.
* Avoid per-line regex unless needed; prefer substring search.

**Success criteria**

* In a benchmark pushing 5k lines/s into a 10k ring, UI remains responsive (< 50ms per frame average on a dev laptop).
* No unbounded memory growth.

**Minimum tests**

* `BenchmarkAppend_AndFilter`
* `TestNoLeak_OnRapidStream` (monitor memory over time; heuristic)

---

## 19) The nano‑style toolbar & keymap

**Goal**
Always-visible toolbar showing hotkeys and current severity mapping.

**Implement**

* Bottom line sample:

  ```
  ^Q Quit  ^C Cancel  h Highlight  f Find  F Filter  U FilterOut  l Containers  P Presets
  1:DEBUG[on] 2:INFO[on] 3:WARN[on] 4:ERROR[on] 5:NOTICE[off] ... 9:OTHER[on]
  ```
* Update dynamically when new level assigned.

**Success criteria**

* Keys work as labeled.
* Mapping text reflects current toggles.

**Minimum tests**

* `TestToolbar_ShowsMappingAndStates`

---

## 20) Glue: visibility computation

**Goal**
Compute **visible lines** from ring given **filters**, **severity toggles**, and **docker visibility**.

**Implement**

```go
type VisiblePlan struct{
  Include core.Filters
  LevelMap *core.LevelMap
  DockerVisible map[string]bool // by name or id; empty means all
}

func ComputeVisible(r *core.Ring, plan VisiblePlan) []core.LogEvent
```

* Apply order:

  1. Severity enabled?
  2. Docker container visible? (only in docker mode)
  3. Include/Exclude match
* This function is pure and testable; TUI calls it on each refresh (with optimizations).

**Success criteria**

* Deterministic results for the same inputs.
* Fast for ring sizes up to N=100k (if configured).

**Minimum tests**

* `TestComputeVisible_OrderOfOps`
* `TestComputeVisible_DockerAndSeverity`

---

## 21) Packaging & help

**Goal**
Ship a single binary with clear `--help` and examples.

**Implement**

* `--help` shows:

  * `siftail /var/log/app.log`
  * `siftail docker`
  * `tail -f file | siftail`
* Exit codes: 0 on normal quit, non‑zero on fatal errors (e.g., file not found).

**Success criteria**

* `siftail --help` includes all hotkeys and modes summary.

**Minimum tests**

* `TestHelp_ContainsExamplesAndHotkeys`

---

## 22) Optional niceties (guarded flags; low risk)

*(Only if you want; not required by spec, keep isolated flags to avoid scope creep)*

* `--wrap` soft wraps long lines.
* `--time` show parsed timestamps (from JSON fields like `time`, `ts`) with `--time-format`.
* `--no-timestamps` to hide times.
* `--prefix=container|severity|time|none` to control columns.

Add separate tasks + tests only if enabled.

---

# Keybindings (default)

* **Global**: `q` or `Ctrl+C` quit; `Esc` cancels current prompt.
* **Highlight**: `h` → textbox → Enter to add highlight (no scroll).
* **Find**: `f` → textbox → Enter to activate; **Up/Down** jump prev/next hit.
* **Filter‑in**: `Shift+F` (capital F) → textbox → Enter to apply.
* **Filter‑out**: `Shift+U` (capital U) → textbox → Enter to apply.
* **Severity**: `1..9` toggle respective severity buckets.
* **Docker list**: `l` open; inside list: `Space` toggle container, `a` toggle all, `Enter/Esc` close.
* **Presets (Docker)**: `P` open manager (apply, save current, delete).
* **Follow tail**: when at bottom, auto-follow; if scrolled up, disable until end pressed (optional: `End`).

---

# Data parsing details

* **JSON extraction** keys (case-insensitive): `level`, `lvl`, `severity`, `sev`, `log.level`, `priority`. Value normalization:

  * Numeric → map `0/10/20/30/40/50` to `DEBUG/INFO/WARN/ERROR/FATAL` (syslog-esque).
  * String → trim `[]<>:`, to upper, map `WARNING`→`WARN`, `ERR`→`ERROR`.
* **logfmt**: parse a limited set—scan tokens split by spaces; token with `=` and non‑quoted simple values.
* **Common text**: regex word boundary match for typical level names.

---

# Non-functional requirements

* **Cross‑platform**: Linux/macOS; Windows “best effort” (fsnotify supported).
* **No root required** (Docker socket permissions as usual).
* **Resource bounds**: default ring 10k lines; max line length 512KB (configurable).

---

# Example test fixtures (place in `testdata/`)

* `plain.log`: mixed INFO/WARN/ERROR lines with bracketed levels.
* `json.log`: each line a JSON object with `time`, `level`, `msg`.
* `custom_levels.log`: includes `NOTICE`, `ALERT`, `CRITICAL`.
* Rotation scripts for copytruncate and rename styles.

---

# Pseudocode: Putting it together (program main)

```go
func main() {
  cfg := cli.Parse()
  m := tui.NewModel(cfg)

  ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
  defer cancel()

  var readers []input.Reader
  switch cfg.Mode {
  case ModeStdin:
    readers = append(readers, input.NewStdinReader(detector))
  case ModeFile:
    readers = append(readers, input.NewFileTailReader(cfg.Path, detector, cfg.FromStart))
  case ModeDocker:
    readers = append(readers, input.NewDockerReader(dockerClient, detector))
  }

  events, errs := input.NewFanIn(readers...).Start(ctx)
  p := tea.NewProgram(m, tea.WithAltScreen())
  go func() {
    for {
      select {
      case e := <-events: p.Send(tui.EventMsg{Event: e})
      case err := <-errs: p.Send(tui.ErrorMsg{Err: err})
      case <-ctx.Done(): return
      }
    }
  }()
  if _, err := p.Run(); err != nil { os.Exit(1) }
}
```

---

## Acceptance checklist for v1 (tie back to your spec)

* [ ] `siftail file.txt` opens TUI and tails file with rotation handling.
* [ ] `siftail docker` lists/streams all running containers; `l` toggles per container; `a` toggles All; presets can be saved/applied.
* [ ] `tail -f file.txt | siftail` opens streaming stdin mode.
* [ ] **Highlight (h)**: adds highlight without scrolling.
* [ ] **Find (f)**: highlights and navigates with **Up/Down** across matches.
* [ ] **Filter (Shift+F)** & **Filter out (Shift+U)**: include/exclude by text.
* [ ] **Severity filtering (1..9)**: default `1=DEBUG,2=INFO,3=WARN,4=ERROR`; new levels populate 5..9 dynamically; overflow bundles into 9\:OTHER.
* [ ] Simple nano‑style toolbar with hotkeys and severity map.
* [ ] Text input overlay appears, focuses correctly; **Enter** applies; **Esc** cancels.
* [ ] Basic performance verified; no leaks; graceful shutdown.

---

## Notes on risks & choices

* **Tail semantics** are tricky; tests cover copytruncate and rename rotations.
* **Docker multiplexing** uses `stdcopy.StdCopy`; ensure correct handling so partial frames don’t break lines.
* **Regex** is optional; keep off hot path (only when requested).
* **Dynamic levels** are capped at 9 to match key availability; overflow grouped into OTHER by design.

---