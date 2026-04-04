package auth

import "strings"

type Config struct {
	FrontendURL         string
	PasswordAuthEnabled bool
	SignUpEnabled       bool
	OIDC                OIDCConfig
}

type OIDCConfig struct {
	Enabled             bool
	ProviderID          string
	ProviderName        string
	IssuerURL           string
	ClientID            string
	ClientSecret        string
	RedirectURL         string
	FrontendCallbackURL string
	Scopes              []string
	PKCEEnabled         bool
	StateCookieName     string
	StateSecret         []byte
	EmailClaim          string
	NameClaim           string
	GroupsClaim         string
	AdminGroups         []string
}

func (c Config) OIDCEnabled() bool {
	return c.OIDC.Enabled && c.OIDC.IsConfigured()
}

func (c OIDCConfig) IsConfigured() bool {
	return strings.TrimSpace(c.IssuerURL) != "" &&
		strings.TrimSpace(c.ClientID) != "" &&
		strings.TrimSpace(c.RedirectURL) != "" &&
		len(c.StateSecret) > 0
}

func (c OIDCConfig) NormalizedProviderID() string {
	value := strings.TrimSpace(c.ProviderID)
	if value == "" {
		return "oidc"
	}
	return value
}

func (c OIDCConfig) NormalizedProviderName() string {
	value := strings.TrimSpace(c.ProviderName)
	if value == "" {
		return "Single Sign-On"
	}
	return value
}

func (c OIDCConfig) NormalizedStateCookieName() string {
	value := strings.TrimSpace(c.StateCookieName)
	if value == "" {
		return "babelsuite_oidc_state"
	}
	return value
}

func (c OIDCConfig) NormalizedScopes() []string {
	if len(c.Scopes) == 0 {
		return []string{"openid", "profile", "email", "groups"}
	}

	scopes := make([]string, 0, len(c.Scopes))
	for _, scope := range c.Scopes {
		scope = strings.TrimSpace(scope)
		if scope == "" {
			continue
		}
		scopes = append(scopes, scope)
	}
	if len(scopes) == 0 {
		return []string{"openid", "profile", "email", "groups"}
	}
	return scopes
}

func (c OIDCConfig) NormalizedEmailClaim() string {
	value := strings.TrimSpace(c.EmailClaim)
	if value == "" {
		return "email"
	}
	return value
}

func (c OIDCConfig) NormalizedNameClaim() string {
	value := strings.TrimSpace(c.NameClaim)
	if value == "" {
		return "name"
	}
	return value
}

func (c OIDCConfig) NormalizedGroupsClaim() string {
	value := strings.TrimSpace(c.GroupsClaim)
	if value == "" {
		return "groups"
	}
	return value
}
