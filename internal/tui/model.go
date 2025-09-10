package tui

import (
	"sort"
	"time"

	"regexp"
	"strings"

	"github.com/atotto/clipboard"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	xansi "github.com/charmbracelet/x/ansi"
	"github.com/germanoeich/siftail/internal/core"
	"github.com/germanoeich/siftail/internal/persist"
	"github.com/mattn/go-runewidth"
	"github.com/muesli/termenv"
)

// Mode represents the operational mode of the application
type Mode int

const (
	ModeFile Mode = iota
	ModeStdin
	ModeDocker
)

// PromptKind represents the type of text input prompt currently active
type PromptKind int

const (
	PromptFind PromptKind = iota
	PromptHighlight
	PromptFilterIn
	PromptFilterOut
	PromptPresetName
)

// DockerUIState manages Docker-specific UI state
type DockerUIState struct {
	ContainerListOpen bool
	PresetManagerOpen bool
	Containers        map[string]bool // container id/name -> visible
	AllToggle         bool
	SelectedContainer int              // index in sorted container list for navigation
	Presets           []persist.Preset // loaded presets for UI
	SelectedPreset    int              // index in presets list for navigation
}

// PerformanceConfig holds performance-related configuration
type PerformanceConfig struct {
	MaxLineLength  int           // maximum line length before truncation (default: 2048)
	RenderThrottle time.Duration // minimum time between renders (default: 33ms for ~30fps)
}

// Model represents the main TUI state and manages all UI components
type Model struct {
	// Core UI components
	vp    viewport.Model
	input textinput.Model

	// Prompt state
	inPrompt   bool
	promptKind PromptKind

	// Data and filters
	ring    *core.Ring
	filters *core.Filters
	search  *core.SearchState
	levels  *core.LevelMap

	// Docker UI state
	dockerUI DockerUIState
	presets  *persist.PresetsManager

	// Clear menu state
	clearMenuOpen bool
	clearMenuSel  int // 0..N-1

	// Performance configuration
	perf PerformanceConfig

	// App state
	mode       Mode
	followTail bool // auto-scroll when at bottom
	width      int
	height     int
	errMsg     string
	errTime    time.Time // timestamp of the error for auto-clearing

	// Throttling for smooth updates
	lastRender time.Time
	dirty      bool // needs re-render

	// Sequence -> current line index mapping
	seqIndex map[uint64]int

	// Cached content lines (styled and plain) currently set in the viewport
	contentLines      []string // includes ANSI styling
	contentPlainLines []string // ANSI stripped for selection/copy

	// Theme
	theme    *Theme
	themeIdx int

	// Selection-friendly mode (mouse disabled, alt screen off)
	selectionMode bool

	// Mouse selection state within the viewport
	selecting bool
	selStartX int // column within viewport
	selStartY int // row within viewport
	selEndX   int
	selEndY   int

	// Help overlay
	helpOpen bool
}

// NewModel creates a new TUI model with default configuration
func NewModel(ring *core.Ring, filters *core.Filters, search *core.SearchState, levels *core.LevelMap, mode Mode) *Model {
	// Initialize viewport
	vp := viewport.New(80, 24)
	vp.SetContent("")

	// Initialize text input
	input := textinput.New()
	input.Placeholder = "Enter search term..."
	input.CharLimit = 256

	// Initialize presets manager (ignore error for now)
	presetsManager, _ := persist.NewPresetsManager()

	return &Model{
		vp:         vp,
		input:      input,
		ring:       ring,
		filters:    filters,
		search:     search,
		levels:     levels,
		mode:       mode,
		followTail: true,
		dockerUI: DockerUIState{
			Containers: make(map[string]bool),
			AllToggle:  true,
		},
		presets: presetsManager,
		perf: PerformanceConfig{
			MaxLineLength:  2048,                  // 2KB per line max
			RenderThrottle: 33 * time.Millisecond, // ~30 FPS
		},
		width:    80,
		height:   24,
		seqIndex: make(map[uint64]int),
		theme:    DarkTheme(),
		themeIdx: 0,
	}
}

// Init initializes the model for the Bubble Tea runtime
func (m Model) Init() tea.Cmd {
	return tea.Batch(
		textinput.Blink,
		tickCmd(), // Start render throttling ticker
	)
}

