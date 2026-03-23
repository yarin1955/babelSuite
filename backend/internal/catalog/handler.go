package catalog

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/babelsuite/babelsuite/internal/auth"
	"github.com/babelsuite/babelsuite/internal/domain"
	"github.com/babelsuite/babelsuite/internal/store"
	"github.com/google/uuid"
)

type Handler struct {
	store  store.Store
	jwtMw  func(http.Handler) http.Handler
}

func NewHandler(st store.Store, jwtSvc *auth.JWTService) *Handler {
	return &Handler{store: st, jwtMw: auth.Middleware(jwtSvc)}
}

func (h *Handler) Register(mux *http.ServeMux) {
	wrap := func(fn http.HandlerFunc) http.Handler {
		return h.jwtMw(fn)
	}
	admin := func(fn http.HandlerFunc) http.Handler {
		return h.jwtMw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if c := auth.ClaimsFrom(r); c == nil || !c.IsAdmin {
				writeErr(w, http.StatusForbidden, "admin only")
				return
			}
			fn(w, r)
		}))
	}

	// Catalog (user-facing — shows enabled packages)
	mux.Handle("GET /api/catalog", wrap(h.listCatalog))
	mux.Handle("GET /api/catalog/{id}", wrap(h.getPackage))

	// Admin — registry management
	mux.Handle("GET /api/admin/registries",        admin(h.listRegistries))
	mux.Handle("POST /api/admin/registries",        admin(h.createRegistry))
	mux.Handle("PUT /api/admin/registries/{id}",    admin(h.updateRegistry))
	mux.Handle("DELETE /api/admin/registries/{id}", admin(h.deleteRegistry))

	// Admin — package management
	mux.Handle("GET /api/admin/catalog",               admin(h.listAllPackages))
	mux.Handle("POST /api/admin/catalog/{id}/enable",   admin(h.enablePackage))
	mux.Handle("POST /api/admin/catalog/{id}/disable",  admin(h.disablePackage))
	mux.Handle("DELETE /api/admin/catalog/{id}",        admin(h.deletePackage))
	mux.Handle("POST /api/admin/registries/{id}/sync",  admin(h.syncRegistry))
}

// ── catalog (user) ────────────────────────────────────────────────────────────

func (h *Handler) listCatalog(w http.ResponseWriter, r *http.Request) {
	claims := auth.ClaimsFrom(r)
	f := parseFilter(r, true)
	pkgs, total, err := h.store.ListPackages(r.Context(), claims.OrgID, f)
	if err != nil {
		writeErr(w, 500, "internal error")
		return
	}
	writeJSON(w, 200, map[string]any{
		"packages":   pkgs,
		"total":      total,
		"page":       f.Page,
		"page_size":  f.PageSize,
		"total_pages": totalPages(total, f.PageSize),
	})
}

func (h *Handler) getPackage(w http.ResponseWriter, r *http.Request) {
	pkg, err := h.store.GetPackage(r.Context(), r.PathValue("id"))
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeErr(w, 404, "not found")
			return
		}
		writeErr(w, 500, "internal error")
		return
	}
	writeJSON(w, 200, pkg)
}

// ── admin registries ──────────────────────────────────────────────────────────

func (h *Handler) listRegistries(w http.ResponseWriter, r *http.Request) {
	claims := auth.ClaimsFrom(r)
	regs, err := h.store.ListRegistries(r.Context(), claims.OrgID)
	if err != nil {
		writeErr(w, 500, "internal error")
		return
	}
	writeJSON(w, 200, regs)
}

func (h *Handler) createRegistry(w http.ResponseWriter, r *http.Request) {
	claims := auth.ClaimsFrom(r)
	var body domain.Registry
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, 400, "invalid json")
		return
	}
	body.RegistryID = uuid.NewString()
	body.OrgID = claims.OrgID
	body.CreatedAt = time.Now().UTC()
	if err := h.store.CreateRegistry(r.Context(), &body); err != nil {
		writeErr(w, 500, "internal error")
		return
	}
	body.Token = ""
	writeJSON(w, 201, body)
}

