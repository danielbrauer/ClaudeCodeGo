package tui

import (
	"fmt"
	"sort"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/anthropics/claude-code-go/internal/skills"
)

// SlashCommand defines a slash command handler.
type SlashCommand struct {
	Name        string
	Description string
	IsHidden    bool                                  // hidden commands are not shown in the help screen
	IsAlias     bool                                  // alias commands (e.g. exit→quit) are hidden from help
	IsSkill     bool                                  // true for commands added via registerSkills (shown in custom-commands tab)
	Execute     func(m *model, args string) (tea.Model, tea.Cmd) // returns updated model and command
}

// textCommand wraps a simple function that returns display text into a full
// SlashCommand Execute handler. Use this for commands that only need to print
// output without changing mode or running async work.
func textCommand(fn func(m *model) string) func(m *model, args string) (tea.Model, tea.Cmd) {
	return func(m *model, args string) (tea.Model, tea.Cmd) {
		output := fn(m)
		return *m, tea.Println(output)
	}
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

	registerClearCommand(r)
	registerResumeCommand(r)
	registerContinueCommand(r)
	registerLoginCommand(r)
	registerLogoutCommand(r)
	registerVersionCommand(r)
	registerCostCommand(r)
	registerContextCommand(r)
	registerMCPCommand(r)
	registerFastCommand(r)
	registerHelpCommand(r)
	registerConfigCommand(r)
	registerModelCommand(r)
	registerDiffCommand(r)
	registerMemoryCommand(r)
	registerCompactCommand(r)
	registerInitCommand(r)
	registerReviewCommand(r)
	registerExitCommands(r)

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

// fuzzyComplete returns command names that fuzzy-match the given input,
// ordered by match quality (best first). Prefix matches are tried first;
// if none are found, fuzzy subsequence matching is used.
func (r *slashRegistry) fuzzyComplete(input string) []string {
	// Try exact prefix matches first.
	if prefixMatches := r.complete(input); len(prefixMatches) > 0 {
		return prefixMatches
	}

	// Fall back to fuzzy matching.
	ranked := fuzzyRankCandidates(input, r.names)
	result := make([]string, len(ranked))
	for i, r := range ranked {
		result[i] = r.Name
	}
	return result
}

// fuzzyBest returns the single best fuzzy match for input, or ("", false)
// if nothing matches. Used for auto-correcting misspelled commands on Enter.
func (r *slashRegistry) fuzzyBest(input string) (string, bool) {
	// Exact match is always best.
	if _, ok := r.commands[input]; ok {
		return input, true
	}

	matches := r.fuzzyComplete(input)
	if len(matches) > 0 {
		return matches[0], true
	}
	return "", false
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
			IsSkill:     true,
			Execute: func(m *model, args string) (tea.Model, tea.Cmd) {
				return sendToLoop(m, skillContent)
			},
		})
	}
}

// visibleCommands returns all commands that should be shown in the help screen,
// filtering out aliases and hidden commands. Commands are returned in sorted order.
func (r *slashRegistry) visibleCommands() []SlashCommand {
	var cmds []SlashCommand
	for _, name := range r.names {
		cmd := r.commands[name]
		if cmd.IsAlias || cmd.IsHidden {
			continue
		}
		cmds = append(cmds, cmd)
	}
	return cmds
}

// helpText returns formatted help output.
func (r *slashRegistry) helpText() string {
	var b strings.Builder
	b.WriteString("Available commands:\n")
	for _, cmd := range r.visibleCommands() {
		b.WriteString(fmt.Sprintf("  /%-12s %s\n", cmd.Name, cmd.Description))
	}
	return strings.TrimRight(b.String(), "\n")
}

// sendToLoop switches to streaming mode and sends a prompt to the agentic loop.
// This is a shared helper for commands that delegate to the conversation loop.
func sendToLoop(m *model, prompt string) (tea.Model, tea.Cmd) {
	m.mode = modeStreaming
	m.textInput.Blur()
	loopCmd := func() tea.Msg {
		err := m.loop.SendMessage(m.ctx, prompt)
		return LoopDoneMsg{Err: err}
	}
	return *m, tea.Batch(loopCmd, m.spinner.Tick)
}
