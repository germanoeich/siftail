package tui

import (
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/germanoeich/siftail/internal/core"
)

// Styling constants and themes
var (
	// Color palette
	colorDebug    = lipgloss.Color("240") // Gray
	colorInfo     = lipgloss.Color("36")  // Cyan
	colorWarn     = lipgloss.Color("11")  // Yellow
	colorError    = lipgloss.Color("9")   // Red
	colorOther    = lipgloss.Color("13")  // Magenta
	colorContainer = lipgloss.Color("33") // Blue
	colorTimestamp = lipgloss.Color("240") // Gray

	// Highlight colors
	colorHighlight = lipgloss.Color("11")  // Yellow background
	colorFindHit   = lipgloss.Color("201") // Pink background

	// UI colors
	colorToolbar   = lipgloss.Color("0")   // Black text
	colorToolbarBg = lipgloss.Color("15")  // White background
	colorStatus    = lipgloss.Color("240") // Gray text
	colorPrompt    = lipgloss.Color("12")  // Bright blue

	// Base styles
	baseStyle = lipgloss.NewStyle()

	// Severity badge styles
	debugBadgeStyle = baseStyle.Copy().
			Foreground(colorDebug).
			Bold(false)

	infoBadgeStyle = baseStyle.Copy().
			Foreground(colorInfo).
			Bold(true)

	warnBadgeStyle = baseStyle.Copy().
			Foreground(colorWarn).
			Bold(true)

	errorBadgeStyle = baseStyle.Copy().
			Foreground(colorError).
			Bold(true)

	otherBadgeStyle = baseStyle.Copy().
			Foreground(colorOther).
			Bold(true)

	// Container name style
	containerStyle = baseStyle.Copy().
			Foreground(colorContainer).
			Bold(true)

	// Timestamp style
	timestampStyle = baseStyle.Copy().
			Foreground(colorTimestamp)

	// Highlight styles
	highlightStyle = baseStyle.Copy().
			Background(colorHighlight).
			Foreground(lipgloss.Color("0"))

	findHitStyle = baseStyle.Copy().
			Background(colorFindHit).
			Foreground(lipgloss.Color("15"))

	// Toolbar styles
	toolbarStyle = baseStyle.Copy().
			Foreground(colorToolbar).
			Background(colorToolbarBg).
			Bold(true).
			Padding(0, 1)

	// Status line style
	statusStyle = baseStyle.Copy().
			Foreground(colorStatus).
			Italic(true)

	// Prompt style
	promptStyle = baseStyle.Copy().
			Foreground(colorPrompt).
			Bold(true)
)

// View renders the complete TUI interface
func (m Model) View() string {
	if m.width <= 0 || m.height <= 0 {
		return "Terminal too small"
	}

	var sections []string

	// Status line at top
	sections = append(sections, m.renderStatusLine())

	// Main viewport content
	sections = append(sections, m.vp.View())

	// Prompt overlay or toolbar at bottom
	if m.inPrompt {
		sections = append(sections, m.renderPrompt())
	} else {
		sections = append(sections, m.renderToolbar())
	}

	return lipgloss.JoinVertical(lipgloss.Left, sections...)
}