// Update handles incoming messages and updates the model state
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m = m.handleResize()

	case tea.MouseMsg:
		// Custom selection + copy handler (left drag, copy on release)
		if !m.helpOpen && !m.dockerUI.ContainerListOpen && !m.dockerUI.PresetManagerOpen && !m.clearMenuOpen {
			vpTopY := 1
			vpBottomY := vpTopY + m.vp.Height - 1
			if msg.Button == tea.MouseButtonLeft {
				switch msg.Action {
				case tea.MouseActionPress:
					if msg.Y >= vpTopY && msg.Y <= vpBottomY {
						m.selecting = true
						m.selStartX = clamp(msg.X, 0, m.vp.Width-1)
						m.selStartY = clamp(msg.Y-vpTopY, 0, m.vp.Height-1)
						m.selEndX, m.selEndY = m.selStartX, m.selStartY
						m.dirty = true
					}
				case tea.MouseActionMotion:
					if m.selecting {
						if msg.Y < vpTopY {
							m.vp.ScrollUp(1)
						} else if msg.Y > vpBottomY {
							m.vp.ScrollDown(1)
						}
						m.selEndX = clamp(msg.X, 0, m.vp.Width-1)
						m.selEndY = clamp(msg.Y-vpTopY, 0, m.vp.Height-1)
						m.dirty = true
					}
				case tea.MouseActionRelease:
					if m.selecting {
						m.selEndX = clamp(msg.X, 0, m.vp.Width-1)
						m.selEndY = clamp(msg.Y-vpTopY, 0, m.vp.Height-1)
						if len(m.contentPlainLines) > 0 {
							if text := m.extractSelectedText(); strings.TrimSpace(text) != "" {
								// Copy via OSC52 (termenv) and native clipboard
								cmds = append(cmds,
									func() tea.Msg { termenv.Copy(text); return nil },
									func() tea.Msg { _ = clipboard.WriteAll(text); return nil },
								)
								m = m.setError("Copied selection to clipboard")
							}
						}
						m.selecting = false
						m.dirty = true
					}
				}
			}
		}

		// Forward mouse events (wheel scroll) to the viewport
		var cmd tea.Cmd
		m.vp, cmd = m.vp.Update(msg)
		cmds = append(cmds, cmd)
		m = m.updateFollowTail()

	case tea.KeyMsg:
		// Key handling branches below
		if m.inPrompt {
			// Handle prompt-specific keys
			switch msg.String() {
			case "enter":
				m = m.handlePromptSubmit()
			case "esc":
				m = m.cancelPrompt()
			default:
				// Pass other keys to text input
				var cmd tea.Cmd
				m.input, cmd = m.input.Update(msg)
				cmds = append(cmds, cmd)
			}
		} else if m.dockerUI.ContainerListOpen {
			// Handle Docker container list navigation
			switch msg.String() {
			case "ctrl+q", "ctrl+c":
				return m, tea.Quit
			case "esc", "enter", "q":
				m.dockerUI.ContainerListOpen = false
			case "up":
				m = m.navigateContainerList(true) // up
			case "down":
				m = m.navigateContainerList(false) // down
			case " ":
				m = m.toggleSelectedContainer()
			case "a":
				m = m.toggleAllContainers()
			}
		} else if m.dockerUI.PresetManagerOpen {
			// Handle Docker preset manager navigation
			switch msg.String() {
			case "ctrl+q", "ctrl+c":
				return m, tea.Quit
			case "esc", "q":
				m.dockerUI.PresetManagerOpen = false
			case "up":
				m = m.navigatePresetsList(true) // up
			case "down":
				m = m.navigatePresetsList(false) // down
			case "enter":
				m = m.applySelectedPreset()
			case "s":
				m = m.startPrompt(PromptPresetName, "Save preset as: ")
			case "d", "x":
				m = m.deleteSelectedPreset()
			case "r":
				m = m.refreshPresetsList()
			}
		} else if m.helpOpen {
			// Help overlay interactions
			switch msg.String() {
			case "q", "esc", "?", "enter", "f1":
				m.helpOpen = false
			}
		} else if m.clearMenuOpen {
			// Clear menu navigation and actions
			switch msg.String() {
			case "ctrl+q", "ctrl+c":
				return m, tea.Quit
			case "esc", "c", "q":
				m.clearMenuOpen = false
			case "up":
				if m.clearMenuSel > 0 {
					m.clearMenuSel--
				} else {
					m.clearMenuSel = 3
				}
			case "down":
				if m.clearMenuSel < 3 {
					m.clearMenuSel++
				} else {
					m.clearMenuSel = 0
				}
			case "enter":
				m = m.invokeClearMenuSelection()
			case "h":
				m.filters.ClearHighlights()
				m.clearMenuOpen = false
				m.dirty = true
			case "i":
				m.filters.ClearIncludes()
				m.clearMenuOpen = false
				m.dirty = true
			case "u":
				m.filters.ClearExcludes()
				m.clearMenuOpen = false
				m.dirty = true
			case "a":
				m = m.clearAllFilters()
				m.clearMenuOpen = false
			}
		} else {
			// Handle main app keys
			switch msg.String() {
			case "ctrl+q", "ctrl+c":
				return m, tea.Quit

			// Search and filters
			case "h":
				m = m.startPrompt(PromptHighlight, "Highlight: ")
			case "ctrl+f":
				m = m.startPrompt(PromptFind, "Find: ")
			case "I":
				m = m.startPrompt(PromptFilterIn, "Filter In: ")
			case "O":
				m = m.startPrompt(PromptFilterOut, "Filter Out: ")
			case "0":
				m.levels.EnableAll()
				m.dirty = true
			case "home":
				m.vp.GotoTop()
				m.followTail = false
			case "end":
				m.vp.GotoBottom()
				m.followTail = true
			case "esc":
				if m.search.IsActive() {
					m.search.Clear()
					m.search.SetActive(false)
					m.dirty = true
					break
				}
			case "c":
				m.clearMenuOpen = true
				m.clearMenuSel = 0
			case "C":
				m = m.clearAllFilters()
			case "?", "f1":
				m.helpOpen = true

				// Find navigation (only when find is active)
			case "up":
				if m.search.IsActive() {
					m = m.navigateFind(true) // previous
				}
			case "down":
				if m.search.IsActive() {
					m = m.navigateFind(false) // next
				}

			// Severity level toggles
			case "1", "2", "3", "4", "5", "6", "7", "8", "9":
				level := int(msg.String()[0] - '0')
				m.levels.Toggle(level)
				m.dirty = true
				// Severity focus with Shift+Number
			case "!", "@", "#", "$", "%", "^", "&", "*", "(":
				// Map shifted symbols to indices 1..9
				sym := msg.String()
				idx := 0
				switch sym {
				case "!":
					idx = 1
				case "@":
					idx = 2
				case "#":
					idx = 3
				case "$":
					idx = 4
				case "%":
					idx = 5
				case "^":
					idx = 6
				case "&":
					idx = 7
				case "*":
					idx = 8
				case "(":
					idx = 9
				}
				if idx > 0 {
					// If this idx is already the only enabled level, enable all.
					_, enabled := m.levels.GetSnapshot()
					cnt := 0
					for i := 1; i <= 9; i++ {
						if enabled[i] {
							cnt++
						}
					}
					if cnt == 1 && enabled[idx] {
						m.levels.EnableAll()
					} else {
						m.levels.Focus(idx)
					}
					m.dirty = true
				}

			// Docker mode keys
			case "ctrl+d":
				if m.mode == ModeDocker {
					m.dockerUI.ContainerListOpen = !m.dockerUI.ContainerListOpen
					m.dockerUI.SelectedContainer = -1 // Reset selection to "All"
				}
			case "p":
				if m.mode == ModeDocker {
					m.dockerUI.PresetManagerOpen = true
					m.dockerUI.SelectedPreset = 0
					m = m.refreshPresetsList()
				}
			case "t":
				// Cycle theme
				m.themeIdx = (m.themeIdx + 1) % len(themes)
				m.theme = themes[m.themeIdx]
				m.dirty = true
			case "ctrl+s":
				if m.selectionMode {
					// Return to interactive mode: re-enter alt screen and re-enable mouse
					m.selectionMode = false
					m = m.setError("Selection mode off")
					cmds = append(cmds, tea.EnterAltScreen, tea.EnableMouseCellMotion)
				} else {
					// Enable selection: disable mouse + exit alt screen
					m.selectionMode = true
					m = m.setError("Selection mode: select text; press Ctrl+S to return")
					cmds = append(cmds, tea.DisableMouse, tea.ExitAltScreen)
				}

				// Viewport navigation
			default:
				var cmd tea.Cmd
				m.vp, cmd = m.vp.Update(msg)
				cmds = append(cmds, cmd)
				m = m.updateFollowTail()
			}
		}

	case tickMsg:
		// Throttled render update
		m = m.handleTick()
		cmds = append(cmds, tickCmd())

	case refreshMsg:
		// Force refresh of visible content
		m = m.refreshContent()

	case LogAppendedMsg:
		// When find is active, add new hits incrementally
		if m.search.IsActive() {
			matcher := m.search.GetMatcher()
			if matcher.Match(msg.Event.Line) {
				m.search.AddHit(msg.Event.Seq)
				// If we’re following tail and this is a new match, optionally auto-jump?
				// Keeping behavior passive; user navigates with up/down.
			}
		}

	case DockerContainersMsg:
		// Update container list from Docker reader
		m = m.updateDockerContainers(msg.Containers)

	case DockerErrorMsg:
		// Handle Docker connection errors
		if msg.Error == nil {
			// Success - clear error
			m = m.setError("Docker reconnected successfully")
		} else if msg.Recoverable {
			m = m.setError("Docker unavailable: " + msg.Error.Error())
		} else {
			m = m.setError("Docker error: " + msg.Error.Error())
		}
	}

	return m, tea.Batch(cmds...)
}

