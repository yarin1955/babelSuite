package sso

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	gooidc "github.com/coreos/go-oidc/v3/oidc"
	"github.com/babelsuite/babelsuite/internal/auth"
	"github.com/babelsuite/babelsuite/internal/domain"
	"github.com/babelsuite/babelsuite/internal/store"
	"github.com/google/uuid"
	"golang.org/x/oauth2"
)

type Handler struct {
	store      store.Store
	jwt        *auth.JWTService
	states     *stateStore
	frontendURL string
}

func NewHandler(s store.Store, jwt *auth.JWTService, frontendURL string) *Handler {
	return &Handler{
		store:       s,
		jwt:         jwt,
		states:      newStateStore(),
		frontendURL: frontendURL,
	}
}

func (h *Handler) Register(mux *http.ServeMux) {
	// Public
	mux.HandleFunc("GET /auth/sso/providers", h.listPublicProviders)
	mux.HandleFunc("GET /auth/sso/login", h.initiateLogin)
	mux.HandleFunc("GET /auth/sso/callback", h.handleCallback)

	// Admin
	mux.HandleFunc("GET /api/admin/sso/providers", h.adminMiddleware(h.adminList))
	mux.HandleFunc("POST /api/admin/sso/providers", h.adminMiddleware(h.adminCreate))
	mux.HandleFunc("PUT /api/admin/sso/providers/{id}", h.adminMiddleware(h.adminUpdate))
	mux.HandleFunc("DELETE /api/admin/sso/providers/{id}", h.adminMiddleware(h.adminDelete))
}

// ── public ────────────────────────────────────────────────────────────────────

func (h *Handler) listPublicProviders(w http.ResponseWriter, r *http.Request) {
	providers, err := h.store.ListOIDCProviders(r.Context())
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "internal error")
		return
	}
	type publicProvider struct {
		ProviderID string `json:"provider_id"`
		Name       string `json:"name"`
	}
	var list []publicProvider
	for _, p := range providers {
		if p.Enabled {
			list = append(list, publicProvider{ProviderID: p.ProviderID, Name: p.Name})
		}
	}
	if list == nil {
		list = []publicProvider{}
	}
	writeJSON(w, http.StatusOK, list)
}

func (h *Handler) initiateLogin(w http.ResponseWriter, r *http.Request) {
	providerID := r.URL.Query().Get("provider_id")
	returnURL := r.URL.Query().Get("return_url")
	if returnURL == "" {
		returnURL = "/"
	}
	if providerID == "" {
		writeErr(w, http.StatusBadRequest, "provider_id required")
		return
	}

	p, err := h.store.GetOIDCProvider(r.Context(), providerID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeErr(w, http.StatusNotFound, "provider not found")
		} else {
			writeErr(w, http.StatusInternalServerError, "internal error")
		}
		return
	}
	if !p.Enabled {
		writeErr(w, http.StatusForbidden, "provider disabled")
		return
	}

	oauth2Cfg, err := h.oauth2Config(r.Context(), p, r)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "failed to configure provider")
		return
	}

	nonce := randomState()
	h.states.put(nonce, p.ProviderID, returnURL)

	authURL := oauth2Cfg.AuthCodeURL(nonce, oauth2.AccessTypeOnline)
	http.Redirect(w, r, authURL, http.StatusFound)
}

func (h *Handler) handleCallback(w http.ResponseWriter, r *http.Request) {
	state := r.URL.Query().Get("state")
	code := r.URL.Query().Get("code")
	errParam := r.URL.Query().Get("error")

	if errParam != "" {
		http.Redirect(w, r, h.frontendURL+"/login?sso_error="+errParam, http.StatusFound)
		return
	}

	entry, ok := h.states.pop(state)
	if !ok {
		writeErr(w, http.StatusBadRequest, "invalid or expired state")
		return
	}

	p, err := h.store.GetOIDCProvider(r.Context(), entry.providerID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "internal error")
		return
	}

	oauth2Cfg, err := h.oauth2Config(r.Context(), p, r)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "failed to configure provider")
		return
	}

	token, err := oauth2Cfg.Exchange(r.Context(), code)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "token exchange failed")
		return
	}

	rawIDToken, ok := token.Extra("id_token").(string)
	if !ok {
		writeErr(w, http.StatusBadRequest, "id_token missing")
		return
	}

	oidcProvider, err := gooidc.NewProvider(r.Context(), p.IssuerURL)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "provider discovery failed")
		return
	}
	verifier := oidcProvider.Verifier(&gooidc.Config{ClientID: p.ClientID})
	idToken, err := verifier.Verify(r.Context(), rawIDToken)
	if err != nil {
		writeErr(w, http.StatusUnauthorized, "id_token verification failed")
		return
	}

	var claims struct {
		Sub   string `json:"sub"`
		Email string `json:"email"`
		Name  string `json:"name"`
	}
	if err := idToken.Claims(&claims); err != nil {
		writeErr(w, http.StatusInternalServerError, "failed to parse claims")
		return
	}

	email := strings.ToLower(strings.TrimSpace(claims.Email))
	if email == "" {
		writeErr(w, http.StatusBadRequest, "email claim missing from id_token")
		return
	}

	name := claims.Name
	if name == "" {
		name = email
	}
	username := usernameFromEmail(email)

	// auto-create org if this will be a new user
	org := &domain.Org{
		OrgID:     uuid.NewString(),
		Slug:      username + "-" + uuid.NewString()[:6],
		Name:      name + "'s workspace",
		CreatedAt: time.Now().UTC(),
	}
	_ = h.store.CreateOrg(r.Context(), org)

	// get or create the org by trying to find the user first
	existing, _ := h.store.GetUserByEmail(r.Context(), email)
	var orgID string
	if existing != nil {
		orgID = existing.OrgID
	} else {
		orgID = org.OrgID
	}

	user := &domain.User{
		UserID:    uuid.NewString(),
		OrgID:     orgID,
		Username:  username,
		Email:     email,
		Name:      name,
		PassHash:  "",
		CreatedAt: time.Now().UTC(),
	}
	user, err = h.store.UpsertUserByEmail(r.Context(), user)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "failed to provision user")
		return
	}

	jwtToken, err := h.jwt.Sign(user.UserID, user.OrgID, user.IsAdmin)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "internal error")
		return
	}

	returnURL := entry.returnURL
	if returnURL == "" || !strings.HasPrefix(returnURL, "/") {
		returnURL = "/"
	}
	redirect := fmt.Sprintf("%s/auth/callback#token=%s&return_url=%s", h.frontendURL, jwtToken, returnURL)
	http.Redirect(w, r, redirect, http.StatusFound)
}

