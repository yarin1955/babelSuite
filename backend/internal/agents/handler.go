package agents

import (
	"crypto/rand"
	"encoding/base32"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/babelsuite/babelsuite/internal/auth"
	"github.com/babelsuite/babelsuite/internal/domain"
	"github.com/babelsuite/babelsuite/internal/store"
	"github.com/google/uuid"
)

type Handler struct {
	store store.Store
	jwt   *auth.JWTService
}

func NewHandler(s store.Store, jwt *auth.JWTService) *Handler {
	return &Handler{store: s, jwt: jwt}
}

func (h *Handler) Register(mux *http.ServeMux) {
	// Management routes — user JWT required
	mux.HandleFunc("GET /api/agents", h.adminMiddleware(h.list))
	mux.HandleFunc("POST /api/agents", h.adminMiddleware(h.create))
	mux.HandleFunc("GET /api/agents/{id}", h.adminMiddleware(h.get))
	mux.HandleFunc("PATCH /api/agents/{id}", h.adminMiddleware(h.update))
	mux.HandleFunc("DELETE /api/agents/{id}", h.adminMiddleware(h.delete))

	// Agent-facing routes — agent token required
	mux.HandleFunc("POST /api/agent/register", h.agentMiddleware(h.agentRegister))
	mux.HandleFunc("POST /api/agent/health", h.agentMiddleware(h.agentHealth))
	mux.HandleFunc("GET /api/agent/next", h.agentMiddleware(h.agentNext))
}

// ── management ────────────────────────────────────────────────────────────────

func (h *Handler) list(w http.ResponseWriter, r *http.Request) {
	claims := auth.ClaimsFrom(r)
	agents, err := h.store.ListAgents(r.Context(), claims.OrgID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "internal error")
		return
	}
	if agents == nil {
		agents = []*domain.Agent{}
	}
	writeJSON(w, http.StatusOK, agents)
}

