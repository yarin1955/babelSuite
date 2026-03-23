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

var slugRe = regexp.MustCompile(`^[a-z0-9-]{2,40}$`)

type Handler struct {
	store store.Store
	jwt   *JWTService
}

func NewHandler(s store.Store, jwt *JWTService) *Handler {
	return &Handler{store: s, jwt: jwt}
}

func (h *Handler) Register(mux *http.ServeMux) {
	mux.HandleFunc("POST /auth/register", h.register)
	mux.HandleFunc("POST /auth/login", h.login)
	mux.HandleFunc("GET /auth/me", h.me)
}

// ── register ─────────────────────────────────────────────────────────────────

type registerReq struct {
	Username string `json:"username"`
	Email    string `json:"email"`
	Name     string `json:"name"`
	Password string `json:"password"`
}

func (h *Handler) register(w http.ResponseWriter, r *http.Request) {
	var req registerReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	req.Username = strings.ToLower(strings.TrimSpace(req.Username))
	req.Email = strings.ToLower(strings.TrimSpace(req.Email))
	req.Name = strings.TrimSpace(req.Name)

	if req.Username == "" || req.Email == "" || req.Name == "" || req.Password == "" {
		writeErr(w, http.StatusBadRequest, "username, email, name and password are required")
		return
	}
	if len(req.Password) < 6 {
		writeErr(w, http.StatusBadRequest, "password must be at least 6 characters")
		return
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "internal error")
		return
	}

	// auto-create a personal org named after the username
	slug := req.Username
	if !slugRe.MatchString(slug) {
		slug = uuid.NewString()[:8]
	}
	org := &domain.Org{
		OrgID:     uuid.NewString(),
		Slug:      slug,
		Name:      req.Name + "'s workspace",
		CreatedAt: time.Now().UTC(),
	}
	if err := h.store.CreateOrg(r.Context(), org); err != nil {
		if errors.Is(err, store.ErrDuplicate) {
			// slug taken → make it unique
			org.OrgID = uuid.NewString()
			org.Slug = slug + "-" + uuid.NewString()[:6]
			if err := h.store.CreateOrg(r.Context(), org); err != nil {
				writeErr(w, http.StatusInternalServerError, "internal error")
				return
			}
		} else {
			writeErr(w, http.StatusInternalServerError, "internal error")
			return
		}
	}

	user := &domain.User{
		UserID:    uuid.NewString(),
		OrgID:     org.OrgID,
		Username:  req.Username,
		Email:     req.Email,
		Name:      req.Name,
		PassHash:  string(hash),
		CreatedAt: time.Now().UTC(),
	}
	if err := h.store.CreateUser(r.Context(), user); err != nil {
		if errors.Is(err, store.ErrDuplicate) {
			writeErr(w, http.StatusConflict, "username or email already taken")
			return
		}
		writeErr(w, http.StatusInternalServerError, "internal error")
		return
	}

	token, err := h.jwt.Sign(user.UserID, org.OrgID, user.IsAdmin)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{
		"token": token,
		"user":  user,
		"org":   org,
	})
}

// ── login ─────────────────────────────────────────────────────────────────────

type loginReq struct {
	Username string `json:"username"` // username OR email
	Password string `json:"password"`
}

func (h *Handler) login(w http.ResponseWriter, r *http.Request) {
	var req loginReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	req.Username = strings.TrimSpace(req.Username)

	if req.Username == "" || req.Password == "" {
		writeErr(w, http.StatusBadRequest, "username and password are required")
		return
	}

	// find by username or email
	var user *domain.User
	var err error
	if strings.Contains(req.Username, "@") {
		user, err = h.store.GetUserByEmail(r.Context(), strings.ToLower(req.Username))
	} else {
		user, err = h.store.GetUserByUsername(r.Context(), strings.ToLower(req.Username))
	}
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeErr(w, http.StatusUnauthorized, "invalid credentials")
			return
		}
		writeErr(w, http.StatusInternalServerError, "internal error")
		return
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.PassHash), []byte(req.Password)); err != nil {
		writeErr(w, http.StatusUnauthorized, "invalid credentials")
		return
	}

	org, err := h.store.GetOrgByID(r.Context(), user.OrgID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "internal error")
		return
	}

	token, err := h.jwt.Sign(user.UserID, org.OrgID, user.IsAdmin)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"token": token,
		"user":  user,
		"org":   org,
	})
}

// ── me ────────────────────────────────────────────────────────────────────────

func (h *Handler) me(w http.ResponseWriter, r *http.Request) {
	bearer := r.Header.Get("Authorization")
	if !strings.HasPrefix(bearer, "Bearer ") {
		writeErr(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	claims, err := h.jwt.Verify(strings.TrimPrefix(bearer, "Bearer "))
	if err != nil {
		writeErr(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	user, err := h.store.GetUserByID(r.Context(), claims.UserID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, user)
}

// ── helpers ───────────────────────────────────────────────────────────────────

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}
