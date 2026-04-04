package auth

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	gooidc "github.com/coreos/go-oidc/v3/oidc"
	"golang.org/x/oauth2"
)

var (
	errOIDCDisabled      = errors.New("oidc is disabled")
	errInvalidOIDCState  = errors.New("invalid oidc state")
	errMissingOIDCCookie = errors.New("missing oidc state cookie")
)

const oidcStateTTL = 10 * time.Minute

type oidcProviderConfig struct {
	oauthConfig *oauth2.Config
	provider    *gooidc.Provider
	verifier    *gooidc.IDTokenVerifier
}

type oidcState struct {
	State        string    `json:"state"`
	CodeVerifier string    `json:"codeVerifier"`
	ReturnURL    string    `json:"returnUrl"`
	ExpiresAt    time.Time `json:"expiresAt"`
}

type oidcIdentity struct {
	Subject  string
	Email    string
	FullName string
	Groups   []string
}

type OIDCService struct {
	config OIDCConfig
	client *http.Client

	mu       sync.RWMutex
	resolved *oidcProviderConfig
}

func NewOIDCService(config OIDCConfig) *OIDCService {
	return &OIDCService{
		config: config,
		client: http.DefaultClient,
	}
}

func (s *OIDCService) Enabled() bool {
	return s != nil && s.config.Enabled && s.config.IsConfigured()
}

func (s *OIDCService) Provider(baseURL string) SSOProvider {
	name := s.config.NormalizedProviderName()
	provider := SSOProvider{
		ProviderID:  s.config.NormalizedProviderID(),
		Name:        name,
		ButtonLabel: "Continue with " + name,
		Enabled:     s.Enabled(),
	}
	if provider.Enabled {
		provider.StartURL = strings.TrimRight(baseURL, "/") + "/api/v1/auth/oidc/login"
		return provider
	}

	provider.Hint = "Set OIDC issuer, client, redirect, and state settings on the backend to enable this SSO path."
	return provider
}

func (s *OIDCService) BeginLogin(ctx context.Context, returnURL string, secureCookie bool) (string, *http.Cookie, error) {
	if !s.Enabled() {
		return "", nil, errOIDCDisabled
	}

	provider, err := s.providerConfig(ctx)
	if err != nil {
		return "", nil, err
	}

	state, cookie, err := s.newStateCookie(returnURL, secureCookie)
	if err != nil {
		return "", nil, err
	}

	options := []oauth2.AuthCodeOption{}
	if s.config.PKCEEnabled {
		options = append(options,
			oauth2.SetAuthURLParam("code_challenge_method", "S256"),
			oauth2.SetAuthURLParam("code_challenge", pkceChallenge(state.CodeVerifier)),
		)
	}

	return provider.oauthConfig.AuthCodeURL(state.State, options...), cookie, nil
}

func (s *OIDCService) Exchange(ctx context.Context, requestState string, code string, stateCookie *http.Cookie) (*oidcIdentity, string, error) {
	if !s.Enabled() {
		return nil, "", errOIDCDisabled
	}

	state, err := s.readStateCookie(stateCookie)
	if err != nil {
		return nil, "", err
	}
	if time.Now().UTC().After(state.ExpiresAt) {
		return nil, "", errInvalidOIDCState
	}
	if subtleCompare(state.State, requestState) == false {
		return nil, "", errInvalidOIDCState
	}

	provider, err := s.providerConfig(ctx)
	if err != nil {
		return nil, "", err
	}

	options := []oauth2.AuthCodeOption{}
	if s.config.PKCEEnabled {
		options = append(options, oauth2.SetAuthURLParam("code_verifier", state.CodeVerifier))
	}

	token, err := provider.oauthConfig.Exchange(ctx, code, options...)
	if err != nil {
		return nil, "", err
	}

	rawIDToken, _ := token.Extra("id_token").(string)
	if strings.TrimSpace(rawIDToken) == "" {
		return nil, "", errors.New("oidc response did not include an id token")
	}

	idToken, err := provider.verifier.Verify(ctx, rawIDToken)
	if err != nil {
		return nil, "", err
	}

	identity, err := s.identityFromToken(ctx, provider.provider, token, idToken)
	if err != nil {
		return nil, "", err
	}

	return identity, state.ReturnURL, nil
}

func (s *OIDCService) ClearStateCookie(secureCookie bool) *http.Cookie {
	return &http.Cookie{
		Name:     s.config.NormalizedStateCookieName(),
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		MaxAge:   -1,
		SameSite: http.SameSiteLaxMode,
		Secure:   secureCookie,
	}
}