func (h *Handler) create(w http.ResponseWriter, r *http.Request) {
	claims := auth.ClaimsFrom(r)
	var req struct {
		Name              string            `json:"name"`
		DesiredBackend    string            `json:"desired_backend"`
		DesiredPlatform   string            `json:"desired_platform"`
		DesiredTargetName string            `json:"desired_target_name"`
		DesiredTargetURL  string            `json:"desired_target_url"`
		Capacity          int               `json:"capacity"`
		Labels            map[string]string `json:"labels"`
		NoSchedule        bool              `json:"no_schedule"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	if req.Name == "" {
		writeErr(w, http.StatusBadRequest, "name is required")
		return
	}
	desiredBackend, err := normalizeDesiredBackend(defaultDesiredBackend(req.DesiredBackend))
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	desiredPlatform := normalizeDesiredPlatform(req.DesiredPlatform)
	if req.Capacity <= 0 {
		req.Capacity = 1
	}
	desiredTargetName := normalizeTargetName(req.DesiredTargetName)
	desiredTargetURL := normalizeTargetURL(req.DesiredTargetURL)
	if err := validateDesiredTarget(desiredBackend, desiredTargetName, desiredTargetURL); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}

	token, err := generateToken()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "internal error")
		return
	}

	a := &domain.Agent{
		AgentID:           uuid.NewString(),
		OrgID:             claims.OrgID,
		Name:              req.Name,
		Token:             token,
		DesiredBackend:    desiredBackend,
		DesiredPlatform:   desiredPlatform,
		DesiredTargetName: desiredTargetName,
		DesiredTargetURL:  desiredTargetURL,
		Capacity:          req.Capacity,
		Labels:            req.Labels,
		NoSchedule:        req.NoSchedule,
		LastContact:       time.Now().UTC(),
		CreatedAt:         time.Now().UTC(),
	}
	if err := h.store.CreateAgent(r.Context(), a); err != nil {
		writeErr(w, http.StatusInternalServerError, "internal error")
		return
	}
	// Return token once on creation
	writeJSON(w, http.StatusCreated, map[string]any{
		"agent": a,
		"token": token,
	})
}

func (h *Handler) get(w http.ResponseWriter, r *http.Request) {
	claims := auth.ClaimsFrom(r)
	id := r.PathValue("id")
	a, err := h.store.GetAgent(r.Context(), id)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeErr(w, http.StatusNotFound, "not found")
		} else {
			writeErr(w, http.StatusInternalServerError, "internal error")
		}
		return
	}
	if a.OrgID != claims.OrgID {
		writeErr(w, http.StatusForbidden, "forbidden")
		return
	}
	writeJSON(w, http.StatusOK, a)
}

func (h *Handler) update(w http.ResponseWriter, r *http.Request) {
	claims := auth.ClaimsFrom(r)
	id := r.PathValue("id")
	a, err := h.store.GetAgent(r.Context(), id)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeErr(w, http.StatusNotFound, "not found")
		} else {
			writeErr(w, http.StatusInternalServerError, "internal error")
		}
		return
	}
	if a.OrgID != claims.OrgID {
		writeErr(w, http.StatusForbidden, "forbidden")
		return
	}
	var req struct {
		Name              *string           `json:"name"`
		DesiredBackend    *string           `json:"desired_backend"`
		DesiredPlatform   *string           `json:"desired_platform"`
		DesiredTargetName *string           `json:"desired_target_name"`
		DesiredTargetURL  *string           `json:"desired_target_url"`
		NoSchedule        *bool             `json:"no_schedule"`
		Capacity          *int              `json:"capacity"`
		Labels            map[string]string `json:"labels"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	if req.Name != nil {
		a.Name = *req.Name
	}
	if req.DesiredBackend != nil {
		desiredBackend, err := normalizeDesiredBackend(*req.DesiredBackend)
		if err != nil {
			writeErr(w, http.StatusBadRequest, err.Error())
			return
		}
		a.DesiredBackend = desiredBackend
	}
	if req.DesiredPlatform != nil {
		a.DesiredPlatform = normalizeDesiredPlatform(*req.DesiredPlatform)
	}
	if req.DesiredTargetName != nil {
		a.DesiredTargetName = normalizeTargetName(*req.DesiredTargetName)
	}
	if req.DesiredTargetURL != nil {
		a.DesiredTargetURL = normalizeTargetURL(*req.DesiredTargetURL)
	}
	if req.NoSchedule != nil {
		a.NoSchedule = *req.NoSchedule
	}
	if req.Capacity != nil && *req.Capacity > 0 {
		a.Capacity = *req.Capacity
	}
	if req.Labels != nil {
		a.Labels = req.Labels
	}
	if err := validateDesiredTarget(a.DesiredBackend, a.DesiredTargetName, a.DesiredTargetURL); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := h.store.UpdateAgent(r.Context(), a); err != nil {
		writeErr(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, a)
}

func (h *Handler) delete(w http.ResponseWriter, r *http.Request) {
	claims := auth.ClaimsFrom(r)
	id := r.PathValue("id")
	a, err := h.store.GetAgent(r.Context(), id)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeErr(w, http.StatusNotFound, "not found")
		} else {
			writeErr(w, http.StatusInternalServerError, "internal error")
		}
		return
	}
	if a.OrgID != claims.OrgID {
		writeErr(w, http.StatusForbidden, "forbidden")
		return
	}
	active, err := h.store.CountActiveRunsByAgent(r.Context(), id)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "internal error")
		return
	}
	if active > 0 {
		writeErr(w, http.StatusConflict, "agent has active runs")
		return
	}
	if err := h.store.DeleteAgent(r.Context(), id); err != nil {
		writeErr(w, http.StatusInternalServerError, "internal error")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ── agent-facing ──────────────────────────────────────────────────────────────

// agentRegister is called by the agent on startup to update its capabilities.
func (h *Handler) agentRegister(w http.ResponseWriter, r *http.Request) {
	a := agentFrom(r)
	var req struct {
		Name       string            `json:"name"`
		Platform   string            `json:"platform"`
		Backend    string            `json:"backend"`
		TargetName string            `json:"target_name"`
		TargetURL  string            `json:"target_url"`
		Capacity   int               `json:"capacity"`
		Version    string            `json:"version"`
		Labels     map[string]string `json:"labels"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	if req.Name != "" {
		a.Name = req.Name
	}
	a.Platform = req.Platform
	a.Backend = req.Backend
	a.TargetName = normalizeTargetName(req.TargetName)
	a.TargetURL = normalizeTargetURL(req.TargetURL)
	if req.Capacity > 0 {
		a.Capacity = req.Capacity
	}
	a.Version = req.Version
	if req.Labels != nil {
		a.Labels = req.Labels
	}
	a.LastContact = time.Now().UTC()
	if err := h.store.UpdateAgent(r.Context(), a); err != nil {
		writeErr(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"agent_id": a.AgentID})
}

// agentHealth is called periodically by the agent as a heartbeat.
func (h *Handler) agentHealth(w http.ResponseWriter, r *http.Request) {
	a := agentFrom(r)
	a.LastContact = time.Now().UTC()
	if err := h.store.UpdateAgent(r.Context(), a); err != nil {
		writeErr(w, http.StatusInternalServerError, "internal error")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// agentNext returns the next pending task for this agent, or 204 if none.
func (h *Handler) agentNext(w http.ResponseWriter, r *http.Request) {
	a := agentFrom(r)
	if a.NoSchedule {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	// Task queue will be wired here when the runs system is added.
	w.WriteHeader(http.StatusNoContent)
}

// ── middleware ────────────────────────────────────────────────────────────────

func agentFrom(r *http.Request) *domain.Agent {
	return agentFromContext(r.Context())
}

func (h *Handler) userMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return auth.Middleware(h.jwt)(http.HandlerFunc(next)).ServeHTTP
}

func (h *Handler) adminMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return auth.Middleware(h.jwt)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		claims := auth.ClaimsFrom(r)
		if claims == nil || !claims.IsAdmin {
			writeErr(w, http.StatusForbidden, "admin only")
			return
		}
		next(w, r)
	})).ServeHTTP
}

func (h *Handler) agentMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		bearer := r.Header.Get("Authorization")
		if !strings.HasPrefix(bearer, "Bearer ") {
			writeErr(w, http.StatusUnauthorized, "unauthorized")
			return
		}
		token := strings.TrimPrefix(bearer, "Bearer ")
		a, err := h.store.GetAgentByToken(r.Context(), token)
		if err != nil {
			writeErr(w, http.StatusUnauthorized, "invalid agent token")
			return
		}
		next(w, r.WithContext(contextWithAgent(r.Context(), a)))
	}
}

// ── helpers ───────────────────────────────────────────────────────────────────

func generateToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(b), nil
}

func defaultDesiredBackend(value string) string {
	if strings.TrimSpace(value) == "" {
		return "docker"
	}
	return value
}

func normalizeDesiredBackend(value string) (string, error) {
	normalized := strings.ToLower(strings.TrimSpace(value))
	switch normalized {
	case "", "docker", "kubernetes", "local":
		return normalized, nil
	default:
		return "", errors.New("desired_backend must be docker, kubernetes, local, or empty")
	}
}

func normalizeDesiredPlatform(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

func normalizeTargetName(value string) string {
	return strings.TrimSpace(value)
}

func normalizeTargetURL(value string) string {
	return strings.TrimSpace(value)
}

func validateDesiredTarget(backend, targetName, targetURL string) error {
	if targetURL != "" {
		parsed, err := url.ParseRequestURI(targetURL)
		if err != nil || parsed.Scheme == "" || parsed.Host == "" {
			return errors.New("desired_target_url must be a valid absolute URL")
		}
	}
	if backend == "kubernetes" && targetName == "" && targetURL == "" {
		return errors.New("kubernetes agents must set desired_target_name or desired_target_url")
	}
	return nil
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}
