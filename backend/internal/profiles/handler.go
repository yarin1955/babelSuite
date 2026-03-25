package profiles

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/babelsuite/babelsuite/internal/auth"
	"github.com/babelsuite/babelsuite/internal/domain"
	"github.com/babelsuite/babelsuite/internal/store"
	"github.com/google/uuid"
	"gopkg.in/yaml.v3"
)

type Handler struct {
	store store.Store
	jwt   *auth.JWTService
}

func NewHandler(st store.Store, jwt *auth.JWTService) *Handler {
	return &Handler{store: st, jwt: jwt}
}

func (h *Handler) Register(mux *http.ServeMux) {
	withUser := auth.Middleware(h.jwt)
	mux.Handle("GET /api/profiles", withUser(http.HandlerFunc(h.list)))
	mux.Handle("POST /api/profiles", withUser(http.HandlerFunc(h.create)))
	mux.Handle("GET /api/profiles/{id}", withUser(http.HandlerFunc(h.get)))
	mux.Handle("PUT /api/profiles/{id}", withUser(http.HandlerFunc(h.update)))
	mux.Handle("DELETE /api/profiles/{id}", withUser(http.HandlerFunc(h.delete)))
}

func (h *Handler) list(w http.ResponseWriter, r *http.Request) {
	claims := auth.ClaimsFrom(r)
	profiles, err := h.store.ListProfiles(r.Context(), claims.OrgID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "internal error")
		return
	}
	if profiles == nil {
		profiles = []*domain.Profile{}
	}
	writeJSON(w, http.StatusOK, profiles)
}

func (h *Handler) get(w http.ResponseWriter, r *http.Request) {
	claims := auth.ClaimsFrom(r)
	profile, err := h.store.GetProfile(r.Context(), r.PathValue("id"))
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeErr(w, http.StatusNotFound, "profile not found")
		} else {
			writeErr(w, http.StatusInternalServerError, "internal error")
		}
		return
	}
	if profile.OrgID != claims.OrgID {
		writeErr(w, http.StatusForbidden, "forbidden")
		return
	}
	writeJSON(w, http.StatusOK, profile)
}