func (s *OIDCService) MapsGroupsToAdmin(groups []string) bool {
	if len(groups) == 0 || len(s.config.AdminGroups) == 0 {
		return false
	}

	expected := make(map[string]struct{}, len(s.config.AdminGroups))
	for _, group := range s.config.AdminGroups {
		group = strings.ToLower(strings.TrimSpace(group))
		if group == "" {
			continue
		}
		expected[group] = struct{}{}
	}
	for _, group := range groups {
		if _, ok := expected[strings.ToLower(strings.TrimSpace(group))]; ok {
			return true
		}
	}
	return false
}

func (s *OIDCService) providerConfig(ctx context.Context) (*oidcProviderConfig, error) {
	s.mu.RLock()
	if s.resolved != nil {
		defer s.mu.RUnlock()
		return s.resolved, nil
	}
	s.mu.RUnlock()

	s.mu.Lock()
	defer s.mu.Unlock()
	if s.resolved != nil {
		return s.resolved, nil
	}

	discoveryCtx := context.WithValue(ctx, oauth2.HTTPClient, s.client)
	provider, err := gooidc.NewProvider(discoveryCtx, s.config.IssuerURL)
	if err != nil {
		return nil, err
	}

	resolved := &oidcProviderConfig{
		oauthConfig: &oauth2.Config{
			ClientID:     s.config.ClientID,
			ClientSecret: s.config.ClientSecret,
			Endpoint:     provider.Endpoint(),
			RedirectURL:  s.config.RedirectURL,
			Scopes:       s.config.NormalizedScopes(),
		},
		provider: provider,
		verifier: provider.Verifier(&gooidc.Config{ClientID: s.config.ClientID}),
	}
	s.resolved = resolved
	return resolved, nil
}

func (s *OIDCService) identityFromToken(
	ctx context.Context,
	provider *gooidc.Provider,
	token *oauth2.Token,
	idToken *gooidc.IDToken,
) (*oidcIdentity, error) {
	claims := map[string]any{}
	if err := idToken.Claims(&claims); err != nil {
		return nil, err
	}

	identity := s.identityFromClaims(claims)
	if accessToken := strings.TrimSpace(token.AccessToken); accessToken != "" {
		if identity.Email == "" || identity.FullName == "" || len(identity.Groups) == 0 {
			userInfo, err := provider.UserInfo(context.WithValue(ctx, oauth2.HTTPClient, s.client), oauth2.StaticTokenSource(token))
			if err == nil {
				extra := map[string]any{}
				if claimsErr := userInfo.Claims(&extra); claimsErr == nil {
					identity = mergeOIDCIdentity(identity, s.identityFromClaims(extra))
				}
				if identity.Email == "" {
					identity.Email = strings.TrimSpace(userInfo.Email)
				}
			}
		}
	}

	identity.Subject = strings.TrimSpace(idToken.Subject)
	if identity.Email == "" {
		return nil, errors.New("oidc identity did not include an email claim")
	}
	if identity.FullName == "" {
		identity.FullName = firstNonEmpty(identity.Email, "Member")
	}

	return &identity, nil
}

func (s *OIDCService) identityFromClaims(claims map[string]any) oidcIdentity {
	identity := oidcIdentity{
		Email:    extractStringClaim(claims, s.config.NormalizedEmailClaim(), "email"),
		FullName: extractStringClaim(claims, s.config.NormalizedNameClaim(), "name", "preferred_username"),
		Groups:   extractGroupsClaim(claims, s.config.NormalizedGroupsClaim(), "groups"),
	}
	return identity
}

func (s *OIDCService) newStateCookie(returnURL string, secureCookie bool) (*oidcState, *http.Cookie, error) {
	stateValue, err := randomURLToken(32)
	if err != nil {
		return nil, nil, err
	}
	verifier, err := randomURLToken(32)
	if err != nil {
		return nil, nil, err
	}

	state := &oidcState{
		State:        stateValue,
		CodeVerifier: verifier,
		ReturnURL:    returnURL,
		ExpiresAt:    time.Now().UTC().Add(oidcStateTTL),
	}

	value, err := s.signStateCookie(state)
	if err != nil {
		return nil, nil, err
	}

	cookie := &http.Cookie{
		Name:     s.config.NormalizedStateCookieName(),
		Value:    value,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   secureCookie,
		Expires:  state.ExpiresAt,
		MaxAge:   int(oidcStateTTL.Seconds()),
	}

	return state, cookie, nil
}

