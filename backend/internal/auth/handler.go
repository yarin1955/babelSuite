package auth

import (
	"encoding/json"
	"errors"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/babelsuite/babelsuite/internal/domain"
	"github.com/babelsuite/babelsuite/internal/store"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
)

var (
	emailRE     = regexp.MustCompile(`^[^\s@]+@[^\s@]+\.[^\s@]+$`)
	slugStripRE = regexp.MustCompile(`[^a-z0-9]+`)
	spacesRE    = regexp.MustCompile(`\s+`)
)

type SSOProvider struct {
	ProviderID  string `json:"providerId"`
	Name        string `json:"name"`
	ButtonLabel string `json:"buttonLabel"`
	StartURL    string `json:"startUrl,omitempty"`
	Enabled     bool   `json:"enabled"`
	Hint        string `json:"hint,omitempty"`
}

type Handler struct {
	store        store.Store
	jwt          *JWTService
	ssoProviders []SSOProvider
}

type signUpRequest struct {
	FullName string `json:"fullName"`
	Email    string `json:"email"`
	Password string `json:"password"`
}

type signInRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type authResponse struct {
	Token     string            `json:"token"`
	User      *domain.User      `json:"user"`
	Workspace *domain.Workspace `json:"workspace"`
	ExpiresAt time.Time         `json:"expiresAt"`
}

func NewHandler(st store.Store, jwt *JWTService, ssoProviders []SSOProvider) *Handler {
	return &Handler{store: st, jwt: jwt, ssoProviders: ssoProviders}
}

func DefaultSSOProviders(githubURL, gitlabURL string) []SSOProvider {
	return []SSOProvider{
		buildProvider("github", "GitHub", githubURL),
		buildProvider("gitlab", "GitLab", gitlabURL),
	}
}

func buildProvider(providerID, name, startURL string) SSOProvider {
	provider := SSOProvider{
		ProviderID:  providerID,
		Name:        name,
		ButtonLabel: "Continue with " + name,
		StartURL:    strings.TrimSpace(startURL),
		Enabled:     strings.TrimSpace(startURL) != "",
	}
	if !provider.Enabled {
		provider.Hint = "Set " + strings.ToUpper(providerID) + "_OAUTH_URL on the backend to enable this SSO path."
	}
	return provider
}

func (h *Handler) Register(mux *http.ServeMux) {
	mux.HandleFunc("POST /api/v1/auth/sign-up", h.signUp)
	mux.HandleFunc("POST /api/v1/auth/sign-in", h.signIn)
	mux.HandleFunc("GET /api/v1/auth/me", h.me)
	mux.HandleFunc("GET /api/v1/auth/sso/providers", h.listSSOProviders)

	mux.HandleFunc("POST /auth/register", h.signUp)
	mux.HandleFunc("POST /auth/login", h.signIn)
	mux.HandleFunc("GET /auth/me", h.me)
	mux.HandleFunc("GET /auth/sso/providers", h.listSSOProviders)
}

func (h *Handler) listSSOProviders(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"providers": h.ssoProviders})
}

func (h *Handler) signUp(w http.ResponseWriter, r *http.Request) {
	var req signUpRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body.")
		return
	}

	req.FullName = strings.TrimSpace(req.FullName)
	req.Email = strings.ToLower(strings.TrimSpace(req.Email))

	if req.FullName == "" || req.Email == "" || req.Password == "" {
		writeError(w, http.StatusBadRequest, "Full name, email, and password are required.")
		return
	}
	if !emailRE.MatchString(req.Email) {
		writeError(w, http.StatusBadRequest, "Enter a valid email address.")
		return
	}
	if len(req.Password) < 8 {
		writeError(w, http.StatusBadRequest, "Password must be at least 8 characters.")
		return
	}
	if _, err := h.store.GetUserByEmail(r.Context(), req.Email); err == nil {
		writeError(w, http.StatusConflict, "An account already exists for that email address.")
		return
	} else if !errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusInternalServerError, "Could not create your account right now.")
		return
	}

	passHash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Could not create your account right now.")
		return
	}

	workspace, err := h.createWorkspace(r, req.FullName, req.Email)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Could not create your workspace right now.")
		return
	}

	user, err := h.createUser(r, req.FullName, req.Email, string(passHash), workspace.WorkspaceID)
	if err != nil {
		if errors.Is(err, store.ErrDuplicate) {
			writeError(w, http.StatusConflict, "An account already exists for that email address.")
			return
		}
		writeError(w, http.StatusInternalServerError, "Could not create your account right now.")
		return
	}

	token, expiresAt, err := h.jwt.Sign(user.UserID, user.WorkspaceID, user.IsAdmin)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Could not create your session right now.")
		return
	}

	writeJSON(w, http.StatusCreated, authResponse{
		Token:     token,
		User:      user,
		Workspace: workspace,
		ExpiresAt: expiresAt,
	})
}