// ── admin ─────────────────────────────────────────────────────────────────────

func (h *Handler) adminList(w http.ResponseWriter, r *http.Request) {
	providers, err := h.store.ListOIDCProviders(r.Context())
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "internal error")
		return
	}
	if providers == nil {
		providers = []*domain.OIDCProvider{}
	}
	writeJSON(w, http.StatusOK, providers)
}

func (h *Handler) adminCreate(w http.ResponseWriter, r *http.Request) {
	var req domain.OIDCProvider
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	if req.Name == "" || req.IssuerURL == "" || req.ClientID == "" || req.ClientSecret == "" {
		writeErr(w, http.StatusBadRequest, "name, issuer_url, client_id and client_secret are required")
		return
	}
	req.ProviderID = uuid.NewString()
	req.CreatedAt = time.Now().UTC()
	if len(req.Scopes) == 0 {
		req.Scopes = []string{"email", "profile"}
	}
	req.Enabled = true
	if err := h.store.CreateOIDCProvider(r.Context(), &req); err != nil {
		writeErr(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusCreated, req)
}

func (h *Handler) adminUpdate(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	existing, err := h.store.GetOIDCProvider(r.Context(), id)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeErr(w, http.StatusNotFound, "not found")
		} else {
			writeErr(w, http.StatusInternalServerError, "internal error")
		}
		return
	}
	var req domain.OIDCProvider
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	existing.Name = req.Name
	existing.IssuerURL = req.IssuerURL
	existing.ClientID = req.ClientID
	if req.ClientSecret != "" {
		existing.ClientSecret = req.ClientSecret
	}
	existing.Scopes = req.Scopes
	existing.Enabled = req.Enabled
	if err := h.store.UpdateOIDCProvider(r.Context(), existing); err != nil {
		writeErr(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, existing)
}

func (h *Handler) adminDelete(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := h.store.DeleteOIDCProvider(r.Context(), id); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeErr(w, http.StatusNotFound, "not found")
		} else {
			writeErr(w, http.StatusInternalServerError, "internal error")
		}
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ── middleware ────────────────────────────────────────────────────────────────

func (h *Handler) adminMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		bearer := r.Header.Get("Authorization")
		if !strings.HasPrefix(bearer, "Bearer ") {
			writeErr(w, http.StatusUnauthorized, "unauthorized")
			return
		}
		claims, err := h.jwt.Verify(strings.TrimPrefix(bearer, "Bearer "))
		if err != nil || !claims.IsAdmin {
			writeErr(w, http.StatusForbidden, "admin required")
			return
		}
		next(w, r)
	}
}

// ── helpers ───────────────────────────────────────────────────────────────────

func (h *Handler) oauth2Config(ctx context.Context, p *domain.OIDCProvider, r *http.Request) (*oauth2.Config, error) {
	oidcProvider, err := gooidc.NewProvider(ctx, p.IssuerURL)
	if err != nil {
		return nil, err
	}
	scopes := []string{gooidc.ScopeOpenID}
	scopes = append(scopes, p.Scopes...)

	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	callbackURL := fmt.Sprintf("%s://%s/auth/sso/callback", scheme, r.Host)

	return &oauth2.Config{
		ClientID:     p.ClientID,
		ClientSecret: p.ClientSecret,
		Endpoint:     oidcProvider.Endpoint(),
		RedirectURL:  callbackURL,
		Scopes:       scopes,
	}, nil
}

func randomState() string {
	b := make([]byte, 16)
	rand.Read(b)
	return base64.RawURLEncoding.EncodeToString(b)
}

func usernameFromEmail(email string) string {
	parts := strings.SplitN(email, "@", 2)
	username := parts[0]
	username = strings.Map(func(r rune) rune {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' {
			return r
		}
		if r >= 'A' && r <= 'Z' {
			return r + 32
		}
		return '-'
	}, username)
	if len(username) > 30 {
		username = username[:30]
	}
	return username
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}