// SetTheme applies the theme by name; falls back to dark.
func (m *Model) SetTheme(name string) {
	m.theme = themeByName(name)
	// sync index for cycling
	for i, t := range themes {
		if t.Name == m.theme.Name {
			m.themeIdx = i
			break
		}
	}
	m.dirty = true
}

// handleResize adjusts viewport and other components to new terminal size
func (m Model) handleResize() Model {
	// Reserve space for status line (1) and toolbar (2)
	viewportHeight := m.height - 3
	if viewportHeight < 5 {
		viewportHeight = 5
	}

	m.vp.Width = m.width
	m.vp.Height = viewportHeight

	// Adjust text input width
	m.input.Width = m.width - 20 // leave space for prompt label

	m.dirty = true
	return m
}

// startPrompt initiates a text input prompt
func (m Model) startPrompt(kind PromptKind, placeholder string) Model {
	m.inPrompt = true
	m.promptKind = kind
	m.input.Placeholder = placeholder
	m.input.SetValue("")
	m.input.Focus()
	return m
}

// cancelPrompt cancels the current prompt and returns to normal mode
func (m Model) cancelPrompt() Model {
	m.inPrompt = false
	m.input.Blur()
	return m
}

// handlePromptSubmit processes the entered text based on prompt type
func (m Model) handlePromptSubmit() Model {
	text := m.input.Value()
	m = m.cancelPrompt()

	if text == "" {
		return m
	}

	matcher, err := core.NewMatcher(text)
	if err != nil {
		return m.setError("Invalid pattern: " + err.Error())
	}

	switch m.promptKind {
	case PromptHighlight:
		m.filters.AddHighlight(matcher)
	case PromptFind:
		m.search.SetMatcher(matcher)
		m.search.SetActive(true)
		m = m.refreshFindIndex()
		// Jump to first match if found
		if seq := m.search.JumpToFirst(); seq != 0 {
			m = m.scrollToSequence(seq)
		}
	case PromptFilterIn:
		m.filters.AddInclude(matcher)
	case PromptFilterOut:
		m.filters.AddExclude(matcher)
	case PromptPresetName:
		// Save current container visibility as a preset
		if m.mode == ModeDocker && m.presets != nil {
			preset := persist.CreatePresetFromCurrent(text, m.dockerUI.Containers)
			if err := m.presets.SavePreset(preset); err != nil {
				return m.setError("Failed to save preset: " + err.Error())
			} else {
				m = m.setError("Preset '" + text + "' saved successfully")
				m = m.refreshPresetsList() // Refresh the presets list
			}
		} else {
			return m.setError("Presets are only available in Docker mode")
		}
		// Don't use matcher for preset names, so exit early
		m.dirty = true
		return m
	}

	m.errMsg = ""
	m.dirty = true
	return m
}

