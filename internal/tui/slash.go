package tui

import (
	"fmt"
	"sort"
	"strings"
)

// SlashCommand defines a slash command handler.
type SlashCommand struct {
	Name        string
	Description string
	Execute     func(m *model) string // returns output text to Println
}

// slashRegistry holds all registered slash commands.
type slashRegistry struct {
	commands map[string]SlashCommand
	names    []string // sorted for completion
}

// newSlashRegistry creates a registry with all built-in slash commands.
func newSlashRegistry() *slashRegistry {
	r := &slashRegistry{
		commands: make(map[string]SlashCommand),
	}

	r.register(SlashCommand{
		Name:        "help",
		Description: "Show available commands",
		Execute: func(m *model) string {
			return m.slashReg.helpText()
		},
	})

	r.register(SlashCommand{
		Name:        "model",
		Description: "Show current model",
		Execute: func(m *model) string {
			return fmt.Sprintf("Current model: %s", m.modelName)
		},
	})

	r.register(SlashCommand{
		Name:        "version",
		Description: "Show version",
		Execute: func(m *model) string {
			return fmt.Sprintf("claude %s (Go)", m.version)
		},
	})

	r.register(SlashCommand{
		Name:        "cost",
		Description: "Show token usage and cost",
		Execute: func(m *model) string {
			return renderCostSummary(&m.tokens)
		},
	})

	r.register(SlashCommand{
		Name:        "context",
		Description: "Show context window usage",
		Execute: func(m *model) string {
			return fmt.Sprintf("Messages in history: %d", m.loop.History().Len())
		},
	})

	r.register(SlashCommand{
		Name:        "compact",
		Description: "Compact conversation history",
		Execute:     nil, // handled specially in Update (needs async)
	})

	r.register(SlashCommand{
		Name:        "quit",
		Description: "Exit the program",
		Execute:     nil, // handled specially in Update
	})

	r.register(SlashCommand{
		Name:        "exit",
		Description: "Exit the program",
		Execute:     nil, // alias for quit
	})

	return r
}

func (r *slashRegistry) register(cmd SlashCommand) {
	r.commands[cmd.Name] = cmd
	r.names = append(r.names, cmd.Name)
	sort.Strings(r.names)
}

// lookup returns a command and whether it was found.
func (r *slashRegistry) lookup(name string) (SlashCommand, bool) {
	cmd, ok := r.commands[name]
	return cmd, ok
}

// complete returns command names matching the given prefix.
func (r *slashRegistry) complete(prefix string) []string {
	var matches []string
	for _, name := range r.names {
		if strings.HasPrefix(name, prefix) {
			matches = append(matches, name)
		}
	}
	return matches
}

// helpText returns formatted help output.
func (r *slashRegistry) helpText() string {
	var b strings.Builder
	b.WriteString("Available commands:\n")
	for _, name := range r.names {
		if name == "exit" {
			continue // don't show exit since quit is listed
		}
		cmd := r.commands[name]
		b.WriteString(fmt.Sprintf("  /%-12s %s\n", name, cmd.Description))
	}
	return strings.TrimRight(b.String(), "\n")
}