func (h *Handler) updateRegistry(w http.ResponseWriter, r *http.Request) {
	claims := auth.ClaimsFrom(r)
	existing, err := h.store.GetRegistry(r.Context(), r.PathValue("id"))
	if err != nil {
		writeErr(w, 404, "not found")
		return
	}
	if existing.OrgID != claims.OrgID {
		writeErr(w, 403, "forbidden")
		return
	}
	var body domain.Registry
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, 400, "invalid json")
		return
	}
	body.RegistryID = existing.RegistryID
	body.OrgID = existing.OrgID
	body.CreatedAt = existing.CreatedAt
	if body.Token == "" {
		body.Token = existing.Token
	}
	if err := h.store.UpdateRegistry(r.Context(), &body); err != nil {
		writeErr(w, 500, "internal error")
		return
	}
	body.Token = ""
	writeJSON(w, 200, body)
}

func (h *Handler) deleteRegistry(w http.ResponseWriter, r *http.Request) {
	claims := auth.ClaimsFrom(r)
	existing, err := h.store.GetRegistry(r.Context(), r.PathValue("id"))
	if err != nil {
		writeErr(w, 404, "not found")
		return
	}
	if existing.OrgID != claims.OrgID {
		writeErr(w, 403, "forbidden")
		return
	}
	if err := h.store.DeleteRegistry(r.Context(), r.PathValue("id")); err != nil {
		writeErr(w, 500, "internal error")
		return
	}
	w.WriteHeader(204)
}

// ── admin packages ────────────────────────────────────────────────────────────

func (h *Handler) listAllPackages(w http.ResponseWriter, r *http.Request) {
	claims := auth.ClaimsFrom(r)
	f := parseFilter(r, false)
	pkgs, total, err := h.store.ListPackages(r.Context(), claims.OrgID, f)
	if err != nil {
		writeErr(w, 500, "internal error")
		return
	}
	writeJSON(w, 200, map[string]any{
		"packages":    pkgs,
		"total":       total,
		"page":        f.Page,
		"page_size":   f.PageSize,
		"total_pages": totalPages(total, f.PageSize),
	})
}

func (h *Handler) enablePackage(w http.ResponseWriter, r *http.Request) {
	h.togglePackage(w, r, true)
}

func (h *Handler) disablePackage(w http.ResponseWriter, r *http.Request) {
	h.togglePackage(w, r, false)
}

func (h *Handler) togglePackage(w http.ResponseWriter, r *http.Request, enabled bool) {
	if err := h.store.SetPackageEnabled(r.Context(), r.PathValue("id"), enabled); err != nil {
		writeErr(w, 500, "internal error")
		return
	}
	writeJSON(w, 200, map[string]bool{"enabled": enabled})
}

func (h *Handler) deletePackage(w http.ResponseWriter, r *http.Request) {
	if err := h.store.DeletePackage(r.Context(), r.PathValue("id")); err != nil {
		writeErr(w, 500, "internal error")
		return
	}
	w.WriteHeader(204)
}

// ── sync ──────────────────────────────────────────────────────────────────────

func (h *Handler) syncRegistry(w http.ResponseWriter, r *http.Request) {
	claims := auth.ClaimsFrom(r)
	reg, err := h.store.GetRegistry(r.Context(), r.PathValue("id"))
	if err != nil || reg.OrgID != claims.OrgID {
		writeErr(w, 404, "not found")
		return
	}

	var body struct {
		Repos []string `json:"repos"`
	}
	json.NewDecoder(r.Body).Decode(&body)
	if len(body.Repos) == 0 {
		writeErr(w, 400, "repos list is required")
		return
	}

	var synced int
	for _, repo := range body.Repos {
		pkgs, err := SyncRepo(reg, claims.OrgID, repo)
		if err != nil {
			continue
		}
		for _, pkg := range pkgs {
			h.store.UpsertPackage(r.Context(), pkg)
			synced++
		}
	}
	writeJSON(w, 200, map[string]int{"synced": synced})
}

// ── helpers ───────────────────────────────────────────────────────────────────

func parseFilter(r *http.Request, enabledOnly bool) domain.CatalogFilter {
	q := r.URL.Query()
	page, _ := strconv.Atoi(q.Get("page"))
	pageSize, _ := strconv.Atoi(q.Get("page_size"))
	if pageSize <= 0 {
		pageSize = 20
	}
	if page <= 0 {
		page = 1
	}
	return domain.CatalogFilter{
		Search:       q.Get("q"),
		RegistryKind: domain.RegistryKind(q.Get("registry")),
		EnabledOnly:  enabledOnly,
		Page:         page,
		PageSize:     pageSize,
	}
}

func totalPages(total int64, pageSize int) int {
	if pageSize <= 0 || total == 0 {
		return 1
	}
	pages := int(total) / pageSize
	if int(total)%pageSize != 0 {
		pages++
	}
	return pages
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}
