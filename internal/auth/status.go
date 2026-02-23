package auth

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

// AuthMethod describes how the user is authenticated.
type AuthMethod string

const (
	AuthMethodNone       AuthMethod = "none"
	AuthMethodClaudeAI   AuthMethod = "claude.ai"
	AuthMethodAPIKey     AuthMethod = "api_key"
	AuthMethodOAuthToken AuthMethod = "oauth_token"
	AuthMethodThirdParty AuthMethod = "third_party"
)

// APIProvider describes which API backend is in use.
type APIProvider string

const (
	APIProviderFirstParty APIProvider = "firstParty"
	APIProviderBedrock    APIProvider = "bedrock"
	APIProviderVertex     APIProvider = "vertex"
	APIProviderFoundry    APIProvider = "foundry"
)

// AuthStatus holds the authentication status information returned by the
// status command.
type AuthStatus struct {
	LoggedIn         bool        `json:"loggedIn"`
	AuthMethod       AuthMethod  `json:"authMethod"`
	APIProvider      APIProvider `json:"apiProvider"`
	APIKeySource     string      `json:"apiKeySource,omitempty"`
	Email            *string     `json:"email"`
	OrgID            *string     `json:"orgId"`
	OrgName          *string     `json:"orgName"`
	SubscriptionType *string     `json:"subscriptionType"`
}

// subscriptionDisplayName returns a human-readable label for subscription types.
func subscriptionDisplayName(subType string) string {
	switch strings.ToLower(subType) {
	case "enterprise":
		return "Claude Enterprise"
	case "team":
		return "Claude Team"
	case "max":
		return "Claude Max"
	case "pro":
		return "Claude Pro"
	default:
		return "Claude API"
	}
}

// GetAuthStatus inspects the local authentication state (environment variables
// and stored credentials) and returns an AuthStatus. No network calls are made.
func GetAuthStatus(store *CredentialStore) *AuthStatus {
	status := &AuthStatus{
		APIProvider: detectAPIProvider(),
	}

	// Check for third-party providers first.
	if isThirdPartyProvider() {
		status.LoggedIn = true
		status.AuthMethod = AuthMethodThirdParty
		return status
	}

	// Check CLAUDE_CODE_OAUTH_TOKEN env var.
	if envToken := os.Getenv("CLAUDE_CODE_OAUTH_TOKEN"); envToken != "" {
		status.LoggedIn = true
		status.AuthMethod = AuthMethodOAuthToken
		status.APIKeySource = "CLAUDE_CODE_OAUTH_TOKEN"
		return status
	}

	// Check ANTHROPIC_API_KEY env var.
	if apiKey := os.Getenv("ANTHROPIC_API_KEY"); apiKey != "" {
		status.LoggedIn = true
		status.AuthMethod = AuthMethodAPIKey
		status.APIKeySource = "ANTHROPIC_API_KEY"
		return status
	}

	// Check stored claude.ai OAuth credentials.
	if store != nil {
		tokens, err := store.Load()
		if err == nil && tokens != nil && tokens.AccessToken != "" {
			status.LoggedIn = true
			status.AuthMethod = AuthMethodClaudeAI

			// Populate subscription type from stored token metadata.
			if tokens.SubscriptionType != "" {
				subDisplay := subscriptionDisplayName(tokens.SubscriptionType)
				status.SubscriptionType = &subDisplay
			}

			// Load account metadata for email, org.
			account, err := store.LoadAccount()
			if err == nil && account != nil {
				if account.EmailAddress != "" {
					status.Email = &account.EmailAddress
				}
				if account.OrganizationUUID != "" {
					status.OrgID = &account.OrganizationUUID
				}
				if account.OrganizationName != "" {
					status.OrgName = &account.OrganizationName
				}
			}

			return status
		}
	}

	// Not authenticated.
	status.AuthMethod = AuthMethodNone
	return status
}

// FormatStatusJSON returns the status as indented JSON.
func FormatStatusJSON(status *AuthStatus) (string, error) {
	data, err := json.MarshalIndent(status, "", "  ")
	if err != nil {
		return "", fmt.Errorf("marshaling status: %w", err)
	}
	return string(data), nil
}

// FormatStatusText returns the status as human-readable text lines.
func FormatStatusText(status *AuthStatus) string {
	if !status.LoggedIn {
		return "Not logged in. Run claude login to authenticate."
	}

	var lines []string

	switch status.AuthMethod {
	case AuthMethodClaudeAI:
		label := "Claude Account"
		if status.SubscriptionType != nil {
			label = *status.SubscriptionType + " Account"
		}
		lines = append(lines, fmt.Sprintf("Login method: %s", label))
	case AuthMethodOAuthToken:
		lines = append(lines, "Login method: OAuth Token")
		if status.APIKeySource != "" {
			lines = append(lines, fmt.Sprintf("Auth token source: %s", status.APIKeySource))
		}
	case AuthMethodAPIKey:
		lines = append(lines, "Login method: API Key")
		if status.APIKeySource != "" {
			lines = append(lines, fmt.Sprintf("API key source: %s", status.APIKeySource))
		}
	case AuthMethodThirdParty:
		lines = append(lines, "Login method: Third-Party Provider")
	default:
		lines = append(lines, "Login method: Unknown")
	}

	if status.OrgName != nil {
		lines = append(lines, fmt.Sprintf("Organization: %s", *status.OrgName))
	}

	if status.Email != nil {
		lines = append(lines, fmt.Sprintf("Email: %s", *status.Email))
	}

	if status.APIProvider != APIProviderFirstParty {
		lines = append(lines, fmt.Sprintf("API provider: %s", providerDisplayName(status.APIProvider)))
	}

	return strings.Join(lines, "\n")
}

// detectAPIProvider returns the API provider based on environment variables.
func detectAPIProvider() APIProvider {
	if os.Getenv("CLAUDE_CODE_USE_BEDROCK") != "" {
		return APIProviderBedrock
	}
	if os.Getenv("CLAUDE_CODE_USE_VERTEX") != "" {
		return APIProviderVertex
	}
	if os.Getenv("CLAUDE_CODE_USE_FOUNDRY") != "" {
		return APIProviderFoundry
	}
	return APIProviderFirstParty
}

// isThirdPartyProvider returns true if a third-party API provider is configured.
func isThirdPartyProvider() bool {
	return os.Getenv("CLAUDE_CODE_USE_BEDROCK") != "" ||
		os.Getenv("CLAUDE_CODE_USE_VERTEX") != "" ||
		os.Getenv("CLAUDE_CODE_USE_FOUNDRY") != ""
}

// providerDisplayName returns a human-readable name for an API provider.
func providerDisplayName(p APIProvider) string {
	switch p {
	case APIProviderBedrock:
		return "AWS Bedrock"
	case APIProviderVertex:
		return "Google Vertex AI"
	case APIProviderFoundry:
		return "Microsoft Foundry"
	default:
		return "Anthropic"
	}
}
