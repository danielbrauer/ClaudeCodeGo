package tui

import (
	"fmt"
	"sort"
	"strings"

	"github.com/anthropics/claude-code-go/internal/skills"
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
		Description: "Show or switch model",
		Execute:     nil, // handled specially in handleSubmit (needs interactive picker)
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
		Name:        "mcp",
		Description: "Show MCP server status",
		Execute: func(m *model) string {
			if m.mcpStatus == nil {
				return "No MCP servers configured."
			}
			servers := m.mcpStatus.Servers()
			if len(servers) == 0 {
				return "No MCP servers connected."
			}
			var b strings.Builder
			b.WriteString(fmt.Sprintf("MCP servers (%d):\n", len(servers)))
			for _, name := range servers {
				b.WriteString("  " + m.mcpStatus.ServerStatus(name) + "\n")
			}
			return strings.TrimRight(b.String(), "\n")
		},
	})

	r.register(SlashCommand{
		Name:        "config",
		Description: "Open config panel",
		Execute:     nil, // handled specially in handleSubmit (needs mode switch)
	})

	r.register(SlashCommand{
		Name:        "settings",
		Description: "Open config panel",
		Execute:     nil, // alias for config
	})

	r.register(SlashCommand{
		Name:        "clear",
		Description: "Clear conversation history and free up context",
		Execute:     nil, // handled specially in handleSubmit (resets session state)
	})

	r.register(SlashCommand{
		Name:        "reset",
		Description: "Clear conversation history and free up context",
		Execute:     nil, // alias for clear
	})

	r.register(SlashCommand{
		Name:        "new",
		Description: "Clear conversation history and free up context",
		Execute:     nil, // alias for clear
	})

	r.register(SlashCommand{
		Name:        "memory",
		Description: "Edit Claude memory files",
		Execute:     nil, // handled specially in handleSubmit (needs tea.Exec)
	})

	r.register(SlashCommand{
		Name:        "init",
		Description: "Initialize a new CLAUDE.md file with codebase documentation",
		Execute:     nil, // handled specially in handleSubmit (sends prompt to loop)
	})

	r.register(SlashCommand{
		Name:        "login",
		Description: "Sign in to your Anthropic account",
		Execute:     nil, // handled specially in handleSubmit (triggers quit + re-auth)
	})

	r.register(SlashCommand{
		Name:        "logout",
		Description: "Log out from your Anthropic account",
		Execute:     nil, // handled specially in handleSubmit (clears credentials + quits)
	})

	r.register(SlashCommand{
		Name:        "compact",
		Description: "Compact conversation history",
		Execute:     nil, // handled specially in Update (needs async)
	})

	r.register(SlashCommand{
		Name:        "resume",
		Description: "Resume a previous session",
		Execute:     nil, // handled specially in Update (needs session picker)
	})

	r.register(SlashCommand{
		Name:        "continue",
		Description: "Resume the most recent session",
		Execute:     nil, // handled specially in Update
	})

	r.register(SlashCommand{
		Name:        "diff",
		Description: "View uncommitted changes",
		Execute:     nil, // handled specially in handleSubmit (needs async git)
	})

	r.register(SlashCommand{
		Name:        "review",
		Description: "Review a pull request",
		Execute:     nil, // handled specially in handleSubmit (sends prompt to loop)
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

// registerSkills adds slash commands for skills that have triggers.
// Each skill slash command is flagged as a "skill command" so handleSubmit
// can send the skill's content as a user message to the loop.
func (r *slashRegistry) registerSkills(loadedSkills []skills.Skill) {
	for _, s := range loadedSkills {
		if s.Trigger == "" {
			continue
		}
		name := strings.TrimPrefix(s.Trigger, "/")
		skillContent := s.Content // capture for closure
		r.register(SlashCommand{
			Name:        name,
			Description: s.Description,
			Execute: func(m *model) string {
				// Return the skill content as a sentinel — handleSubmit
				// will detect this prefix and send it as a message.
				return skillCommandPrefix + skillContent
			},
		})
	}
}

// skillCommandPrefix is a sentinel prefix used to identify skill slash commands.
// When a slash command's Execute returns a string starting with this prefix,
// the remainder is sent as a user message to the agentic loop.
const skillCommandPrefix = "\x00SKILL:"

// helpText returns formatted help output.
func (r *slashRegistry) helpText() string {
	var b strings.Builder
	b.WriteString("Available commands:\n")
	for _, name := range r.names {
		if name == "exit" || name == "reset" || name == "new" || name == "settings" {
			continue // don't show aliases
		}
		cmd := r.commands[name]
		b.WriteString(fmt.Sprintf("  /%-12s %s\n", name, cmd.Description))
	}
	return strings.TrimRight(b.String(), "\n")
}
