package catalog

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/babelsuite/babelsuite/internal/auth"
	"github.com/babelsuite/babelsuite/internal/domain"
	"github.com/babelsuite/babelsuite/internal/store"
	"github.com/babelsuite/babelsuite/pkg/api"
	"github.com/google/uuid"
)

type Handler struct {
	store store.Store
	jwtMw func(http.Handler) http.Handler
}

type registryUpsertRequest struct {
	Name                  string `json:"name"`
	Kind                  string `json:"kind"`
	URL                   string `json:"url"`
	InsecureSkipTLSVerify bool   `json:"insecure_skip_tls_verify"`
	Username              string `json:"username"`
	Password              string `json:"password"`
	BearerToken           string `json:"bearer_token"`
	TLSCAData             string `json:"tls_ca_data"`
	TLSCertData           string `json:"tls_cert_data"`
	TLSKeyData            string `json:"tls_key_data"`
	ClearPassword         bool   `json:"clear_password"`
	ClearBearerToken      bool   `json:"clear_bearer_token"`
	ClearTLSCAData        bool   `json:"clear_tls_ca_data"`
	ClearTLSCertData      bool   `json:"clear_tls_cert_data"`
	ClearTLSKeyData       bool   `json:"clear_tls_key_data"`
	Enabled               bool   `json:"enabled"`
}

type registryConnection struct {
	Kind                  domain.RegistryKind
	URL                   string
	InsecureSkipTLSVerify bool
	Username              string
	Password              string
	BearerToken           string
	TLSCAData             string
	TLSCertData           string
	TLSKeyData            string
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
	mux.Handle("GET /api/admin/registries", admin(h.listRegistries))
	mux.Handle("POST /api/admin/registries", admin(h.createRegistry))
	mux.Handle("PUT /api/admin/registries/{id}", admin(h.updateRegistry))
	mux.Handle("DELETE /api/admin/registries/{id}", admin(h.deleteRegistry))

	// Admin — package management
	mux.Handle("GET /api/admin/catalog", admin(h.listAllPackages))
	mux.Handle("POST /api/admin/catalog/{id}/enable", admin(h.enablePackage))
	mux.Handle("POST /api/admin/catalog/{id}/disable", admin(h.disablePackage))
	mux.Handle("DELETE /api/admin/catalog/{id}", admin(h.deletePackage))
	mux.Handle("POST /api/admin/registries/{id}/sync", admin(h.syncRegistry))
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
		"packages":    pkgs,
		"total":       total,
		"page":        f.Page,
		"page_size":   f.PageSize,
		"total_pages": totalPages(total, f.PageSize),
	})
}

func (h *Handler) getPackage(w http.ResponseWriter, r *http.Request) {
	claims := auth.ClaimsFrom(r)
	pkg, err := h.store.GetPackage(r.Context(), r.PathValue("id"))
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeErr(w, 404, "not found")
			return
		}
		writeErr(w, 500, "internal error")
		return
	}
	if pkg.OrgID != claims.OrgID {
		writeErr(w, http.StatusForbidden, "forbidden")
		return
	}
	if !claims.IsAdmin && !pkg.Enabled {
		writeErr(w, http.StatusNotFound, "not found")
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
	payload := make([]*api.Registry, 0, len(regs))
	for _, reg := range regs {
		payload = append(payload, registryPayload(reg, false))
	}
	writeJSON(w, 200, payload)
}