// renderStatusLine shows current mode, filters, and stats
func (m Model) renderStatusLine() string {
	var parts []string

	// Mode indicator
	var modeStr string
	switch m.mode {
	case ModeFile:
		modeStr = "FILE"
	case ModeStdin:
		modeStr = "STDIN"
	case ModeDocker:
		modeStr = "DOCKER"
	}
	parts = append(parts, fmt.Sprintf("[%s]", modeStr))

	// Log count
	totalEvents := m.ring.Size()
	parts = append(parts, fmt.Sprintf("Lines: %d", totalEvents))

	// Active filters
	if len(m.filters.Include) > 0 {
		parts = append(parts, fmt.Sprintf("Include: %d", len(m.filters.Include)))
	}
	if len(m.filters.Exclude) > 0 {
		parts = append(parts, fmt.Sprintf("Exclude: %d", len(m.filters.Exclude)))
	}
	if len(m.filters.Highlights) > 0 {
		parts = append(parts, fmt.Sprintf("Highlights: %d", len(m.filters.Highlights)))
	}

	// Find status
	if m.search.IsActive() {
		current, total := m.search.Position()
		parts = append(parts, fmt.Sprintf("Find: %d/%d", current, total))
	}

	// Docker container count (in docker mode)
	if m.mode == ModeDocker {
		visibleContainers := 0
		for _, visible := range m.dockerUI.Containers {
			if visible {
				visibleContainers++
			}
		}
		parts = append(parts, fmt.Sprintf("Containers: %d/%d", visibleContainers, len(m.dockerUI.Containers)))
	}

	// Error message
	if m.errMsg != "" {
		parts = append(parts, fmt.Sprintf("ERROR: %s", m.errMsg))
	}

	statusLine := strings.Join(parts, " | ")
	
	// Pad to full width and apply style
	statusLine = lipgloss.NewStyle().
		Width(m.width).
		Align(lipgloss.Left).
		Render(statusStyle.Render(statusLine))

	return statusLine
}

// renderToolbar displays the nano-style hotkey toolbar
func (m Model) renderToolbar() string {
	// First line: main hotkeys
	var hotkeys []string
	hotkeys = append(hotkeys, "^Q Quit", "^C Cancel", "h Highlight", "f Find", "F Filter", "U FilterOut")
	
	if m.mode == ModeDocker {
		hotkeys = append(hotkeys, "l Containers", "P Presets")
	}

	hotkeyLine := strings.Join(hotkeys, "  ")

	// Second line: severity level mapping
	levelLine := m.renderLevelMapping()

	// Combine both lines
	toolbar := lipgloss.JoinVertical(lipgloss.Left, 
		toolbarStyle.Width(m.width).Render(hotkeyLine),
		toolbarStyle.Width(m.width).Render(levelLine),
	)

	return toolbar
}

// renderLevelMapping shows the dynamic severity level mapping
func (m Model) renderLevelMapping() string {
	indexToName, enabled := m.levels.GetSnapshot()
	
	var parts []string
	for i := 1; i <= 9; i++ {
		name := indexToName[i]
		if name == "" {
			continue // Skip empty slots
		}
		
		status := "off"
		if enabled[i] {
			status = "on"
		}
		
		part := fmt.Sprintf("%d:%s[%s]", i, name, status)
		parts = append(parts, part)
	}
	
	return strings.Join(parts, " ")
}

// renderPrompt shows the current text input prompt
func (m Model) renderPrompt() string {
	var promptLabel string
	switch m.promptKind {
	case PromptFind:
		promptLabel = "Find: "
	case PromptHighlight:
		promptLabel = "Highlight: "
	case PromptFilterIn:
		promptLabel = "Filter In: "
	case PromptFilterOut:
		promptLabel = "Filter Out: "
	case PromptPresetName:
		promptLabel = "Preset Name: "
	}

	prompt := lipgloss.JoinHorizontal(
		lipgloss.Left,
		promptStyle.Render(promptLabel),
		m.input.View(),
	)

	return lipgloss.NewStyle().
		Width(m.width).
		Align(lipgloss.Left).
		Render(prompt)
}

// renderEventsWithFullStyling renders events with comprehensive styling
func (m Model) renderEventsWithFullStyling(events []core.LogEvent) string {
	if len(events) == 0 {
		return lipgloss.NewStyle().
			Foreground(lipgloss.Color("240")).
			Width(m.vp.Width).
			Height(m.vp.Height).
			Align(lipgloss.Center, lipgloss.Center).
			Render("No log entries...")
	}

	var lines []string
	maxLines := m.vp.Height

	// Render only the visible portion to improve performance
	startIdx := 0
	if len(events) > maxLines {
		startIdx = len(events) - maxLines
	}

	for i := startIdx; i < len(events); i++ {
		line := m.renderEventWithFullStyling(events[i])
		lines = append(lines, line)
	}

	return strings.Join(lines, "\n")
}