// navigateFind moves to the next or previous find match
func (m Model) navigateFind(isPrev bool) Model {
	if !m.search.IsActive() {
		return m
	}

	var seq uint64
	if isPrev {
		seq = m.search.Prev()
	} else {
		seq = m.search.Next()
	}

	if seq != 0 {
		m = m.scrollToSequence(seq)
	}

	return m
}

// scrollToSequence scrolls the viewport to show the event with the given sequence number
func (m Model) scrollToSequence(seq uint64) Model {
	// Look up line index for sequence; rebuild mapping if necessary
	idx, ok := m.seqIndex[seq]
	if !ok {
		plan := core.VisiblePlan{Include: m.filters, LevelMap: m.levels, DockerVisible: m.dockerUI.Containers}
		events := core.ComputeVisible(m.ring.Snapshot(), plan)
		m.seqIndex = make(map[uint64]int, len(events))
		for i, e := range events {
			m.seqIndex[e.Seq] = i
		}
		idx, ok = m.seqIndex[seq]
		if !ok {
			return m
		}
	}

	// Scroll so the target line is near the middle, within bounds
	target := idx - m.vp.Height/2
	if target < 0 {
		target = 0
	}
	m.vp.SetYOffset(target)
	m.followTail = false
	m.dirty = true
	return m
}

