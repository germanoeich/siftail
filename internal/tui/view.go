package tui

import (
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/germanoeich/siftail/internal/core"
)

// Styling constants and themes now come from m.theme

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

	baseView := lipgloss.JoinVertical(lipgloss.Left, sections...)

	// Docker container list overlay (if open)
	if m.dockerUI.ContainerListOpen {
		overlay := m.renderDockerContainerList()
		// Center the overlay on the screen
		overlayStyle := lipgloss.NewStyle().
			Align(lipgloss.Center, lipgloss.Center).
			Width(m.width).
			Height(m.height)
		return overlayStyle.Render(overlay)
	}

	// Docker preset manager overlay (if open)
	if m.dockerUI.PresetManagerOpen {
		overlay := m.renderDockerPresetManager()
		// Center the overlay on the screen
		overlayStyle := lipgloss.NewStyle().
			Align(lipgloss.Center, lipgloss.Center).
			Width(m.width).
			Height(m.height)
		return overlayStyle.Render(overlay)
	}

	// Clear menu overlay
	if m.clearMenuOpen {
		overlay := m.renderClearMenu()
		overlayStyle := lipgloss.NewStyle().
			Align(lipgloss.Center, lipgloss.Center).
			Width(m.width).
			Height(m.height)
		return overlayStyle.Render(overlay)
	}

	return baseView
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

	// Error message with timestamp
	if m.errMsg != "" {
		timeStr := m.errTime.Format("15:04:05")
		parts = append(parts, fmt.Sprintf("ERROR [%s]: %s", timeStr, m.errMsg))
	}

	statusLine := strings.Join(parts, " | ")

	// Pad to full width and apply style
	statusLine = lipgloss.NewStyle().
		Width(m.width).
		Align(lipgloss.Left).
		Render(m.theme.StatusStyle.Render(statusLine))

	return statusLine
}

// renderToolbar displays the nano-style hotkey toolbar
func (m Model) renderToolbar() string {
	// First line: render hotkeys as per-element "pills"
	type hk struct{ key, label string }
	var keys []hk
	keys = append(keys,
		hk{"^Q", "Quit"},
		hk{"Shift+1..9", "Focus"},
		hk{"0", "EnableAll"},
		hk{"h", "Highlight"},
		hk{"f", "Find"},
		hk{"F", "FilterIn"},
		hk{"U", "FilterOut"},
		hk{"c", "Clear"},
		hk{"C", "ClearAll"},
		hk{"T", "Theme"},
		hk{"Ctrl+S", "Select"},
	)
	if m.mode == ModeDocker {
		keys = append(keys, hk{"l", "Containers"}, hk{"P", "Presets"})
	}

	renderHK := func(k hk) string {
		// Only the key gets a background; the label stays plain/themed
		key := m.theme.HotkeyPillStyle.Render(m.theme.HotkeyKeyStyle.Render(k.key))
		label := m.theme.HotkeyLabelStyle.Render(" " + k.label)
		return key + label
	}

	var pills []string
	for _, k := range keys {
		pills = append(pills, renderHK(k))
	}
	hotkeyLine := strings.Join(pills, "")

	// Severity level mapping (now above hotkeys)
	levelLine := m.theme.ToolbarStyle.Render(m.renderLevelMapping())

	return lipgloss.JoinVertical(lipgloss.Left, levelLine, hotkeyLine)
}

// renderClearMenu draws a small menu to clear filters/highlights selectively
func (m Model) renderClearMenu() string {
	items := []string{
		"h: Clear Highlights",
		"i: Clear Include Filters",
		"u: Clear Exclude Filters",
		"a: Clear ALL (filters + highlights)",
	}

	var lines []string
	lines = append(lines, "Clear Menu (Esc/c to close, Enter to apply)")
	lines = append(lines, "")
	for i, it := range items {
		prefix := "  "
		if i == m.clearMenuSel {
			prefix = "> "
		}
		lines = append(lines, prefix+it)
	}

	content := strings.Join(lines, "\n")
	overlay := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("205")).
		Padding(1).
		Width(min(50, m.width-4)).
		Render(content)
	return overlay
}

