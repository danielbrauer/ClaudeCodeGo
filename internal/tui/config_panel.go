package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/anthropics/claude-code-go/internal/config"
)

// configSettingType defines the kind of config setting.
type configSettingType int

const (
	configBool configSettingType = iota
	configEnum
)

// configSetting describes a single setting in the config panel.
type configSetting struct {
	id      string            // JSON key in settings
	label   string            // Display label
	typ     configSettingType // bool or enum
	options []string          // For enum: available values
}

// configPanel holds the state for the interactive config panel.
type configPanel struct {
	settings    *config.Settings
	items       []configSetting
	cursor      int
	scrollOff   int
	searchQuery string
	searching   bool
	filtered    []int // indices into items matching the search
	changes     []string
	// Snapshot of initial values for change tracking.
	initial *config.Settings
}

// newConfigPanel creates a config panel with the current settings.
func newConfigPanel(s *config.Settings) *configPanel {
	cp := &configPanel{
		settings: s,
		initial:  snapshotSettings(s),
	}
	cp.items = cp.buildItems()
	cp.resetFilter()
	return cp
}

// snapshotSettings makes a shallow copy of the settings for change detection.
func snapshotSettings(s *config.Settings) *config.Settings {
	c := *s
	// Deep copy bool pointers.
	if s.AutoCompactEnabled != nil {
		v := *s.AutoCompactEnabled
		c.AutoCompactEnabled = &v
	}
	if s.Verbose != nil {
		v := *s.Verbose
		c.Verbose = &v
	}
	if s.ThinkingEnabled != nil {
		v := *s.ThinkingEnabled
		c.ThinkingEnabled = &v
	}
	if s.RespectGitignore != nil {
		v := *s.RespectGitignore
		c.RespectGitignore = &v
	}
	if s.FastMode != nil {
		v := *s.FastMode
		c.FastMode = &v
	}
	return &c
}

// buildItems returns the list of configurable settings.
func (cp *configPanel) buildItems() []configSetting {
	// Build permission mode options — exclude bypassPermissions if disabled.
	permModeOptions := []string{"default", "plan", "acceptEdits"}
	if cp.settings.DisableBypassPermissions != "disable" {
		permModeOptions = append(permModeOptions, "bypassPermissions")
	}

	return []configSetting{
		{id: "defaultPermissionMode", label: "Permission mode", typ: configEnum, options: permModeOptions},
		{id: "autoCompactEnabled", label: "Auto-compact", typ: configBool},
		{id: "thinkingEnabled", label: "Thinking mode", typ: configBool},
		{id: "fastMode", label: "Fast mode", typ: configBool},
		{id: "verbose", label: "Verbose output", typ: configBool},
		{id: "respectGitignore", label: "Respect .gitignore", typ: configBool},
		{id: "editorMode", label: "Editor mode", typ: configEnum, options: []string{"normal", "vim"}},
		{id: "diffTool", label: "Diff tool", typ: configEnum, options: []string{"auto", "terminal"}},
		{id: "theme", label: "Theme", typ: configEnum, options: []string{"dark", "light", "dark-daltonized", "light-daltonized"}},
		{id: "notifChannel", label: "Notifications", typ: configEnum, options: []string{"auto", "terminal_bell", "iterm2", "iterm2_with_bell", "notifications_disabled"}},
	}
}

// resetFilter resets the search filter to show all items.
func (cp *configPanel) resetFilter() {
	cp.filtered = make([]int, len(cp.items))
	for i := range cp.items {
		cp.filtered[i] = i
	}
}

// applyFilter updates the filtered indices based on the search query.
func (cp *configPanel) applyFilter() {
	if cp.searchQuery == "" {
		cp.resetFilter()
		return
	}
	q := strings.ToLower(cp.searchQuery)
	cp.filtered = cp.filtered[:0]
	for i, item := range cp.items {
		if strings.Contains(strings.ToLower(item.label), q) ||
			strings.Contains(strings.ToLower(item.id), q) {
			cp.filtered = append(cp.filtered, i)
		}
	}
	if cp.cursor >= len(cp.filtered) {
		cp.cursor = max(0, len(cp.filtered)-1)
	}
}