func (h *Handler) create(w http.ResponseWriter, r *http.Request) {
	claims := auth.ClaimsFrom(r)
	var req struct {
		Name        string               `json:"name"`
		Description string               `json:"description"`
		Format      domain.ProfileFormat `json:"format"`
		Content     string               `json:"content"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	if err := validateProfileInput(req.Name, req.Format, req.Content); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}

	now := time.Now().UTC()
	actorID, actorName := h.actor(r)
	profile := &domain.Profile{
		ProfileID:     uuid.NewString(),
		OrgID:         claims.OrgID,
		Name:          strings.TrimSpace(req.Name),
		Description:   strings.TrimSpace(req.Description),
		Format:        normalizeFormat(req.Format),
		Content:       normalizeContent(req.Content),
		Revision:      1,
		CreatedBy:     actorID,
		CreatedByName: actorName,
		UpdatedBy:     actorID,
		UpdatedByName: actorName,
		CreatedAt:     now,
		UpdatedAt:     now,
	}
	if err := h.store.CreateProfile(r.Context(), profile); err != nil {
		if errors.Is(err, store.ErrDuplicate) {
			writeErr(w, http.StatusConflict, "profile name already exists")
		} else {
			writeErr(w, http.StatusInternalServerError, "internal error")
		}
		return
	}
	writeJSON(w, http.StatusCreated, profile)
}

func (h *Handler) update(w http.ResponseWriter, r *http.Request) {
	claims := auth.ClaimsFrom(r)
	profile, err := h.store.GetProfile(r.Context(), r.PathValue("id"))
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeErr(w, http.StatusNotFound, "profile not found")
		} else {
			writeErr(w, http.StatusInternalServerError, "internal error")
		}
		return
	}
	if profile.OrgID != claims.OrgID {
		writeErr(w, http.StatusForbidden, "forbidden")
		return
	}

	var req struct {
		Name          string               `json:"name"`
		Description   string               `json:"description"`
		Format        domain.ProfileFormat `json:"format"`
		Content       string               `json:"content"`
		BaseUpdatedAt string               `json:"base_updated_at"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	if err := validateProfileInput(req.Name, req.Format, req.Content); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	if req.BaseUpdatedAt != "" {
		baseUpdatedAt, err := time.Parse(time.RFC3339Nano, req.BaseUpdatedAt)
		if err != nil {
			writeErr(w, http.StatusBadRequest, "base_updated_at must be RFC3339")
			return
		}
		if !profile.UpdatedAt.Equal(baseUpdatedAt) {
			writeErr(w, http.StatusConflict, "profile was updated by someone else; refresh and try again")
			return
		}
	}

	actorID, actorName := h.actor(r)
	profile.Name = strings.TrimSpace(req.Name)
	profile.Description = strings.TrimSpace(req.Description)
	profile.Format = normalizeFormat(req.Format)
	profile.Content = normalizeContent(req.Content)
	profile.Revision++
	profile.UpdatedBy = actorID
	profile.UpdatedByName = actorName
	profile.UpdatedAt = time.Now().UTC()

	if err := h.store.UpdateProfile(r.Context(), profile); err != nil {
		if errors.Is(err, store.ErrDuplicate) {
			writeErr(w, http.StatusConflict, "profile name already exists")
		} else {
			writeErr(w, http.StatusInternalServerError, "internal error")
		}
		return
	}
	writeJSON(w, http.StatusOK, profile)
}

func (h *Handler) delete(w http.ResponseWriter, r *http.Request) {
	claims := auth.ClaimsFrom(r)
	profile, err := h.store.GetProfile(r.Context(), r.PathValue("id"))
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeErr(w, http.StatusNotFound, "profile not found")
		} else {
			writeErr(w, http.StatusInternalServerError, "internal error")
		}
		return
	}
	if profile.OrgID != claims.OrgID {
		writeErr(w, http.StatusForbidden, "forbidden")
		return
	}
	if err := h.store.DeleteProfile(r.Context(), profile.ProfileID); err != nil {
		writeErr(w, http.StatusInternalServerError, "internal error")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func validateProfileInput(name string, format domain.ProfileFormat, content string) error {
	if strings.TrimSpace(name) == "" {
		return errors.New("name is required")
	}
	if len(strings.TrimSpace(name)) > 80 {
		return errors.New("name must be 80 characters or fewer")
	}
	switch normalizeFormat(format) {
	case domain.ProfileFormatYAML:
		var decoded any
		if err := yaml.Unmarshal([]byte(normalizeContent(content)), &decoded); err != nil {
			return errors.New("content must be valid YAML when format=yaml")
		}
	case domain.ProfileFormatJSON:
		if !json.Valid([]byte(normalizeContent(content))) {
			return errors.New("content must be valid JSON when format=json")
		}
	default:
		return errors.New("format must be yaml or json")
	}
	return nil
}

func normalizeFormat(format domain.ProfileFormat) domain.ProfileFormat {
	switch strings.ToLower(strings.TrimSpace(string(format))) {
	case "json":
		return domain.ProfileFormatJSON
	default:
		return domain.ProfileFormatYAML
	}
}

func normalizeContent(content string) string {
	trimmed := strings.ReplaceAll(content, "\r\n", "\n")
	if trimmed == "" {
		return ""
	}
	return trimmed
}

func (h *Handler) actor(r *http.Request) (string, string) {
	claims := auth.ClaimsFrom(r)
	if claims == nil {
		return "", ""
	}
	user, err := h.store.GetUserByID(r.Context(), claims.UserID)
	if err != nil {
		return claims.UserID, claims.UserID
	}
	if strings.TrimSpace(user.Name) != "" {
		return user.UserID, user.Name
	}
	return user.UserID, user.Username
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}