// updateFollowTail determines if we should follow new log entries
func (m Model) updateFollowTail() Model {
	// If viewport is scrolled to the bottom, enable follow tail
	m.followTail = m.vp.AtBottom()
	return m
}

// refreshFindIndex rebuilds the find index based on current ring contents
func (m Model) refreshFindIndex() Model {
	m.search.Clear()
	matcher := m.search.GetMatcher()

	// Scan all events in ring and add matches to search index
	events := m.ring.Snapshot()
	for _, event := range events {
		if matcher.Match(event.Line) {
			m.search.AddHit(event.Seq)
		}
	}

	return m
}

// refreshContent forces a refresh of the viewport content
func (m Model) refreshContent() Model {
	m.dirty = true
	return m
}

// handleTick processes throttled render updates
func (m Model) handleTick() Model {
	now := time.Now()

	// Auto-clear expired errors
	if m.isErrorExpired() {
		m = m.clearError()
	}

	// Throttle rendering based on configuration
	if m.dirty && now.Sub(m.lastRender) > m.perf.RenderThrottle {
		m = m.updateViewportContent()
		m.lastRender = now
		m.dirty = false
	}

	return m
}

// updateViewportContent refreshes the viewport with current log data
func (m Model) updateViewportContent() Model {
	// Get visible events based on filters and docker visibility
	plan := core.VisiblePlan{
		Include:       m.filters,
		LevelMap:      m.levels,
		DockerVisible: m.dockerUI.Containers,
	}

	events := m.ring.Snapshot()
	visibleEvents := core.ComputeVisible(events, plan)
	// Build index for quick scrolling to sequences
	m.seqIndex = make(map[uint64]int, len(visibleEvents))
	for i, e := range visibleEvents {
		m.seqIndex[e.Seq] = i
	}

	// Render events to viewport content
	content := m.renderEventsWithFullStyling(visibleEvents)
	// Split into lines for possible selection overlay and caching
	lines := strings.Split(strings.ReplaceAll(content, "\r\n", "\n"), "\n")

	// Apply selection overlay if actively selecting
	if m.selecting {
		lines = m.applySelectionHighlight(lines)
	}

	// Set viewport content from possibly highlighted lines
	content = strings.Join(lines, "\n")
	m.vp.SetContent(content)

	// Cache content as lines (styled + plain) for selection/copy
	m.contentLines = lines
	m.contentPlainLines = make([]string, len(m.contentLines))
	for i := range m.contentLines {
		m.contentPlainLines[i] = stripANSI(m.contentLines[i])
	}

	// Auto-scroll if following tail
	if m.followTail {
		m.vp.GotoBottom()
	}

	return m
}

// renderBasicEvents converts log events to basic styled viewport content
func (m Model) renderBasicEvents(events []core.LogEvent) string {
	// Deprecated basic renderer kept for reference; full styling now used.
	return m.renderEventsWithFullStyling(events)
}

// renderEvent formats a single log event with styling
func (m Model) renderEvent(event core.LogEvent) string {
	// Use full styling path
	return m.renderEventWithFullStyling(event)
}

// Message types for internal communication
type tickMsg time.Time
type refreshMsg struct{}

// LogAppendedMsg notifies the model that a new log event was appended.
// Used to incrementally update find hits when active.
type LogAppendedMsg struct {
	Event core.LogEvent
}

// DockerContainersMsg updates the list of available containers
type DockerContainersMsg struct {
	Containers map[string]bool // container name -> initially visible
}

// DockerErrorMsg indicates Docker connection issues
type DockerErrorMsg struct {
	Error       error
	Recoverable bool // true if user can attempt reconnection
}

