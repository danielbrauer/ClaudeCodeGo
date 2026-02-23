package tui

import "fmt"

// registerVersionCommand registers /version.
func registerVersionCommand(r *slashRegistry) {
	r.register(SlashCommand{
		Name:        "version",
		Description: "Show version",
		Execute:     textCommand(versionText),
	})
}

func versionText(m *model) string {
	return fmt.Sprintf("claude %s (Go)", m.version)
}