func (h *Handler) signIn(w http.ResponseWriter, r *http.Request) {
	var req signInRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body.")
		return
	}

	req.Email = strings.ToLower(strings.TrimSpace(req.Email))
	if req.Email == "" || req.Password == "" {
		writeError(w, http.StatusBadRequest, "Email and password are required.")
		return
	}

	user, err := h.store.GetUserByEmail(r.Context(), req.Email)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeError(w, http.StatusUnauthorized, "Incorrect email or password.")
			return
		}
		writeError(w, http.StatusInternalServerError, "Could not sign you in right now.")
		return
	}
	if err := bcrypt.CompareHashAndPassword([]byte(user.PassHash), []byte(req.Password)); err != nil {
		writeError(w, http.StatusUnauthorized, "Incorrect email or password.")
		return
	}

	workspace, err := h.store.GetWorkspaceByID(r.Context(), user.WorkspaceID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Could not sign you in right now.")
		return
	}

	token, expiresAt, err := h.jwt.Sign(user.UserID, user.WorkspaceID, user.IsAdmin)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Could not create your session right now.")
		return
	}

	writeJSON(w, http.StatusOK, authResponse{
		Token:     token,
		User:      user,
		Workspace: workspace,
		ExpiresAt: expiresAt,
	})
}

func (h *Handler) me(w http.ResponseWriter, r *http.Request) {
	token := strings.TrimSpace(strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer"))
	if token == "" {
		writeError(w, http.StatusUnauthorized, "Missing bearer token.")
		return
	}

	claims, err := h.jwt.Verify(token)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "Session expired or invalid.")
		return
	}

	user, err := h.store.GetUserByID(r.Context(), claims.UserID)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "Session expired or invalid.")
		return
	}

	workspace, err := h.store.GetWorkspaceByID(r.Context(), claims.WorkspaceID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Could not load your workspace.")
		return
	}

	writeJSON(w, http.StatusOK, authResponse{
		Token:     token,
		User:      user,
		Workspace: workspace,
		ExpiresAt: claims.ExpiresAt.Time,
	})
}

func (h *Handler) createWorkspace(r *http.Request, fullName, email string) (*domain.Workspace, error) {
	baseSlug := slugify(firstNonEmpty(fullName, emailPrefix(email)))
	if baseSlug == "" {
		baseSlug = "workspace"
	}

	for attempt := 0; attempt < 5; attempt++ {
		slug := baseSlug
		if attempt > 0 {
			slug = slug + "-" + uuid.NewString()[:6]
		}

		workspace := &domain.Workspace{
			WorkspaceID: uuid.NewString(),
			Slug:        slug,
			Name:        firstName(fullName) + "'s workspace",
			CreatedAt:   time.Now().UTC(),
		}
		if err := h.store.CreateWorkspace(r.Context(), workspace); err != nil {
			if errors.Is(err, store.ErrDuplicate) {
				continue
			}
			return nil, err
		}
		return workspace, nil
	}

	return nil, store.ErrDuplicate
}

func (h *Handler) createUser(r *http.Request, fullName, email, passHash, workspaceID string) (*domain.User, error) {
	baseUsername := usernameBase(fullName, email)
	if baseUsername == "" {
		baseUsername = "member"
	}

	for attempt := 0; attempt < 5; attempt++ {
		username := baseUsername
		if attempt > 0 {
			username = username + "-" + uuid.NewString()[:6]
		}

		user := &domain.User{
			UserID:      uuid.NewString(),
			WorkspaceID: workspaceID,
			Username:    username,
			Email:       email,
			FullName:    fullName,
			IsAdmin:     false,
			PassHash:    passHash,
			CreatedAt:   time.Now().UTC(),
		}

		if err := h.store.CreateUser(r.Context(), user); err != nil {
			if errors.Is(err, store.ErrDuplicate) {
				continue
			}
			return nil, err
		}
		return user, nil
	}

	return nil, store.ErrDuplicate
}

func usernameBase(fullName, email string) string {
	candidate := slugify(fullName)
	if candidate == "" {
		candidate = slugify(emailPrefix(email))
	}
	if candidate == "" {
		return "member"
	}
	return candidate
}

func emailPrefix(email string) string {
	if at := strings.Index(email, "@"); at > 0 {
		return email[:at]
	}
	return email
}

func slugify(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	value = spacesRE.ReplaceAllString(value, "-")
	value = slugStripRE.ReplaceAllString(value, "-")
	value = strings.Trim(value, "-")
	return value
}

func firstName(fullName string) string {
	parts := strings.Fields(strings.TrimSpace(fullName))
	if len(parts) == 0 {
		return "Your"
	}
	return parts[0]
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}