// renderEventWithFullStyling applies comprehensive styling to a log event
func (m Model) renderEventWithFullStyling(event core.LogEvent) string {
	var parts []string

	// 1. Timestamp prefix (optional, configurable)
	if !event.Time.IsZero() {
		timestamp := event.Time.Format("15:04:05.000")
		parts = append(parts, timestampStyle.Render(timestamp))
	}

	// 2. Container name prefix (Docker mode only)
	if m.mode == ModeDocker && event.Container != "" {
		container := fmt.Sprintf("[%s]", event.Container)
		parts = append(parts, containerStyle.Render(container))
	}

	// 3. Severity badge
	if event.LevelStr != "" {
		badge := m.renderSeverityBadge(event.Level, event.LevelStr)
		parts = append(parts, badge)
	}

	// 4. Main log line with highlighting
	logLine := m.applyHighlighting(event.Line, event.Seq)
	parts = append(parts, logLine)

	// Join all parts with single space
	fullLine := strings.Join(parts, " ")

	// 5. Truncate if too long for viewport width
	return m.truncateToWidth(fullLine, m.vp.Width)
}

// renderSeverityBadge creates a styled severity level indicator
func (m Model) renderSeverityBadge(level core.Severity, levelStr string) string {
	var style lipgloss.Style
	
	switch level {
	case core.SevDebug:
		style = debugBadgeStyle
	case core.SevInfo:
		style = infoBadgeStyle
	case core.SevWarn:
		style = warnBadgeStyle
	case core.SevError:
		style = errorBadgeStyle
	default:
		style = otherBadgeStyle
	}

	// Normalize badge width for alignment
	badge := fmt.Sprintf("%-5s", strings.ToUpper(levelStr))
	return style.Render(badge)
}

// applyHighlighting applies highlight and find match styling to text
func (m Model) applyHighlighting(line string, seq uint64) string {
	// Check if this line should be highlighted
	shouldHighlight := m.filters.ShouldHighlight(line)
	
	// Check if this is the current find hit
	isCurrentFindHit := m.search.IsActive() && m.search.Current() == seq
	
	// Check if this line matches the find pattern
	var findMatcher core.TextMatcher
	var isFindMatch bool
	if m.search.IsActive() {
		findMatcher = m.search.GetMatcher()
		isFindMatch = findMatcher.Match(line)
	}

	// If no highlighting needed, return as-is
	if !shouldHighlight && !isCurrentFindHit && !isFindMatch {
		return line
	}

	// Apply styling based on priority: find hit > find match > highlight
	if isCurrentFindHit {
		// Highlight the entire line for current find hit
		return findHitStyle.Render(line)
	} else if isFindMatch {
		// Apply find match styling to matching portions
		return m.applyInlineHighlight(line, findMatcher, findHitStyle)
	} else if shouldHighlight {
		// Apply highlight styling to matching portions
		return m.applyAllHighlights(line)
	}

	return line
}

// applyAllHighlights applies all highlight patterns to a line
func (m Model) applyAllHighlights(line string) string {
	result := line
	
	// Apply each highlight pattern
	for _, highlight := range m.filters.Highlights {
		result = m.applyInlineHighlight(result, highlight, highlightStyle)
	}
	
	return result
}

// applyInlineHighlight applies styling to matching substrings within a line
func (m Model) applyInlineHighlight(line string, matcher core.TextMatcher, style lipgloss.Style) string {
	if matcher.IsRegex() {
		// For regex patterns, we need to compile and find matches
		// This is a simplified implementation - could be optimized
		return m.applyRegexHighlight(line, matcher, style)
	} else {
		// For substring patterns, use simple case-insensitive replacement
		return m.applySubstringHighlight(line, matcher, style)
	}
}