// tickCmd returns a command that sends tick messages for render throttling
func tickCmd() tea.Cmd {
	return tea.Tick(16*time.Millisecond, func(t time.Time) tea.Msg {
		return tickMsg(t)
	})
}

// RefreshCmd returns a command that forces a content refresh
func RefreshCmd() tea.Cmd {
	return func() tea.Msg {
		return refreshMsg{}
	}
}

// applySelectionHighlight overlays a visual highlight over the current
// selection on top of styled content lines, preserving ANSI sequences.
func (m Model) applySelectionHighlight(lines []string) []string {
	if len(lines) == 0 || m.vp.Height <= 0 || m.vp.Width <= 0 {
		return lines
	}
	// Normalize selection box
	startY := minInt(m.selStartY, m.selEndY)
	endY := maxInt(m.selStartY, m.selEndY)
	startX := minInt(m.selStartX, m.selEndX)
	endX := maxInt(m.selStartX, m.selEndX)
	if startY == endY && startX == endX {
		return lines
	}
	absStart := clamp(m.vp.YOffset+startY, 0, len(lines)-1)
	absEnd := clamp(m.vp.YOffset+endY, 0, len(lines)-1)

	out := make([]string, len(lines))
	copy(out, lines)

	for i := absStart; i <= absEnd; i++ {
		line := lines[i]
		// Determine selection columns for this line
		sx := 0
		ex := xansi.StringWidth(line)
		if i == absStart {
			sx = startX
		}
		if i == absEnd {
			ex = endX
		}
		if ex < sx {
			sx, ex = ex, sx
		}
		// Clamp to visible viewport width and line width
		lw := xansi.StringWidth(line)
		sx = clamp(sx, 0, minInt(m.vp.Width, lw))
		ex = clamp(ex, 0, minInt(m.vp.Width, lw))
		if sx == ex {
			continue
		}

		left := xansi.Cut(line, 0, sx)
		mid := xansi.Cut(line, sx, ex)
		right := xansi.Cut(line, ex, lw)
		// Use sticky inverse to ensure highlight survives inner SGR resets
		out[i] = left + stickyInverse(mid) + right
	}
	return out
}

// extractSelectedText builds the selected plain text based on the selection
// box (viewport coordinates) and current viewport offset.
func (m Model) extractSelectedText() string {
	if len(m.contentPlainLines) == 0 || m.vp.Height <= 0 || m.vp.Width <= 0 {
		return ""
	}
	startY := minInt(m.selStartY, m.selEndY)
	endY := maxInt(m.selStartY, m.selEndY)
	startX := minInt(m.selStartX, m.selEndX)
	endX := maxInt(m.selStartX, m.selEndX)
	if startY == endY && startX == endX {
		return ""
	}
	absStart := clamp(m.vp.YOffset+startY, 0, len(m.contentPlainLines)-1)
	absEnd := clamp(m.vp.YOffset+endY, 0, len(m.contentPlainLines)-1)
	var out []string
	for i := absStart; i <= absEnd; i++ {
		line := m.contentPlainLines[i]
		sx := 0
		ex := ansiStringWidth(line)
		if i == absStart {
			sx = startX
		}
		if i == absEnd {
			ex = endX
		}
		if ex < sx {
			sx, ex = ex, sx
		}
		sx = clamp(sx, 0, m.vp.Width)
		ex = clamp(ex, 0, m.vp.Width)
		if sx == ex {
			if absStart != absEnd {
				out = append(out, "")
			}
			continue
		}
		out = append(out, sliceByColumns(line, sx, ex))
	}
	return strings.Join(out, "\n")
}

// sliceByColumns slices s by screen columns [start, end) with rune width.
func sliceByColumns(s string, start, end int) string {
	if end <= start {
		return ""
	}
	var b strings.Builder
	col := 0
	for _, r := range s {
		w := runewidth.RuneWidth(r)
		next := col + w
		if next > start && col < end {
			b.WriteRune(r)
		}
		if next >= end {
			break
		}
		col = next
	}
	return b.String()
}

var ansiRegexp = regexp.MustCompile("\x1b\x5b[0-9;]*[ -/]*[@-~]")