// renderLevelMapping shows the dynamic severity level mapping
func (m Model) renderLevelMapping() string {
	indexToName, enabled := m.levels.GetSnapshot()

	var parts []string
	for i := 1; i <= 9; i++ {
		name := indexToName[i]
		if name == "" {
			continue
		}

		// Choose a style per severity bucket
		var st lipgloss.Style
		switch i {
		case 1:
			st = m.theme.DebugBadgeStyle
		case 2:
			st = m.theme.InfoBadgeStyle
		case 3:
			st = m.theme.WarnBadgeStyle
		case 4:
			st = m.theme.ErrorBadgeStyle
		default:
			st = m.theme.OtherBadgeStyle
		}

		token := fmt.Sprintf("%d:%s", i, name)
		if !enabled[i] {
			st = st.Strikethrough(true).Faint(true)
		}
		parts = append(parts, st.Render(token))
	}

	return strings.Join(parts, "  ")
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
		m.theme.PromptStyle.Render(promptLabel),
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
	lines = make([]string, 0, len(events))
	for i := 0; i < len(events); i++ {
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
		parts = append(parts, m.theme.TimestampStyle.Render(timestamp))
	}

	// 2. Container name prefix (Docker mode only)
	if m.mode == ModeDocker && event.Container != "" {
		container := fmt.Sprintf("[%s]", event.Container)
		parts = append(parts, m.theme.ContainerStyle.Render(container))
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
		style = m.theme.DebugBadgeStyle
	case core.SevInfo:
		style = m.theme.InfoBadgeStyle
	case core.SevWarn:
		style = m.theme.WarnBadgeStyle
	case core.SevError:
		style = m.theme.ErrorBadgeStyle
	default:
		style = m.theme.OtherBadgeStyle
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
		return m.theme.FindHitStyle.Render(line)
	} else if isFindMatch {
		// Apply find match styling to matching portions
		return m.applyInlineHighlight(line, findMatcher, m.theme.FindHitStyle)
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
		result = m.applyInlineHighlight(result, highlight, m.theme.HighlightStyle)
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

// renderDockerPresetManager renders the preset management overlay
func (m Model) renderDockerPresetManager() string {
	if !m.dockerUI.PresetManagerOpen {
		return ""
	}

	var lines []string
	lines = append(lines, "Preset Manager (Enter: apply, s: save current, d: delete, r: refresh, Esc: close)")
	lines = append(lines, "")

	if len(m.dockerUI.Presets) == 0 {
		lines = append(lines, "No presets found.")
		lines = append(lines, "Press 's' to save current container visibility as a preset.")
	} else {
		// List presets
		for i, preset := range m.dockerUI.Presets {
			line := fmt.Sprintf("  %s", preset.Name)

			// Show container count
			visibleCount := 0
			totalCount := len(preset.Visible)
			for _, visible := range preset.Visible {
				if visible {
					visibleCount++
				}
			}
			line += fmt.Sprintf(" (%d/%d visible)", visibleCount, totalCount)

			// Highlight selected preset
			if i == m.dockerUI.SelectedPreset {
				line = "> " + line[2:] // Replace indentation with selection indicator
				line = lipgloss.NewStyle().
					Background(lipgloss.Color("240")).
					Foreground(lipgloss.Color("15")).
					Render(line)
			}

			lines = append(lines, line)
		}

		lines = append(lines, "")
		lines = append(lines, "Actions: Enter=Apply, s=Save Current, d=Delete Selected, r=Refresh")
	}

	// Create bordered overlay
	content := strings.Join(lines, "\n")
	overlay := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("33")). // Different color from container list
		Padding(1).
		Width(min(80, m.width-4)).
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