// applySubstringHighlight highlights all occurrences of a substring
func (m Model) applySubstringHighlight(line string, matcher core.TextMatcher, style lipgloss.Style) string {
	pattern := strings.TrimSpace(matcher.Raw())
	if pattern == "" {
		return line
	}

	// Case-insensitive find and replace
	lowerLine := strings.ToLower(line)
	lowerPattern := strings.ToLower(pattern)
	
	if !strings.Contains(lowerLine, lowerPattern) {
		return line
	}

	// Find all occurrences and replace them with styled versions
	result := line
	startIdx := 0
	
	for {
		idx := strings.Index(strings.ToLower(result[startIdx:]), lowerPattern)
		if idx == -1 {
			break
		}
		
		actualIdx := startIdx + idx
		actualEnd := actualIdx + len(pattern)
		
		// Extract the actual case-preserved match
		originalMatch := result[actualIdx:actualEnd]
		styledMatch := style.Render(originalMatch)
		
		// Replace in the result string
		result = result[:actualIdx] + styledMatch + result[actualEnd:]
		
		// Move past this match (accounting for style escape sequences)
		startIdx = actualIdx + len(styledMatch)
	}
	
	return result
}

// applyRegexHighlight highlights regex matches (simplified implementation)
func (m Model) applyRegexHighlight(line string, matcher core.TextMatcher, style lipgloss.Style) string {
	// Extract regex pattern from matcher
	raw := matcher.Raw()
	if len(raw) < 3 || !strings.HasPrefix(raw, "/") || !strings.HasSuffix(raw, "/") {
		return line
	}
	
	pattern := raw[1 : len(raw)-1]
	regex, err := regexp.Compile("(?i)" + pattern)
	if err != nil {
		return line // If regex is invalid, return original line
	}
	
	// Find all matches and replace with styled versions
	return regex.ReplaceAllStringFunc(line, func(match string) string {
		return style.Render(match)
	})
}

// truncateToWidth ensures a line fits within the specified width
func (m Model) truncateToWidth(line string, maxWidth int) string {
	if maxWidth <= 0 {
		return ""
	}

	// Account for escape sequences by using display width
	displayWidth := lipgloss.Width(line)
	
	if displayWidth <= maxWidth {
		return line
	}

	// Simplified truncation - in a real implementation, this would need
	// to properly handle ANSI escape sequences
	if len(line) > maxWidth-3 {
		return line[:maxWidth-3] + "..."
	}
	
	return line
}

// renderDockerContainerList renders the container selection overlay
func (m Model) renderDockerContainerList() string {
	if !m.dockerUI.ContainerListOpen {
		return ""
	}

	// Get sorted list of containers
	var containers []string
	for name := range m.dockerUI.Containers {
		containers = append(containers, name)
	}
	sort.Strings(containers)

	var lines []string
	lines = append(lines, "Container List (Space: toggle, a: toggle all, Enter/Esc: close)")
	lines = append(lines, "")

	// All toggle option
	allStatus := "[ ]"
	if m.dockerUI.AllToggle {
		allStatus = "[x]"
	}
	allLine := fmt.Sprintf("  %s ALL", allStatus)
	if m.dockerUI.SelectedContainer == -1 {
		allLine = "> " + allLine[2:] // Highlight selection
	}
	lines = append(lines, allLine)

	// Individual containers
	for i, container := range containers {
		status := "[ ]"
		if m.dockerUI.Containers[container] {
			status = "[x]"
		}
		
		line := fmt.Sprintf("  %s %s", status, container)
		if m.dockerUI.SelectedContainer == i {
			line = "> " + line[2:] // Highlight selection
		}
		lines = append(lines, line)
	}

	// Create bordered overlay
	content := strings.Join(lines, "\n")
	overlay := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("36")).
		Padding(1).
		Width(min(60, m.width-4)).
		Render(content)

	return overlay
}

// Helper function for min (Go 1.21+ has this built-in)
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