func stripANSI(s string) string    { return ansiRegexp.ReplaceAllString(s, "") }
func ansiStringWidth(s string) int { return runewidth.StringWidth(s) }

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// clamp returns v clamped to [low, high].
func clamp(v, low, high int) int {
	if high < low {
		low, high = high, low
	}
	if v < low {
		return low
	}
	if v > high {
		return high
	}
	return v
}

// stickyInverse wraps s with reverse-video SGR and re-applies it after any
// inner SGR sequence so that the visual selection remains visible across
// styled segments.
const esc = "\x1b"

var sgrSeq = regexp.MustCompile(esc + `\[[0-9;]*m`)

func stickyInverse(s string) string {
	const on = "\x1b[7m"   // reverse video on
	const off = "\x1b[27m" // reverse video off
	if s == "" {
		return s
	}
	with := sgrSeq.ReplaceAllStringFunc(s, func(seq string) string { return seq + on })
	return on + with + off
}

// navigateContainerList moves the selection cursor in the container list
func (m Model) navigateContainerList(up bool) Model {
	containerCount := len(m.dockerUI.Containers)

	if up {
		if m.dockerUI.SelectedContainer > -1 {
			m.dockerUI.SelectedContainer--
		} else {
			// Wrap to last container when going up from "All"
			m.dockerUI.SelectedContainer = containerCount - 1
		}
	} else {
		if m.dockerUI.SelectedContainer < containerCount-1 {
			m.dockerUI.SelectedContainer++
		} else {
			// Wrap to "All" when going down from last container
			m.dockerUI.SelectedContainer = -1
		}
	}

	return m
}

// toggleSelectedContainer toggles visibility of the currently selected container
func (m Model) toggleSelectedContainer() Model {
	if m.dockerUI.SelectedContainer == -1 {
		// Toggle All
		return m.toggleAllContainers()
	}

	// Get sorted container list to find the selected container
	var containers []string
	for name := range m.dockerUI.Containers {
		containers = append(containers, name)
	}
	sort.Strings(containers)

	if m.dockerUI.SelectedContainer >= 0 && m.dockerUI.SelectedContainer < len(containers) {
		selectedContainer := containers[m.dockerUI.SelectedContainer]
		m.dockerUI.Containers[selectedContainer] = !m.dockerUI.Containers[selectedContainer]
		m.dirty = true
	}

	return m
}

// toggleAllContainers toggles visibility of all containers at once
func (m Model) toggleAllContainers() Model {
	m.dockerUI.AllToggle = !m.dockerUI.AllToggle

	// Apply the all toggle to all containers
	for name := range m.dockerUI.Containers {
		m.dockerUI.Containers[name] = m.dockerUI.AllToggle
	}

	m.dirty = true
	return m
}

// updateDockerContainers updates the container list with new containers
func (m Model) updateDockerContainers(containers map[string]bool) Model {
	// Preserve existing visibility settings for containers that still exist
	newContainers := make(map[string]bool)

	for name, defaultVisible := range containers {
		if existing, exists := m.dockerUI.Containers[name]; exists {
			// Keep existing setting
			newContainers[name] = existing
		} else {
			// New container, use default visibility
			newContainers[name] = defaultVisible
		}
	}

	m.dockerUI.Containers = newContainers
	m.dirty = true

	return m
}

// refreshPresetsList loads the current presets from disk into the UI
func (m Model) refreshPresetsList() Model {
	if m.presets == nil {
		m.errMsg = "Presets manager not available"
		return m
	}

	presets, err := m.presets.LoadPresets()
	if err != nil {
		m.errMsg = "Failed to load presets: " + err.Error()
		m.dockerUI.Presets = nil
	} else {
		m.dockerUI.Presets = presets
		// Reset selection if it's out of bounds
		if m.dockerUI.SelectedPreset >= len(presets) {
			m.dockerUI.SelectedPreset = len(presets) - 1
		}
		if m.dockerUI.SelectedPreset < 0 && len(presets) > 0 {
			m.dockerUI.SelectedPreset = 0
		}
	}

	m.dirty = true
	return m
}

// navigatePresetsList moves the selection cursor in the presets list
func (m Model) navigatePresetsList(up bool) Model {
	presetCount := len(m.dockerUI.Presets)
	if presetCount == 0 {
		return m
	}

	if up {
		if m.dockerUI.SelectedPreset > 0 {
			m.dockerUI.SelectedPreset--
		} else {
			// Wrap to last preset
			m.dockerUI.SelectedPreset = presetCount - 1
		}
	} else {
		if m.dockerUI.SelectedPreset < presetCount-1 {
			m.dockerUI.SelectedPreset++
		} else {
			// Wrap to first preset
			m.dockerUI.SelectedPreset = 0
		}
	}

	return m
}