func (h *Handler) createRegistry(w http.ResponseWriter, r *http.Request) {
	claims := auth.ClaimsFrom(r)
	var req registryUpsertRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, 400, "invalid json")
		return
	}

	reg, err := buildRegistry(claims.OrgID, nil, req)
	if err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	reg.RegistryID = uuid.NewString()
	reg.CreatedAt = time.Now().UTC()

	if err := h.store.CreateRegistry(r.Context(), reg); err != nil {
		writeErr(w, 500, "internal error")
		return
	}
	writeJSON(w, 201, registryPayload(reg, false))
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
	var req registryUpsertRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, 400, "invalid json")
		return
	}

	reg, err := buildRegistry(claims.OrgID, existing, req)
	if err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	reg.RegistryID = existing.RegistryID
	reg.CreatedAt = existing.CreatedAt

	if err := h.store.UpdateRegistry(r.Context(), reg); err != nil {
		writeErr(w, 500, "internal error")
		return
	}
	writeJSON(w, 200, registryPayload(reg, false))
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

func buildRegistry(orgID string, existing *domain.Registry, req registryUpsertRequest) (*domain.Registry, error) {
	name := strings.TrimSpace(req.Name)
	if name == "" {
		return nil, errors.New("name is required")
	}

	kind, err := normalizeRegistryKind(req.Kind)
	if err != nil {
		return nil, err
	}

	password := mergeRegistrySecretValue(req.Password, req.ClearPassword, existingRegistryPassword(existing))
	bearerToken := strings.TrimSpace(mergeRegistrySecretValue(req.BearerToken, req.ClearBearerToken, existingRegistryBearerToken(existing)))
	tlsCAData := mergeRegistrySecretValue(req.TLSCAData, req.ClearTLSCAData, existingRegistryTLSCAData(existing))
	tlsCertData := mergeRegistrySecretValue(req.TLSCertData, req.ClearTLSCertData, existingRegistryTLSCertData(existing))
	tlsKeyData := mergeRegistrySecretValue(req.TLSKeyData, req.ClearTLSKeyData, existingRegistryTLSKeyData(existing))

	connection := registryConnection{
		Kind:                  kind,
		URL:                   strings.TrimSpace(req.URL),
		InsecureSkipTLSVerify: req.InsecureSkipTLSVerify,
		Username:              strings.TrimSpace(req.Username),
		Password:              password,
		BearerToken:           bearerToken,
		TLSCAData:             strings.TrimSpace(tlsCAData),
		TLSCertData:           strings.TrimSpace(tlsCertData),
		TLSKeyData:            strings.TrimSpace(tlsKeyData),
	}
	if err := validateRegistryConnection(connection); err != nil {
		return nil, err
	}

	reg := &domain.Registry{
		OrgID:                 orgID,
		Kind:                  kind,
		Name:                  name,
		URL:                   connection.URL,
		InsecureSkipTLSVerify: connection.InsecureSkipTLSVerify,
		Username:              connection.Username,
		Token:                 connection.BearerToken,
		Password:              connection.Password,
		TLSCAData:             connection.TLSCAData,
		TLSCertData:           connection.TLSCertData,
		TLSKeyData:            connection.TLSKeyData,
		Enabled:               req.Enabled,
	}
	if existing != nil {
		reg.RegistryID = existing.RegistryID
		reg.CreatedAt = existing.CreatedAt
	}
	return reg, nil
}

func normalizeRegistryKind(value string) (domain.RegistryKind, error) {
	switch kind := domain.RegistryKind(strings.ToLower(strings.TrimSpace(value))); kind {
	case domain.RegistryGHCR, domain.RegistryJFrog:
		return kind, nil
	default:
		return "", errors.New("kind must be ghcr or jfrog")
	}
}

