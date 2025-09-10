package tui

import (
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/germanoeich/siftail/internal/core"
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
	SelectedContainer int // index in sorted container list for navigation
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

	// App state
	mode       Mode
	followTail bool // auto-scroll when at bottom
	width      int
	height     int
	errMsg     string

	// Throttling for smooth updates
	lastRender time.Time
	dirty      bool // needs re-render
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
		width:  80,
		height: 24,
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

	case tea.KeyMsg:
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
		} else {
			// Handle main app keys
			switch msg.String() {
			case "q", "ctrl+c":
				return m, tea.Quit

			// Search and filters
			case "h":
				m = m.startPrompt(PromptHighlight, "Highlight: ")
			case "f":
				m = m.startPrompt(PromptFind, "Find: ")
			case "F":
				m = m.startPrompt(PromptFilterIn, "Filter In: ")
			case "U":
				m = m.startPrompt(PromptFilterOut, "Filter Out: ")

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

			// Docker mode keys
			case "l":
				if m.mode == ModeDocker {
					m.dockerUI.ContainerListOpen = !m.dockerUI.ContainerListOpen
				}
			case "P":
				if m.mode == ModeDocker {
					m = m.startPrompt(PromptPresetName, "Preset Name: ")
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
	}

	return m, tea.Batch(cmds...)
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
		m.errMsg = "Invalid pattern: " + err.Error()
		return m
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
		// TODO: Implement preset saving
		m.errMsg = "Preset saving not yet implemented"
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
	// TODO: Implement scrolling to specific sequence
	// This requires mapping sequence numbers to viewport line positions
	m.followTail = false
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
	
	// Throttle to ~30 FPS
	if m.dirty && now.Sub(m.lastRender) > 33*time.Millisecond {
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

	// Render events to viewport content
	content := m.renderEventsWithFullStyling(visibleEvents)
	m.vp.SetContent(content)

	// Auto-scroll if following tail
	if m.followTail {
		m.vp.GotoBottom()
	}

	return m
}

// renderBasicEvents converts log events to basic styled viewport content
func (m Model) renderBasicEvents(events []core.LogEvent) string {
	if len(events) == 0 {
		return lipgloss.NewStyle().
			Foreground(lipgloss.Color("240")).
			Render("No log entries...")
	}

	var lines []string
	for _, event := range events {
		line := m.renderEvent(event)
		lines = append(lines, line)
	}

	return lipgloss.JoinVertical(lipgloss.Left, lines...)
}

// renderEvent formats a single log event with styling
func (m Model) renderEvent(event core.LogEvent) string {
	// This is a basic implementation - will be enhanced in view.go
	line := event.Line
	
	// Add container prefix for Docker mode
	if m.mode == ModeDocker && event.Container != "" {
		containerStyle := lipgloss.NewStyle().
			Foreground(lipgloss.Color("33")).
			Bold(true)
		line = containerStyle.Render("["+event.Container+"]") + " " + line
	}

	// Apply highlighting
	if m.filters.ShouldHighlight(event.Line) {
		line = lipgloss.NewStyle().
			Background(lipgloss.Color("11")).
			Foreground(lipgloss.Color("0")).
			Render(line)
	}

	return line
}

// Message types for internal communication
type tickMsg time.Time
type refreshMsg struct{}

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