// getValue returns the current display value for a setting.
func (cp *configPanel) getValue(item configSetting) string {
	s := cp.settings
	switch item.id {
	case "defaultPermissionMode":
		if s.DefaultPermissionMode == "" {
			return "default"
		}
		return s.DefaultPermissionMode
	case "autoCompactEnabled":
		return fmt.Sprintf("%v", config.BoolVal(s.AutoCompactEnabled, true))
	case "thinkingEnabled":
		return fmt.Sprintf("%v", config.BoolVal(s.ThinkingEnabled, false))
	case "fastMode":
		return fmt.Sprintf("%v", config.BoolVal(s.FastMode, false))
	case "verbose":
		return fmt.Sprintf("%v", config.BoolVal(s.Verbose, false))
	case "respectGitignore":
		return fmt.Sprintf("%v", config.BoolVal(s.RespectGitignore, true))
	case "editorMode":
		if s.EditorMode == "" {
			return "normal"
		}
		return s.EditorMode
	case "diffTool":
		if s.DiffTool == "" {
			return "auto"
		}
		return s.DiffTool
	case "theme":
		if s.Theme == "" {
			return "dark"
		}
		return s.Theme
	case "notifChannel":
		if s.NotifChannel == "" {
			return "auto"
		}
		return s.NotifChannel
	}
	return ""
}

// toggleOrCycle applies the change for a setting. Booleans toggle; enums cycle.
func (cp *configPanel) toggleOrCycle() {
	if len(cp.filtered) == 0 {
		return
	}
	idx := cp.filtered[cp.cursor]
	item := cp.items[idx]
	s := cp.settings

	switch item.typ {
	case configBool:
		cp.toggleBool(item.id, s)
	case configEnum:
		cp.cycleEnum(item.id, item.options, s)
	}
}

func (cp *configPanel) toggleBool(id string, s *config.Settings) {
	var ptr **bool
	var def bool

	switch id {
	case "autoCompactEnabled":
		ptr = &s.AutoCompactEnabled
		def = true
	case "thinkingEnabled":
		ptr = &s.ThinkingEnabled
		def = false
	case "fastMode":
		ptr = &s.FastMode
		def = false
	case "verbose":
		ptr = &s.Verbose
		def = false
	case "respectGitignore":
		ptr = &s.RespectGitignore
		def = true
	default:
		return
	}

	current := config.BoolVal(*ptr, def)
	newVal := !current
	*ptr = config.BoolPtr(newVal)

	// Persist to disk.
	_ = config.SaveUserSetting(id, newVal)
}

func (cp *configPanel) cycleEnum(id string, options []string, s *config.Settings) {
	var currentPtr *string
	switch id {
	case "defaultPermissionMode":
		currentPtr = &s.DefaultPermissionMode
	case "editorMode":
		currentPtr = &s.EditorMode
	case "diffTool":
		currentPtr = &s.DiffTool
	case "theme":
		currentPtr = &s.Theme
	case "notifChannel":
		currentPtr = &s.NotifChannel
	default:
		return
	}

	current := *currentPtr
	if current == "" && len(options) > 0 {
		current = options[0]
	}

	// Find current index and cycle to next.
	nextIdx := 0
	for i, opt := range options {
		if opt == current {
			nextIdx = (i + 1) % len(options)
			break
		}
	}
	*currentPtr = options[nextIdx]

	// Persist to disk.
	_ = config.SaveUserSetting(id, options[nextIdx])
}