func (s *OIDCService) signStateCookie(state *oidcState) (string, error) {
	payload, err := json.Marshal(state)
	if err != nil {
		return "", err
	}
	encodedPayload := base64.RawURLEncoding.EncodeToString(payload)
	signature := signWithHMAC([]byte(encodedPayload), s.config.StateSecret)
	return encodedPayload + "." + base64.RawURLEncoding.EncodeToString(signature), nil
}

func (s *OIDCService) readStateCookie(cookie *http.Cookie) (*oidcState, error) {
	if cookie == nil || strings.TrimSpace(cookie.Value) == "" {
		return nil, errMissingOIDCCookie
	}

	parts := strings.Split(cookie.Value, ".")
	if len(parts) != 2 {
		return nil, errInvalidOIDCState
	}

	signature, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return nil, errInvalidOIDCState
	}
	if subtleCompareBytes(signature, signWithHMAC([]byte(parts[0]), s.config.StateSecret)) == false {
		return nil, errInvalidOIDCState
	}

	payload, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return nil, errInvalidOIDCState
	}

	var state oidcState
	if err := json.Unmarshal(payload, &state); err != nil {
		return nil, errInvalidOIDCState
	}
	return &state, nil
}

func mergeOIDCIdentity(primary oidcIdentity, fallback oidcIdentity) oidcIdentity {
	if primary.Email == "" {
		primary.Email = fallback.Email
	}
	if primary.FullName == "" {
		primary.FullName = fallback.FullName
	}
	if len(primary.Groups) == 0 {
		primary.Groups = fallback.Groups
	}
	return primary
}

func extractStringClaim(claims map[string]any, keys ...string) string {
	for _, key := range keys {
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}
		raw, ok := claims[key]
		if !ok {
			continue
		}
		if value, ok := raw.(string); ok && strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func extractGroupsClaim(claims map[string]any, keys ...string) []string {
	for _, key := range keys {
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}
		raw, ok := claims[key]
		if !ok {
			continue
		}
		switch value := raw.(type) {
		case []string:
			return normalizeGroups(value)
		case []any:
			groups := make([]string, 0, len(value))
			for _, item := range value {
				text, ok := item.(string)
				if !ok {
					continue
				}
				groups = append(groups, text)
			}
			return normalizeGroups(groups)
		case string:
			return normalizeGroups(strings.Split(value, ","))
		}
	}
	return nil
}

func normalizeGroups(groups []string) []string {
	if len(groups) == 0 {
		return nil
	}
	normalized := make([]string, 0, len(groups))
	for _, group := range groups {
		group = strings.TrimSpace(group)
		if group == "" {
			continue
		}
		normalized = append(normalized, group)
	}
	if len(normalized) == 0 {
		return nil
	}
	return normalized
}

func sanitizeReturnURL(frontendURL, candidate string) string {
	candidate = strings.TrimSpace(candidate)
	if candidate == "" {
		return "/"
	}
	if strings.HasPrefix(candidate, "/") && !strings.HasPrefix(candidate, "//") {
		return candidate
	}

	base, err := url.Parse(frontendURL)
	if err != nil {
		return "/"
	}
	target, err := url.Parse(candidate)
	if err != nil {
		return "/"
	}
	if !target.IsAbs() || !strings.EqualFold(base.Scheme, target.Scheme) || !strings.EqualFold(base.Host, target.Host) {
		return "/"
	}

	result := target.EscapedPath()
	if result == "" {
		result = "/"
	}
	if target.RawQuery != "" {
		result += "?" + target.RawQuery
	}
	return result
}

func appendFragmentParams(base string, values map[string]string) string {
	if strings.TrimSpace(base) == "" {
		return ""
	}

	parsed, err := url.Parse(base)
	if err != nil {
		return base
	}

	params := url.Values{}
	if parsed.Fragment != "" {
		if existing, parseErr := url.ParseQuery(parsed.Fragment); parseErr == nil {
			params = existing
		}
	}
	for key, value := range values {
		if strings.TrimSpace(value) == "" {
			continue
		}
		params.Set(key, value)
	}
	parsed.Fragment = params.Encode()
	return parsed.String()
}

func pkceChallenge(verifier string) string {
	sum := sha256.Sum256([]byte(verifier))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}

func signWithHMAC(payload []byte, secret []byte) []byte {
	signer := hmac.New(sha256.New, secret)
	_, _ = signer.Write(payload)
	return signer.Sum(nil)
}

func subtleCompare(left, right string) bool {
	return subtleCompareBytes([]byte(left), []byte(right))
}

func subtleCompareBytes(left, right []byte) bool {
	return hmac.Equal(left, right)
}

func randomURLToken(size int) (string, error) {
	buf := make([]byte, size)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("random token: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}