func validateRegistryConnection(connection registryConnection) error {
	endpointScheme, hasDefaultBase, err := validateRegistryURL(connection.Kind, connection.URL)
	if err != nil {
		return err
	}

	hasBasicAuth := connection.Username != "" || connection.Password != ""
	hasBearerToken := connection.BearerToken != ""
	hasClientCert := connection.TLSCertData != "" || connection.TLSKeyData != ""
	hasTLSConfig := connection.InsecureSkipTLSVerify || connection.TLSCAData != "" || hasClientCert

	if hasBasicAuth && connection.Username == "" {
		return errors.New("username is required when password is set")
	}
	if connection.Username != "" && connection.Password == "" {
		return errors.New("password is required when username is set")
	}
	if hasBasicAuth && hasBearerToken {
		return errors.New("choose either username/password or bearer token, not both")
	}
	if (connection.TLSCertData == "") != (connection.TLSKeyData == "") {
		return errors.New("tls_cert_data and tls_key_data must be provided together")
	}
	if connection.TLSCAData != "" {
		pool := x509.NewCertPool()
		if !pool.AppendCertsFromPEM([]byte(connection.TLSCAData)) {
			return errors.New("tls_ca_data must contain valid PEM certificates")
		}
	}
	if hasClientCert {
		if _, err := tls.X509KeyPair([]byte(connection.TLSCertData), []byte(connection.TLSKeyData)); err != nil {
			return errors.New("tls_cert_data and tls_key_data must contain a valid client certificate pair")
		}
	}

	if endpointScheme == "" {
		if hasTLSConfig || hasBasicAuth || hasBearerToken {
			if !hasDefaultBase {
				return errors.New("url is required when using authentication or tls settings")
			}
			endpointScheme = "https"
		}
	}
	if endpointScheme == "http" && hasTLSConfig {
		return errors.New("tls settings require an https registry url")
	}
	return nil
}

func validateRegistryURL(kind domain.RegistryKind, rawURL string) (string, bool, error) {
	trimmed := strings.TrimSpace(rawURL)
	if trimmed == "" {
		return "", registryDefaultURL(kind) != "", nil
	}
	parsed, err := url.ParseRequestURI(trimmed)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return "", false, errors.New("url must be a valid http or https registry url")
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return "", false, errors.New("url must use http or https")
	}
	return parsed.Scheme, false, nil
}

func registryDefaultURL(kind domain.RegistryKind) string {
	switch kind {
	case domain.RegistryGHCR:
		return "https://ghcr.io"
	case domain.RegistryJFrog:
		return "https://your-org.jfrog.io"
	default:
		return ""
	}
}

func mergeRegistrySecretValue(value string, clear bool, existing string) string {
	if value != "" {
		return value
	}
	if clear {
		return ""
	}
	return existing
}

func existingRegistryPassword(reg *domain.Registry) string {
	if reg == nil {
		return ""
	}
	return reg.Password
}

func existingRegistryBearerToken(reg *domain.Registry) string {
	if reg == nil {
		return ""
	}
	return reg.Token
}

func existingRegistryTLSCAData(reg *domain.Registry) string {
	if reg == nil {
		return ""
	}
	return reg.TLSCAData
}

func existingRegistryTLSCertData(reg *domain.Registry) string {
	if reg == nil {
		return ""
	}
	return reg.TLSCertData
}

func existingRegistryTLSKeyData(reg *domain.Registry) string {
	if reg == nil {
		return ""
	}
	return reg.TLSKeyData
}

func registryPayload(reg *domain.Registry, includeSecrets bool) *api.Registry {
	payload := &api.Registry{
		RegistryID:            reg.RegistryID,
		OrgID:                 reg.OrgID,
		Kind:                  string(reg.Kind),
		Name:                  reg.Name,
		URL:                   reg.URL,
		InsecureSkipTLSVerify: reg.InsecureSkipTLSVerify,
		Username:              reg.Username,
		HasPassword:           reg.Password != "",
		HasBearerToken:        reg.Token != "",
		HasTLSCAData:          reg.TLSCAData != "",
		HasTLSCertData:        reg.TLSCertData != "",
		HasTLSKeyData:         reg.TLSKeyData != "",
		Enabled:               reg.Enabled,
		CreatedAt:             reg.CreatedAt,
	}
	if includeSecrets {
		payload.Password = reg.Password
		payload.BearerToken = reg.Token
		payload.TLSCAData = reg.TLSCAData
		payload.TLSCertData = reg.TLSCertData
		payload.TLSKeyData = reg.TLSKeyData
	}
	return payload
}