// buildChangeSummary returns a list of human-readable change descriptions.
func (cp *configPanel) buildChangeSummary() []string {
	var changes []string

	checkBool := func(label string, oldVal, newVal *bool, def bool) {
		o := config.BoolVal(oldVal, def)
		n := config.BoolVal(newVal, def)
		if o != n {
			if n {
				changes = append(changes, fmt.Sprintf("Enabled %s", label))
			} else {
				changes = append(changes, fmt.Sprintf("Disabled %s", label))
			}
		}
	}

	checkEnum := func(label, oldVal, newVal, def string) {
		if oldVal == "" {
			oldVal = def
		}
		if newVal == "" {
			newVal = def
		}
		if oldVal != newVal {
			changes = append(changes, fmt.Sprintf("Set %s to %s", label, newVal))
		}
	}

	s := cp.settings
	i := cp.initial

	checkEnum("permission mode", i.DefaultPermissionMode, s.DefaultPermissionMode, "default")
	checkBool("auto-compact", i.AutoCompactEnabled, s.AutoCompactEnabled, true)
	checkBool("thinking mode", i.ThinkingEnabled, s.ThinkingEnabled, false)
	checkBool("fast mode", i.FastMode, s.FastMode, false)
	checkBool("verbose output", i.Verbose, s.Verbose, false)
	checkBool("respect .gitignore", i.RespectGitignore, s.RespectGitignore, true)
	checkEnum("editor mode", i.EditorMode, s.EditorMode, "normal")
	checkEnum("diff tool", i.DiffTool, s.DiffTool, "auto")
	checkEnum("theme", i.Theme, s.Theme, "dark")
	checkEnum("notifications", i.NotifChannel, s.NotifChannel, "auto")

	return changes
}

// Config panel styles.
var (
	configTitleStyle = lipgloss.NewStyle().
			Foreground(colorPurple).
			Bold(true)

	configLabelStyle = lipgloss.NewStyle().
				Width(28)

	configValueStyle = lipgloss.NewStyle().
				Foreground(colorCyan)

	configSelectedLabelStyle = lipgloss.NewStyle().
					Width(28).
					Foreground(colorPurple).
					Bold(true)

	configSelectedValueStyle = lipgloss.NewStyle().
					Foreground(colorPurple).
					Bold(true)

	configSearchStyle = lipgloss.NewStyle().
				Foreground(colorDim)
)

// viewportHeight returns the number of visible setting rows.
func (cp *configPanel) viewportHeight(termHeight int) int {
	h := termHeight - 15 // leave room for title, search, help, status bar
	if h < 5 {
		h = 5
	}
	return h
}

// renderConfigPanel renders the config panel for the live region.
func (m model) renderConfigPanel() string {
	cp := m.configPanel
	if cp == nil {
		return ""
	}

	var b strings.Builder
	width := m.width
	if width < 40 {
		width = 80
	}

	// Title.
	b.WriteString(configTitleStyle.Render("Configure Claude Code preferences"))
	b.WriteString("\n")

	// Search bar.
	if cp.searching {
		b.WriteString(configSearchStyle.Render("Search: ") + cp.searchQuery + "_")
	} else {
		b.WriteString(configSearchStyle.Render("Search settings..."))
	}
	b.WriteString("\n\n")

	if len(cp.filtered) == 0 {
		b.WriteString(configSearchStyle.Render("  No matching settings."))
		b.WriteString("\n\n")
	} else {
		vpHeight := cp.viewportHeight(m.height)

		// Adjust scroll offset to keep cursor visible.
		if cp.cursor < cp.scrollOff {
			cp.scrollOff = cp.cursor
		}
		if cp.cursor >= cp.scrollOff+vpHeight {
			cp.scrollOff = cp.cursor - vpHeight + 1
		}

		// Show "more above" indicator.
		if cp.scrollOff > 0 {
			b.WriteString(configSearchStyle.Render(fmt.Sprintf("  ↑ %d more above", cp.scrollOff)))
			b.WriteString("\n")
		}

		end := cp.scrollOff + vpHeight
		if end > len(cp.filtered) {
			end = len(cp.filtered)
		}

		for vi := cp.scrollOff; vi < end; vi++ {
			idx := cp.filtered[vi]
			item := cp.items[idx]
			value := cp.getValue(item)

			if vi == cp.cursor {
				pointer := configSelectedLabelStyle.Render("> " + item.label)
				val := configSelectedValueStyle.Render(value)
				b.WriteString(pointer + "  " + val + "\n")
			} else {
				label := configLabelStyle.Render("  " + item.label)
				val := configValueStyle.Render(value)
				b.WriteString(label + "  " + val + "\n")
			}
		}

		// Show "more below" indicator.
		if end < len(cp.filtered) {
			b.WriteString(configSearchStyle.Render(fmt.Sprintf("  ↓ %d more below", len(cp.filtered)-end)))
			b.WriteString("\n")
		}
		b.WriteString("\n")
	}

	// Help text.
	b.WriteString(permHintStyle.Render("  Enter/Space - change  /  / - search  /  Esc - close"))

	return b.String()
}

