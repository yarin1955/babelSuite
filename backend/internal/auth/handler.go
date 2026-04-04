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
	store  store.Store
	jwt    *JWTService
	config Config
	oidc   *OIDCService
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

type authConfigResponse struct {
	PasswordAuthEnabled bool          `json:"passwordAuthEnabled"`
	SignUpEnabled       bool          `json:"signUpEnabled"`
	Providers           []SSOProvider `json:"providers"`
}

type authResponse struct {
	Token     string            `json:"token"`
	User      *domain.User      `json:"user"`
	Workspace *domain.Workspace `json:"workspace"`
	ExpiresAt time.Time         `json:"expiresAt"`
}

func NewHandler(st store.Store, jwt *JWTService, config Config) *Handler {
	return &Handler{
		store:  st,
		jwt:    jwt,
		config: config,
		oidc:   NewOIDCService(config.OIDC),
	}
}

func (h *Handler) Register(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/auth/config", h.getAuthConfig)
	mux.HandleFunc("POST /api/v1/auth/sign-up", h.signUp)
	mux.HandleFunc("POST /api/v1/auth/sign-in", h.signIn)
	mux.HandleFunc("GET /api/v1/auth/me", h.me)
	mux.HandleFunc("GET /api/v1/auth/sso/providers", h.listSSOProviders)
	mux.HandleFunc("GET /api/v1/auth/oidc/login", h.oidcLogin)
	mux.HandleFunc("GET /api/v1/auth/oidc/callback", h.oidcCallback)

	mux.HandleFunc("GET /auth/config", h.getAuthConfig)
	mux.HandleFunc("POST /auth/register", h.signUp)
	mux.HandleFunc("POST /auth/login", h.signIn)
	mux.HandleFunc("GET /auth/me", h.me)
	mux.HandleFunc("GET /auth/sso/providers", h.listSSOProviders)
	mux.HandleFunc("GET /auth/oidc/login", h.oidcLogin)
	mux.HandleFunc("GET /auth/oidc/callback", h.oidcCallback)
}

func (h *Handler) getAuthConfig(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, h.authConfigResponse(r))
}

func (h *Handler) listSSOProviders(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"providers": h.authConfigResponse(r).Providers})
}

