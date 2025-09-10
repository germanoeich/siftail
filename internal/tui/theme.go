package tui

import "github.com/charmbracelet/lipgloss"

// Theme defines all styles used by the UI so we can swap palettes easily.
type Theme struct {
	Name string
	// Severity badges
	DebugBadgeStyle lipgloss.Style
	InfoBadgeStyle  lipgloss.Style
	WarnBadgeStyle  lipgloss.Style
	ErrorBadgeStyle lipgloss.Style
	OtherBadgeStyle lipgloss.Style

	// Prefix styles
	ContainerStyle lipgloss.Style
	TimestampStyle lipgloss.Style

	// Inline emphasis
	HighlightStyle lipgloss.Style
	FindHitStyle   lipgloss.Style

	// Chrome
	ToolbarStyle     lipgloss.Style
	HotkeyPillStyle  lipgloss.Style
	HotkeyKeyStyle   lipgloss.Style
	HotkeyLabelStyle lipgloss.Style
	StatusStyle      lipgloss.Style
	PromptStyle      lipgloss.Style
}

func DarkTheme() *Theme {
	return &Theme{
		Name:            "dark",
		DebugBadgeStyle: lipgloss.NewStyle().Foreground(lipgloss.Color("240")),
		InfoBadgeStyle:  lipgloss.NewStyle().Foreground(lipgloss.Color("39")).Bold(true),
		WarnBadgeStyle:  lipgloss.NewStyle().Foreground(lipgloss.Color("214")).Bold(true),
		ErrorBadgeStyle: lipgloss.NewStyle().Foreground(lipgloss.Color("196")).Bold(true),
		OtherBadgeStyle: lipgloss.NewStyle().Foreground(lipgloss.Color("13")).Bold(true),

		ContainerStyle: lipgloss.NewStyle().Foreground(lipgloss.Color("33")).Bold(true),
		TimestampStyle: lipgloss.NewStyle().Foreground(lipgloss.Color("240")),

		HighlightStyle: lipgloss.NewStyle().Background(lipgloss.Color("220")).Foreground(lipgloss.Color("0")),
		FindHitStyle:   lipgloss.NewStyle().Background(lipgloss.Color("201")).Foreground(lipgloss.Color("15")).Bold(true),

		ToolbarStyle:     lipgloss.NewStyle().Foreground(lipgloss.Color("250")).Bold(true),
		HotkeyPillStyle:  lipgloss.NewStyle().Background(lipgloss.Color("236")).Foreground(lipgloss.Color("15")).Padding(0, 0),
		HotkeyKeyStyle:   lipgloss.NewStyle().Bold(true),
		HotkeyLabelStyle: lipgloss.NewStyle().Faint(true),
		StatusStyle:      lipgloss.NewStyle().Foreground(lipgloss.Color("246")).Italic(true),
		PromptStyle:      lipgloss.NewStyle().Foreground(lipgloss.Color("81")).Bold(true),
	}
}

func DraculaTheme() *Theme {
	// Dracula palette-ish
	return &Theme{
		Name:            "dracula",
		DebugBadgeStyle: lipgloss.NewStyle().Foreground(lipgloss.Color("244")),
		InfoBadgeStyle:  lipgloss.NewStyle().Foreground(lipgloss.Color("81")).Bold(true),
		WarnBadgeStyle:  lipgloss.NewStyle().Foreground(lipgloss.Color("221")).Bold(true),
		ErrorBadgeStyle: lipgloss.NewStyle().Foreground(lipgloss.Color("197")).Bold(true),
		OtherBadgeStyle: lipgloss.NewStyle().Foreground(lipgloss.Color("141")).Bold(true),

		ContainerStyle: lipgloss.NewStyle().Foreground(lipgloss.Color("117")).Bold(true),
		TimestampStyle: lipgloss.NewStyle().Foreground(lipgloss.Color("60")),

		HighlightStyle: lipgloss.NewStyle().Background(lipgloss.Color("228")).Foreground(lipgloss.Color("0")),
		FindHitStyle:   lipgloss.NewStyle().Background(lipgloss.Color("141")).Foreground(lipgloss.Color("231")).Bold(true),

		ToolbarStyle:     lipgloss.NewStyle().Foreground(lipgloss.Color("235")).Bold(true),
		HotkeyPillStyle:  lipgloss.NewStyle().Background(lipgloss.Color("250")).Foreground(lipgloss.Color("235")).Padding(0, 0),
		HotkeyKeyStyle:   lipgloss.NewStyle().Bold(true),
		HotkeyLabelStyle: lipgloss.NewStyle().Foreground(lipgloss.Color("60")),
		StatusStyle:      lipgloss.NewStyle().Foreground(lipgloss.Color("245")).Italic(true),
		PromptStyle:      lipgloss.NewStyle().Foreground(lipgloss.Color("141")).Bold(true),
	}
}