// handleConfigKey processes key events during the config panel.
func (m model) handleConfigKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	cp := m.configPanel
	if cp == nil {
		m.mode = modeInput
		return m, nil
	}

	// Search mode: typing into the search bar.
	if cp.searching {
		switch msg.Type {
		case tea.KeyEscape:
			cp.searching = false
			cp.searchQuery = ""
			cp.resetFilter()
			return m, nil
		case tea.KeyEnter:
			cp.searching = false
			if len(cp.filtered) > 0 {
				cp.cursor = 0
			}
			return m, nil
		case tea.KeyBackspace:
			if len(cp.searchQuery) > 0 {
				cp.searchQuery = cp.searchQuery[:len(cp.searchQuery)-1]
				cp.applyFilter()
			}
			return m, nil
		default:
			if msg.Type == tea.KeyRunes {
				cp.searchQuery += string(msg.Runes)
				cp.applyFilter()
			} else if msg.Type == tea.KeySpace {
				cp.searchQuery += " "
				cp.applyFilter()
			}
			return m, nil
		}
	}

	// Normal config panel navigation.
	switch msg.String() {
	case "up", "k":
		if cp.cursor > 0 {
			cp.cursor--
		}
		return m, nil

	case "down", "j":
		if cp.cursor < len(cp.filtered)-1 {
			cp.cursor++
		}
		return m, nil

	case "enter", " ":
		cp.toggleOrCycle()
		return m, nil

	case "/":
		cp.searching = true
		cp.searchQuery = ""
		return m, nil

	case "esc", "q":
		return m.closeConfigPanel()

	case "ctrl+c":
		return m.closeConfigPanel()
	}

	return m, nil
}

// closeConfigPanel exits the config panel and prints a summary of changes.
func (m model) closeConfigPanel() (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	if m.configPanel != nil {
		// Sync fast mode if it changed via the config panel.
		// The panel already persisted to disk, so we only need to
		// update the runtime state (m.fastMode, loop, model switch).
		newFast := config.BoolVal(m.settings.FastMode, false)
		if newFast != m.fastMode {
			applyFastMode(&m, newFast)
		}

		// Sync permission mode if it changed via the config panel.
		newPermMode := m.settings.DefaultPermissionMode
		if newPermMode == "" {
			newPermMode = "default"
		}
		currentPermMode := m.getPermissionMode()
		if config.PermissionMode(newPermMode) != currentPermMode {
			m.setPermissionMode(config.ValidatePermissionMode(newPermMode))
		}

		changes := m.configPanel.buildChangeSummary()
		if len(changes) > 0 {
			summary := configTitleStyle.Render("Config changes:") + "\n"
			for _, c := range changes {
				summary += "  " + c + "\n"
			}
			cmds = append(cmds, tea.Println(strings.TrimRight(summary, "\n")))
		} else {
			cmds = append(cmds, tea.Println(configSearchStyle.Render("Config dialog dismissed")))
		}
	}

	m.configPanel = nil
	m.mode = modeInput
	m.textInput.Focus()
	cmds = append(cmds, nil) // placeholder; filtered below
	// Filter nil commands.
	var filtered []tea.Cmd
	for _, c := range cmds {
		if c != nil {
			filtered = append(filtered, c)
		}
	}
	return m, tea.Batch(filtered...)
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