func (h *Handler) signUp(w http.ResponseWriter, r *http.Request) {
	if !h.config.SignUpEnabled {
		writeError(w, http.StatusForbidden, "Local sign-up is disabled.")
		return
	}

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

	user, err := h.createUser(r, req.FullName, req.Email, string(passHash), workspace.WorkspaceID, false)
	if err != nil {
		_ = h.store.DeleteWorkspace(r.Context(), workspace.WorkspaceID)
		if errors.Is(err, store.ErrDuplicate) {
			writeError(w, http.StatusConflict, "An account already exists for that email address.")
			return
		}
		writeError(w, http.StatusInternalServerError, "Could not create your account right now.")
		return
	}

	token, expiresAt, err := h.jwt.Sign(user.UserID, user.WorkspaceID, user.IsAdmin, nil, "password")
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
	if !h.config.PasswordAuthEnabled {
		writeError(w, http.StatusForbidden, "Local password sign-in is disabled.")
		return
	}

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

	token, expiresAt, err := h.jwt.Sign(user.UserID, user.WorkspaceID, user.IsAdmin, nil, "password")
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

func (h *Handler) oidcLogin(w http.ResponseWriter, r *http.Request) {
	if !h.config.OIDCEnabled() {
		writeError(w, http.StatusNotFound, "Single sign-on is not configured.")
		return
	}

	returnURL := sanitizeReturnURL(h.config.FrontendURL, r.URL.Query().Get("return_url"))
	redirectURL, stateCookie, err := h.oidc.BeginLogin(r.Context(), returnURL, requestIsSecure(r))
	if err != nil {
		writeError(w, http.StatusBadGateway, "Could not start single sign-on right now.")
		return
	}

	http.SetCookie(w, stateCookie)
	http.Redirect(w, r, redirectURL, http.StatusFound)
}

func (h *Handler) oidcCallback(w http.ResponseWriter, r *http.Request) {
	if !h.config.OIDCEnabled() {
		writeError(w, http.StatusNotFound, "Single sign-on is not configured.")
		return
	}

	http.SetCookie(w, h.oidc.ClearStateCookie(requestIsSecure(r)))

	if description := strings.TrimSpace(r.URL.Query().Get("error_description")); description != "" {
		h.redirectOIDCError(w, r, http.StatusUnauthorized, description)
		return
	}
	if errCode := strings.TrimSpace(r.URL.Query().Get("error")); errCode != "" {
		h.redirectOIDCError(w, r, http.StatusUnauthorized, "Single sign-on failed: "+errCode)
		return
	}

	cookie, err := r.Cookie(h.config.OIDC.NormalizedStateCookieName())
	if err != nil {
		h.redirectOIDCError(w, r, http.StatusUnauthorized, "Single sign-on state has expired. Please try again.")
		return
	}

	code := strings.TrimSpace(r.URL.Query().Get("code"))
	stateValue := strings.TrimSpace(r.URL.Query().Get("state"))
	if code == "" || stateValue == "" {
		h.redirectOIDCError(w, r, http.StatusBadRequest, "Single sign-on response is missing the required state.")
		return
	}

	identity, returnURL, err := h.oidc.Exchange(r.Context(), stateValue, code, cookie)
	if err != nil {
		h.redirectOIDCError(w, r, http.StatusUnauthorized, "Could not complete single sign-on right now.")
		return
	}

	user, workspace, isAdmin, err := h.resolveOIDCUser(r, identity)
	if err != nil {
		h.redirectOIDCError(w, r, http.StatusInternalServerError, "Could not create your session right now.")
		return
	}

	token, expiresAt, err := h.jwt.Sign(user.UserID, workspace.WorkspaceID, isAdmin, identity.Groups, h.config.OIDC.NormalizedProviderID())
	if err != nil {
		h.redirectOIDCError(w, r, http.StatusInternalServerError, "Could not create your session right now.")
		return
	}

	callbackURL := appendFragmentParams(h.config.OIDC.FrontendCallbackURL, map[string]string{
		"token":      token,
		"expiresAt":  expiresAt.Format(time.RFC3339),
		"return_url": returnURL,
	})
	http.Redirect(w, r, callbackURL, http.StatusFound)
}

func (h *Handler) redirectOIDCError(w http.ResponseWriter, r *http.Request, status int, message string) {
	callbackURL := appendFragmentParams(h.config.OIDC.FrontendCallbackURL, map[string]string{
		"error": message,
	})
	http.Redirect(w, r, callbackURL, statusToRedirect(status))
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

	userView := *user
	userView.IsAdmin = claims.IsAdmin

	writeJSON(w, http.StatusOK, authResponse{
		Token:     token,
		User:      &userView,
		Workspace: workspace,
		ExpiresAt: claims.ExpiresAt.Time,
	})
}

func (h *Handler) authConfigResponse(r *http.Request) authConfigResponse {
	providers := []SSOProvider{}
	if h.config.OIDCEnabled() {
		providers = append(providers, h.oidc.Provider(requestBaseURL(r)))
	}

	return authConfigResponse{
		PasswordAuthEnabled: h.config.PasswordAuthEnabled,
		SignUpEnabled:       h.config.SignUpEnabled,
		Providers:           providers,
	}
}

func (h *Handler) resolveOIDCUser(r *http.Request, identity *oidcIdentity) (*domain.User, *domain.Workspace, bool, error) {
	email := strings.ToLower(strings.TrimSpace(identity.Email))
	if email == "" {
		return nil, nil, false, store.ErrNotFound
	}

	user, err := h.store.GetUserByEmail(r.Context(), email)
	if err == nil {
		workspace, workspaceErr := h.store.GetWorkspaceByID(r.Context(), user.WorkspaceID)
		if workspaceErr != nil {
			return nil, nil, false, workspaceErr
		}
		isAdmin := user.IsAdmin || h.oidc.MapsGroupsToAdmin(identity.Groups)
		return user, workspace, isAdmin, nil
	}
	if !errors.Is(err, store.ErrNotFound) {
		return nil, nil, false, err
	}

	fullName := strings.TrimSpace(identity.FullName)
	if fullName == "" {
		fullName = emailPrefix(email)
	}

	workspace, err := h.createWorkspace(r, fullName, email)
	if err != nil {
		return nil, nil, false, err
	}

	isAdmin := h.oidc.MapsGroupsToAdmin(identity.Groups)
	user, err = h.createUser(r, fullName, email, "", workspace.WorkspaceID, isAdmin)
	if err != nil {
		_ = h.store.DeleteWorkspace(r.Context(), workspace.WorkspaceID)
		return nil, nil, false, err
	}

	return user, workspace, isAdmin, nil
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

func (h *Handler) createUser(r *http.Request, fullName, email, passHash, workspaceID string, isAdmin bool) (*domain.User, error) {
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
			IsAdmin:     isAdmin,
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

func requestBaseURL(r *http.Request) string {
	scheme := "http"
	if requestIsSecure(r) {
		scheme = "https"
	}
	return scheme + "://" + r.Host
}

func requestIsSecure(r *http.Request) bool {
	if r.TLS != nil {
		return true
	}
	if strings.EqualFold(strings.TrimSpace(r.Header.Get("X-Forwarded-Proto")), "https") {
		return true
	}
	return false
}

func statusToRedirect(status int) int {
	if status >= 400 {
		return http.StatusFound
	}
	return status
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}
