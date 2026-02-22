package config

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"

	"github.com/bmatcuk/doublestar/v4"
)

// PermissionHandler is the interface that permission handlers implement.
// This mirrors tools.PermissionHandler to avoid import cycles.
type PermissionHandler interface {
	RequestPermission(ctx context.Context, toolName string, input json.RawMessage) (bool, error)
}

// RuleBasedPermissionHandler evaluates permission rules from settings before
// falling back to a terminal prompt. Rules are evaluated in order; the first
// matching rule determines the action.
type RuleBasedPermissionHandler struct {
	rules    []PermissionRule
	fallback PermissionHandler
}

// NewRuleBasedPermissionHandler creates a handler that checks rules first,
// then falls back to the provided handler for unmatched tool calls.
func NewRuleBasedPermissionHandler(rules []PermissionRule, fallback PermissionHandler) *RuleBasedPermissionHandler {
	return &RuleBasedPermissionHandler{
		rules:    rules,
		fallback: fallback,
	}
}

// RequestPermission checks rules for a matching allow/deny before prompting.
func (h *RuleBasedPermissionHandler) RequestPermission(
	ctx context.Context, toolName string, input json.RawMessage,
) (bool, error) {
	action := h.matchRule(toolName, input)
	switch action {
	case "allow":
		return true, nil
	case "deny":
		return false, nil
	default: // "ask" or no match
		return h.fallback.RequestPermission(ctx, toolName, input)
	}
}

// matchRule finds the first rule that matches the tool call and returns its action.
// Returns "" if no rule matches.
func (h *RuleBasedPermissionHandler) matchRule(toolName string, input json.RawMessage) string {
	for _, rule := range h.rules {
		if ruleMatches(rule, toolName, input) {
			return rule.Action
		}
	}
	return ""
}

// ruleMatches checks if a single rule matches the given tool call.
func ruleMatches(rule PermissionRule, toolName string, input json.RawMessage) bool {
	// Tool name must match.
	if rule.Tool != toolName {
		return false
	}

	// If no pattern, matches all calls to this tool.
	if rule.Pattern == "" {
		return true
	}

	// Extract the relevant value from the tool input to match against the pattern.
	value := extractMatchValue(toolName, input, rule.Pattern)
	if value == "" {
		return false
	}

	return matchPattern(rule.Pattern, value)
}

// extractMatchValue gets the value from tool input that should be matched against the pattern.
func extractMatchValue(toolName string, input json.RawMessage, pattern string) string {
	var m map[string]json.RawMessage
	if err := json.Unmarshal(input, &m); err != nil {
		return ""
	}

	getString := func(key string) string {
		v, ok := m[key]
		if !ok {
			return ""
		}
		var s string
		json.Unmarshal(v, &s)
		return s
	}

	switch toolName {
	case "Bash":
		return getString("command")
	case "FileRead", "FileEdit", "FileWrite":
		return getString("file_path")
	case "WebFetch":
		// Pattern like "domain:example.com" matches against URL domain.
		if strings.HasPrefix(pattern, "domain:") {
			return getString("url")
		}
		return getString("url")
	case "Glob", "Grep":
		return getString("path")
	default:
		return ""
	}
}

// matchPattern matches a value against a glob-like permission pattern.
// Supports:
//   - Simple glob patterns: "npm run *", "*.env"
//   - Domain patterns: "domain:example.com" (for WebFetch)
//   - Path patterns: "./.env", "src/**/*.go"
func matchPattern(pattern, value string) bool {
	// Handle domain: prefix for WebFetch.
	if strings.HasPrefix(pattern, "domain:") {
		domain := strings.TrimPrefix(pattern, "domain:")
		return strings.Contains(value, domain)
	}

	// Try doublestar glob matching first (supports **).
	matched, err := doublestar.Match(pattern, value)
	if err == nil && matched {
		return true
	}

	// For Bash commands, also try prefix matching with glob.
	// "npm run *" should match "npm run test" etc.
	// filepath.Match doesn't support spaces in patterns well,
	// so we use doublestar which handles this better.
	matched, err = doublestar.Match(pattern, filepath.Base(value))
	if err == nil && matched {
		return true
	}

	return false
}
