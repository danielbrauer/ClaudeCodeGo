package tui

// registerCostCommand registers /cost.
func registerCostCommand(r *slashRegistry) {
	r.register(SlashCommand{
		Name:        "cost",
		Description: "Show token usage and cost",
		Execute:     textCommand(costText),
	})
}

func costText(m *model) string {
	return renderCostSummary(&m.tokens)
}
