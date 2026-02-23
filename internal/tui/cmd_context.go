package tui

import "fmt"

// registerContextCommand registers /context.
func registerContextCommand(r *slashRegistry) {
	r.register(SlashCommand{
		Name:        "context",
		Description: "Show context window usage",
		Execute:     textCommand(contextText),
	})
}

func contextText(m *model) string {
	return fmt.Sprintf("Messages in history: %d", m.loop.History().Len())
}