// applySelectedPreset applies the currently selected preset to container visibility
func (m Model) applySelectedPreset() Model {
	if len(m.dockerUI.Presets) == 0 || m.dockerUI.SelectedPreset < 0 || m.dockerUI.SelectedPreset >= len(m.dockerUI.Presets) {
		m.errMsg = "No preset selected"
		return m
	}

	selectedPreset := m.dockerUI.Presets[m.dockerUI.SelectedPreset]
	m.dockerUI.Containers = persist.ApplyPreset(selectedPreset, m.dockerUI.Containers)

	m.errMsg = "Applied preset '" + selectedPreset.Name + "'"
	m.dockerUI.PresetManagerOpen = false
	m.dirty = true

	return m
}

// deleteSelectedPreset removes the currently selected preset from disk
func (m Model) deleteSelectedPreset() Model {
	if len(m.dockerUI.Presets) == 0 || m.dockerUI.SelectedPreset < 0 || m.dockerUI.SelectedPreset >= len(m.dockerUI.Presets) {
		m.errMsg = "No preset selected"
		return m
	}

	if m.presets == nil {
		m.errMsg = "Presets manager not available"
		return m
	}

	selectedPreset := m.dockerUI.Presets[m.dockerUI.SelectedPreset]
	if err := m.presets.DeletePreset(selectedPreset.Name); err != nil {
		m.errMsg = "Failed to delete preset: " + err.Error()
	} else {
		m.errMsg = "Deleted preset '" + selectedPreset.Name + "'"
		m = m.refreshPresetsList()
	}

	return m
}

// setError sets an error message with timestamp for auto-clearing
func (m Model) setError(msg string) Model {
	m.errMsg = msg
	m.errTime = time.Now()
	m.dirty = true
	return m
}

// clearError clears the error message
func (m Model) clearError() Model {
	m.errMsg = ""
	m.errTime = time.Time{}
	m.dirty = true
	return m
}

// isErrorExpired returns true if the error should be auto-cleared
func (m Model) isErrorExpired() bool {
	if m.errMsg == "" {
		return false
	}
	// Clear error after 5 seconds
	return time.Since(m.errTime) > 5*time.Second
}

// clearAllFilters clears include, exclude, and highlight filters without
// touching Docker visibility state.
func (m Model) clearAllFilters() Model {
	m.filters.ClearIncludes()
	m.filters.ClearExcludes()
	m.filters.ClearHighlights()
	m.dirty = true
	m.errMsg = "Cleared filters & highlights"
	m.errTime = time.Now()
	return m
}

// invokeClearMenuSelection performs the action for the current clear menu item.
// 0: Clear Highlights, 1: Clear Includes, 2: Clear Excludes, 3: Clear All
func (m Model) invokeClearMenuSelection() Model {
	switch m.clearMenuSel {
	case 0:
		m.filters.ClearHighlights()
		m.errMsg = "Highlights cleared"
	case 1:
		m.filters.ClearIncludes()
		m.errMsg = "Include filters cleared"
	case 2:
		m.filters.ClearExcludes()
		m.errMsg = "Exclude filters cleared"
	case 3:
		return m.clearAllFilters()
	}
	m.errTime = time.Now()
	m.clearMenuOpen = false
	m.dirty = true
	return m
}

// DockerReconnectCmd returns a command to attempt Docker reconnection
func DockerReconnectCmd() tea.Cmd {
	return func() tea.Msg {
		// This is a placeholder - in a real implementation, this would
		// attempt to reconnect to Docker and return appropriate messages
		// For now, we'll just return a success message after a brief delay
		time.Sleep(500 * time.Millisecond)
		return DockerErrorMsg{
			Error:       nil, // nil indicates success
			Recoverable: false,
		}
	}
}

// truncateLine truncates a line to the maximum configured length, adding "..." if truncated
func (m Model) truncateLine(line string) string {
	if len(line) <= m.perf.MaxLineLength {
		return line
	}

	// Use rune-based truncation to handle Unicode properly
	runes := []rune(line)
	if len(runes) <= m.perf.MaxLineLength-3 { // Reserve space for "..."
		return line
	}

	return string(runes[:m.perf.MaxLineLength-3]) + "..."
}