func NordTheme() *Theme {
	// Nord palette-ish
	return &Theme{
		Name:            "nord",
		DebugBadgeStyle: lipgloss.NewStyle().Foreground(lipgloss.Color("245")),
		InfoBadgeStyle:  lipgloss.NewStyle().Foreground(lipgloss.Color("44")).Bold(true),
		WarnBadgeStyle:  lipgloss.NewStyle().Foreground(lipgloss.Color("179")).Bold(true),
		ErrorBadgeStyle: lipgloss.NewStyle().Foreground(lipgloss.Color("204")).Bold(true),
		OtherBadgeStyle: lipgloss.NewStyle().Foreground(lipgloss.Color("141")).Bold(true),

		ContainerStyle: lipgloss.NewStyle().Foreground(lipgloss.Color("81")).Bold(true),
		TimestampStyle: lipgloss.NewStyle().Foreground(lipgloss.Color("243")),

		HighlightStyle: lipgloss.NewStyle().Background(lipgloss.Color("153")).Foreground(lipgloss.Color("234")),
		FindHitStyle:   lipgloss.NewStyle().Background(lipgloss.Color("39")).Foreground(lipgloss.Color("230")).Bold(true),

		ToolbarStyle:     lipgloss.NewStyle().Foreground(lipgloss.Color("238")).Bold(true),
		HotkeyPillStyle:  lipgloss.NewStyle().Background(lipgloss.Color("195")).Foreground(lipgloss.Color("0")).Padding(0, 0),
		HotkeyKeyStyle:   lipgloss.NewStyle().Bold(true),
		HotkeyLabelStyle: lipgloss.NewStyle().Foreground(lipgloss.Color("236")),
		StatusStyle:      lipgloss.NewStyle().Foreground(lipgloss.Color("245")).Italic(true),
		PromptStyle:      lipgloss.NewStyle().Foreground(lipgloss.Color("39")).Bold(true),
	}
}

func LightTheme() *Theme {
	return &Theme{
		Name:            "light",
		DebugBadgeStyle: lipgloss.NewStyle().Foreground(lipgloss.Color("60")),
		InfoBadgeStyle:  lipgloss.NewStyle().Foreground(lipgloss.Color("27")).Bold(true),
		WarnBadgeStyle:  lipgloss.NewStyle().Foreground(lipgloss.Color("130")).Bold(true),
		ErrorBadgeStyle: lipgloss.NewStyle().Foreground(lipgloss.Color("124")).Bold(true),
		OtherBadgeStyle: lipgloss.NewStyle().Foreground(lipgloss.Color("90")).Bold(true),

		ContainerStyle: lipgloss.NewStyle().Foreground(lipgloss.Color("24")).Bold(true),
		TimestampStyle: lipgloss.NewStyle().Foreground(lipgloss.Color("102")),

		HighlightStyle: lipgloss.NewStyle().Background(lipgloss.Color("227")).Foreground(lipgloss.Color("0")),
		FindHitStyle:   lipgloss.NewStyle().Background(lipgloss.Color("171")).Foreground(lipgloss.Color("0")).Bold(true),

		ToolbarStyle:     lipgloss.NewStyle().Foreground(lipgloss.Color("0")).Bold(true),
		HotkeyPillStyle:  lipgloss.NewStyle().Background(lipgloss.Color("253")).Foreground(lipgloss.Color("0")).Padding(0, 0),
		HotkeyKeyStyle:   lipgloss.NewStyle().Bold(true),
		HotkeyLabelStyle: lipgloss.NewStyle().Foreground(lipgloss.Color("60")),
		StatusStyle:      lipgloss.NewStyle().Foreground(lipgloss.Color("60")).Italic(true),
		PromptStyle:      lipgloss.NewStyle().Foreground(lipgloss.Color("27")).Bold(true),
	}
}

var themes = []*Theme{DarkTheme(), DraculaTheme(), NordTheme(), LightTheme()}

func themeByName(name string) *Theme {
	for _, t := range themes {
		if t.Name == name {
			return t
		}
	}
	return DarkTheme()
}

func themeNames() []string {
	out := make([]string, 0, len(themes))
	for _, t := range themes {
		out = append(out, t.Name)
	}
	return out
}
