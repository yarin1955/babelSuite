package agents

import (
	"context"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
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
	"github.com/babelsuite/babelsuite/pkg/api"
	"github.com/google/uuid"
)

type Handler struct {
	store store.Store
	jwt   *auth.JWTService
}

type runtimeTargetUpsertRequest struct {
	Name                  string            `json:"name"`
	Backend               string            `json:"backend"`
	Platform              string            `json:"platform"`
	EndpointURL           string            `json:"endpoint_url"`
	Namespace             string            `json:"namespace"`
	InsecureSkipTLSVerify bool              `json:"insecure_skip_tls_verify"`
	Username              string            `json:"username"`
	Password              string            `json:"password"`
	BearerToken           string            `json:"bearer_token"`
	TLSCAData             string            `json:"tls_ca_data"`
	TLSCertData           string            `json:"tls_cert_data"`
	TLSKeyData            string            `json:"tls_key_data"`
	ClearPassword         bool              `json:"clear_password"`
	ClearBearerToken      bool              `json:"clear_bearer_token"`
	ClearTLSCAData        bool              `json:"clear_tls_ca_data"`
	ClearTLSCertData      bool              `json:"clear_tls_cert_data"`
	ClearTLSKeyData       bool              `json:"clear_tls_key_data"`
	Labels                map[string]string `json:"labels"`
}

type runtimeTargetConnection struct {
	Backend               string
	EndpointURL           string
	InsecureSkipTLSVerify bool
	Username              string
	Password              string
	BearerToken           string
	TLSCAData             string
	TLSCertData           string
	TLSKeyData            string
}

func NewHandler(s store.Store, jwt *auth.JWTService) *Handler {
	return &Handler{store: s, jwt: jwt}
}

func (h *Handler) Register(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/runtime-targets", h.adminMiddleware(h.listTargets))
	mux.HandleFunc("POST /api/runtime-targets", h.adminMiddleware(h.createTarget))
	mux.HandleFunc("GET /api/runtime-targets/{id}", h.adminMiddleware(h.getTarget))
	mux.HandleFunc("PUT /api/runtime-targets/{id}", h.adminMiddleware(h.updateTarget))
	mux.HandleFunc("DELETE /api/runtime-targets/{id}", h.adminMiddleware(h.deleteTarget))

	mux.HandleFunc("GET /api/agents", h.adminMiddleware(h.list))
	mux.HandleFunc("POST /api/agents", h.adminMiddleware(h.create))
	mux.HandleFunc("GET /api/agents/{id}", h.adminMiddleware(h.get))
	mux.HandleFunc("PATCH /api/agents/{id}", h.adminMiddleware(h.update))
	mux.HandleFunc("DELETE /api/agents/{id}", h.adminMiddleware(h.delete))

	mux.HandleFunc("GET /api/agent/bootstrap", h.agentMiddleware(h.agentBootstrap))
	mux.HandleFunc("POST /api/agent/register", h.agentMiddleware(h.agentRegister))
	mux.HandleFunc("POST /api/agent/health", h.agentMiddleware(h.agentHealth))
	mux.HandleFunc("GET /api/agent/next", h.agentMiddleware(h.agentNext))
}

func (h *Handler) listTargets(w http.ResponseWriter, r *http.Request) {
	claims := auth.ClaimsFrom(r)
	targets, err := h.store.ListRuntimeTargets(r.Context(), claims.OrgID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "internal error")
		return
	}
	if targets == nil {
		targets = []*domain.RuntimeTarget{}
	}
	payload := make([]*api.RuntimeTarget, 0, len(targets))
	for _, target := range targets {
		payload = append(payload, runtimeTargetPayload(target))
	}
	writeJSON(w, http.StatusOK, payload)
}

func (h *Handler) getTarget(w http.ResponseWriter, r *http.Request) {
	claims := auth.ClaimsFrom(r)
	target, err := h.store.GetRuntimeTarget(r.Context(), r.PathValue("id"))
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeErr(w, http.StatusNotFound, "runtime target not found")
		} else {
			writeErr(w, http.StatusInternalServerError, "internal error")
		}
		return
	}
	if target.OrgID != claims.OrgID {
		writeErr(w, http.StatusForbidden, "forbidden")
		return
	}
	writeJSON(w, http.StatusOK, runtimeTargetPayload(target))
}

func (h *Handler) createTarget(w http.ResponseWriter, r *http.Request) {
	claims := auth.ClaimsFrom(r)
	var req runtimeTargetUpsertRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}

	target, err := buildRuntimeTarget(claims.OrgID, nil, req)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	target.RuntimeTargetID = uuid.NewString()
	target.CreatedAt = time.Now().UTC()
	target.UpdatedAt = target.CreatedAt

	if err := h.store.CreateRuntimeTarget(r.Context(), target); err != nil {
		if errors.Is(err, store.ErrDuplicate) {
			writeErr(w, http.StatusConflict, "runtime target name already exists")
		} else {
			writeErr(w, http.StatusInternalServerError, "internal error")
		}
		return
	}
	writeJSON(w, http.StatusCreated, runtimeTargetPayload(target))
}

func (h *Handler) updateTarget(w http.ResponseWriter, r *http.Request) {
	claims := auth.ClaimsFrom(r)
	target, err := h.store.GetRuntimeTarget(r.Context(), r.PathValue("id"))
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeErr(w, http.StatusNotFound, "runtime target not found")
		} else {
			writeErr(w, http.StatusInternalServerError, "internal error")
		}
		return
	}
	if target.OrgID != claims.OrgID {
		writeErr(w, http.StatusForbidden, "forbidden")
		return
	}

	var req runtimeTargetUpsertRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}

	updated, err := buildRuntimeTarget(claims.OrgID, target, req)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	updated.RuntimeTargetID = target.RuntimeTargetID
	updated.CreatedAt = target.CreatedAt
	updated.UpdatedAt = time.Now().UTC()

	if err := h.store.UpdateRuntimeTarget(r.Context(), updated); err != nil {
		if errors.Is(err, store.ErrDuplicate) {
			writeErr(w, http.StatusConflict, "runtime target name already exists")
		} else {
			writeErr(w, http.StatusInternalServerError, "internal error")
		}
		return
	}
	if err := h.syncLinkedAgents(r.Context(), updated); err != nil {
		writeErr(w, http.StatusInternalServerError, "runtime target saved, but linked agents could not be updated")
		return
	}
	writeJSON(w, http.StatusOK, runtimeTargetPayload(updated))
}

func (h *Handler) deleteTarget(w http.ResponseWriter, r *http.Request) {
	claims := auth.ClaimsFrom(r)
	target, err := h.store.GetRuntimeTarget(r.Context(), r.PathValue("id"))
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeErr(w, http.StatusNotFound, "runtime target not found")
		} else {
			writeErr(w, http.StatusInternalServerError, "internal error")
		}
		return
	}
	if target.OrgID != claims.OrgID {
		writeErr(w, http.StatusForbidden, "forbidden")
		return
	}

	agents, err := h.store.ListAgents(r.Context(), claims.OrgID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "internal error")
		return
	}
	for _, agent := range agents {
		if agent.RuntimeTargetID == target.RuntimeTargetID {
			writeErr(w, http.StatusConflict, "runtime target is still assigned to one or more agents")
			return
		}
	}

	if err := h.store.DeleteRuntimeTarget(r.Context(), target.RuntimeTargetID); err != nil {
		writeErr(w, http.StatusInternalServerError, "internal error")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

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
		Name            string            `json:"name"`
		RuntimeTargetID string            `json:"runtime_target_id"`
		Capacity        int               `json:"capacity"`
		Labels          map[string]string `json:"labels"`
		NoSchedule      bool              `json:"no_schedule"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	if strings.TrimSpace(req.Name) == "" {
		writeErr(w, http.StatusBadRequest, "name is required")
		return
	}
	if strings.TrimSpace(req.RuntimeTargetID) == "" {
		writeErr(w, http.StatusBadRequest, "runtime_target_id is required")
		return
	}
	if req.Capacity <= 0 {
		req.Capacity = 1
	}

	token, err := generateToken()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "internal error")
		return
	}

	agent := &domain.Agent{
		AgentID:     uuid.NewString(),
		OrgID:       claims.OrgID,
		Name:        strings.TrimSpace(req.Name),
		Token:       token,
		Capacity:    req.Capacity,
		Labels:      req.Labels,
		NoSchedule:  req.NoSchedule,
		LastContact: time.Now().UTC(),
		CreatedAt:   time.Now().UTC(),
	}
	if err := h.applyAgentRuntimeTarget(r.Context(), agent, claims.OrgID, req.RuntimeTargetID); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}

	if err := h.store.CreateAgent(r.Context(), agent); err != nil {
		writeErr(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{
		"agent": agent,
		"token": token,
	})
}

func (h *Handler) get(w http.ResponseWriter, r *http.Request) {
	claims := auth.ClaimsFrom(r)
	agent, err := h.store.GetAgent(r.Context(), r.PathValue("id"))
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeErr(w, http.StatusNotFound, "not found")
		} else {
			writeErr(w, http.StatusInternalServerError, "internal error")
		}
		return
	}
	if agent.OrgID != claims.OrgID {
		writeErr(w, http.StatusForbidden, "forbidden")
		return
	}
	writeJSON(w, http.StatusOK, agent)
}

func (h *Handler) update(w http.ResponseWriter, r *http.Request) {
	claims := auth.ClaimsFrom(r)
	agent, err := h.store.GetAgent(r.Context(), r.PathValue("id"))
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeErr(w, http.StatusNotFound, "not found")
		} else {
			writeErr(w, http.StatusInternalServerError, "internal error")
		}
		return
	}
	if agent.OrgID != claims.OrgID {
		writeErr(w, http.StatusForbidden, "forbidden")
		return
	}

	var req struct {
		Name            *string           `json:"name"`
		RuntimeTargetID *string           `json:"runtime_target_id"`
		NoSchedule      *bool             `json:"no_schedule"`
		Capacity        *int              `json:"capacity"`
		Labels          map[string]string `json:"labels"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}

	if req.Name != nil {
		agent.Name = strings.TrimSpace(*req.Name)
	}
	if req.NoSchedule != nil {
		agent.NoSchedule = *req.NoSchedule
	}
	if req.Capacity != nil && *req.Capacity > 0 {
		agent.Capacity = *req.Capacity
	}
	if req.Labels != nil {
		agent.Labels = req.Labels
	}

	if req.RuntimeTargetID != nil {
		if strings.TrimSpace(*req.RuntimeTargetID) == "" {
			writeErr(w, http.StatusBadRequest, "runtime_target_id cannot be empty")
			return
		}
		if err := h.applyAgentRuntimeTarget(r.Context(), agent, claims.OrgID, *req.RuntimeTargetID); err != nil {
			writeErr(w, http.StatusBadRequest, err.Error())
			return
		}
	} else if agent.RuntimeTargetID == "" {
		writeErr(w, http.StatusBadRequest, "legacy agents are no longer supported; assign a runtime target or delete this agent")
		return
	}

	if err := h.store.UpdateAgent(r.Context(), agent); err != nil {
		writeErr(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, agent)
}

func (h *Handler) delete(w http.ResponseWriter, r *http.Request) {
	claims := auth.ClaimsFrom(r)
	agent, err := h.store.GetAgent(r.Context(), r.PathValue("id"))
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeErr(w, http.StatusNotFound, "not found")
		} else {
			writeErr(w, http.StatusInternalServerError, "internal error")
		}
		return
	}
	if agent.OrgID != claims.OrgID {
		writeErr(w, http.StatusForbidden, "forbidden")
		return
	}
	active, err := h.store.CountActiveRunsByAgent(r.Context(), agent.AgentID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "internal error")
		return
	}
	if active > 0 {
		writeErr(w, http.StatusConflict, "agent has active runs")
		return
	}
	if err := h.store.DeleteAgent(r.Context(), agent.AgentID); err != nil {
		writeErr(w, http.StatusInternalServerError, "internal error")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) agentRegister(w http.ResponseWriter, r *http.Request) {
	agent := agentFrom(r)
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
		agent.Name = req.Name
	}
	agent.Platform = req.Platform
	agent.Backend = req.Backend
	agent.TargetName = normalizeTargetName(req.TargetName)
	agent.TargetURL = normalizeTargetURL(req.TargetURL)
	if req.Capacity > 0 {
		agent.Capacity = req.Capacity
	}
	agent.Version = req.Version
	if req.Labels != nil {
		agent.Labels = req.Labels
	}
	agent.LastContact = time.Now().UTC()
	if err := h.store.UpdateAgent(r.Context(), agent); err != nil {
		writeErr(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"agent_id": agent.AgentID})
}

func (h *Handler) agentBootstrap(w http.ResponseWriter, r *http.Request) {
	agent := agentFrom(r)

	resp := &api.AgentBootstrapResponse{
		AgentID:    agent.AgentID,
		AgentName:  agent.Name,
		Capacity:   agent.Capacity,
		NoSchedule: agent.NoSchedule,
	}

	if strings.TrimSpace(agent.RuntimeTargetID) == "" {
		resp.WorkerBootstrapReason = "this agent does not have a runtime target assigned"
		writeJSON(w, http.StatusOK, resp)
		return
	}

	target, err := h.store.GetRuntimeTarget(r.Context(), agent.RuntimeTargetID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			resp.WorkerBootstrapReason = "the assigned runtime target no longer exists"
			writeJSON(w, http.StatusOK, resp)
			return
		}
		writeErr(w, http.StatusInternalServerError, "internal error")
		return
	}
	if target.OrgID != agent.OrgID {
		writeErr(w, http.StatusForbidden, "forbidden")
		return
	}

	resp.RuntimeTarget = apiRuntimeTarget(target, true)
	resp.RunnerBackend = runtimeTargetRunnerBackend(target)
	resp.WorkerBootstrapSupported, resp.WorkerBootstrapReason = runtimeTargetWorkerSupport(target)
	writeJSON(w, http.StatusOK, resp)
}

func (h *Handler) agentHealth(w http.ResponseWriter, r *http.Request) {
	agent := agentFrom(r)
	agent.LastContact = time.Now().UTC()
	if err := h.store.UpdateAgent(r.Context(), agent); err != nil {
		writeErr(w, http.StatusInternalServerError, "internal error")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) agentNext(w http.ResponseWriter, r *http.Request) {
	agent := agentFrom(r)
	if agent.NoSchedule {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) applyAgentRuntimeTarget(ctx context.Context, agent *domain.Agent, orgID, runtimeTargetID string) error {
	runtimeTargetID = strings.TrimSpace(runtimeTargetID)
	if runtimeTargetID == "" {
		return errors.New("runtime_target_id is required")
	}
	target, err := h.store.GetRuntimeTarget(ctx, runtimeTargetID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return errors.New("runtime_target_id does not exist")
		}
		return err
	}
	if target.OrgID != orgID {
		return errors.New("runtime_target_id is not part of this workspace")
	}
	if supported, reason := runtimeTargetWorkerSupport(target); !supported {
		return errors.New(reason)
	}
	applyRuntimeTarget(agent, target)
	return nil
}

func applyRuntimeTarget(agent *domain.Agent, target *domain.RuntimeTarget) {
	agent.RuntimeTargetID = target.RuntimeTargetID
	agent.DesiredBackend = runtimeTargetRunnerBackend(target)
	agent.DesiredPlatform = target.Platform
	agent.DesiredTargetName = target.Name
	agent.DesiredTargetURL = target.EndpointURL
}

func buildRuntimeTarget(orgID string, existing *domain.RuntimeTarget, req runtimeTargetUpsertRequest) (*domain.RuntimeTarget, error) {
	normalizedName := strings.TrimSpace(req.Name)
	if normalizedName == "" {
		return nil, errors.New("name is required")
	}
	if len(normalizedName) > 80 {
		return nil, errors.New("name must be 80 characters or fewer")
	}
	normalizedBackend, err := normalizeDesiredBackend(defaultDesiredBackend(req.Backend))
	if err != nil {
		return nil, err
	}
	normalizedPlatform := normalizeDesiredPlatform(req.Platform)
	normalizedEndpointURL := normalizeTargetURL(req.EndpointURL)
	normalizedNamespace := strings.TrimSpace(req.Namespace)
	password := mergeSecretValue(req.Password, req.ClearPassword, existingPassword(existing))
	bearerToken := strings.TrimSpace(mergeSecretValue(req.BearerToken, req.ClearBearerToken, existingBearerToken(existing)))
	tlsCAData := mergeSecretValue(req.TLSCAData, req.ClearTLSCAData, existingTLSCAData(existing))
	tlsCertData := mergeSecretValue(req.TLSCertData, req.ClearTLSCertData, existingTLSCertData(existing))
	tlsKeyData := mergeSecretValue(req.TLSKeyData, req.ClearTLSKeyData, existingTLSKeyData(existing))

	connection := runtimeTargetConnection{
		Backend:               normalizedBackend,
		EndpointURL:           normalizedEndpointURL,
		InsecureSkipTLSVerify: req.InsecureSkipTLSVerify,
		Username:              strings.TrimSpace(req.Username),
		Password:              password,
		BearerToken:           bearerToken,
		TLSCAData:             strings.TrimSpace(tlsCAData),
		TLSCertData:           strings.TrimSpace(tlsCertData),
		TLSKeyData:            strings.TrimSpace(tlsKeyData),
	}
	if err := validateRuntimeTarget(connection); err != nil {
		return nil, err
	}

	target := &domain.RuntimeTarget{
		OrgID:                 orgID,
		Name:                  normalizedName,
		Backend:               normalizedBackend,
		Platform:              normalizedPlatform,
		EndpointURL:           normalizedEndpointURL,
		Namespace:             normalizedNamespace,
		InsecureSkipTLSVerify: connection.InsecureSkipTLSVerify,
		Username:              connection.Username,
		Password:              connection.Password,
		BearerToken:           connection.BearerToken,
		TLSCAData:             connection.TLSCAData,
		TLSCertData:           connection.TLSCertData,
		TLSKeyData:            connection.TLSKeyData,
		Labels:                req.Labels,
	}
	if existing != nil {
		target.RuntimeTargetID = existing.RuntimeTargetID
		target.CreatedAt = existing.CreatedAt
		target.UpdatedAt = existing.UpdatedAt
	}
	return target, nil
}

func validateRuntimeTarget(connection runtimeTargetConnection) error {
	endpointScheme := ""
	if trimmedEndpoint := strings.TrimSpace(connection.EndpointURL); trimmedEndpoint != "" {
		var err error
		switch connection.Backend {
		case "docker", "local":
			endpointScheme, err = validateContainerEndpoint(trimmedEndpoint)
		case "kubernetes":
			endpointScheme, err = validateClusterEndpoint(trimmedEndpoint)
		}
		if err != nil {
			return err
		}
	}
	if connection.Backend == "kubernetes" && strings.TrimSpace(connection.EndpointURL) == "" {
		return errors.New("kubernetes targets must set endpoint_url")
	}
	if err := validateRuntimeTargetSecurity(connection, endpointScheme); err != nil {
		return err
	}
	return nil
}

func validateContainerEndpoint(endpointURL string) (string, error) {
	parsed, err := url.Parse(strings.TrimSpace(endpointURL))
	if err != nil || parsed.Scheme == "" {
		return "", errors.New("endpoint_url must be a valid daemon URL such as unix:///var/run/docker.sock or tcp://host:2376")
	}
	switch parsed.Scheme {
	case "unix", "npipe":
		if parsed.Path == "" {
			return "", errors.New("endpoint_url must include a socket path")
		}
	case "tcp", "http", "https", "ssh":
		if parsed.Host == "" {
			return "", errors.New("endpoint_url must include a host")
		}
	default:
		return "", errors.New("endpoint_url must use unix, npipe, tcp, http, https, or ssh")
	}
	return parsed.Scheme, nil
}

func validateClusterEndpoint(endpointURL string) (string, error) {
	parsed, err := url.ParseRequestURI(strings.TrimSpace(endpointURL))
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return "", errors.New("endpoint_url must be a valid cluster API server URL")
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return "", errors.New("endpoint_url must use http or https for cluster targets")
	}
	return parsed.Scheme, nil
}

func validateRuntimeTargetSecurity(connection runtimeTargetConnection, endpointScheme string) error {
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
		return errors.New("choose either basic auth or bearer token, not both")
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
	if endpointScheme == "http" && hasTLSConfig {
		return errors.New("tls settings require an https or tcp endpoint_url")
	}

	if connection.Backend == "docker" || connection.Backend == "local" {
		if endpointScheme == "" {
			if hasBasicAuth || hasBearerToken || hasTLSConfig {
				return errors.New("remote tls or credentials require endpoint_url to be set")
			}
			return nil
		}
		switch endpointScheme {
		case "tcp", "http", "https":
			return nil
		default:
			if hasBasicAuth || hasBearerToken || hasTLSConfig {
				return errors.New("tls and credentials are only supported for tcp, http, or https daemon endpoints")
			}
		}
	}

	return nil
}

func mergeSecretValue(value string, clear bool, existing string) string {
	if value != "" {
		return value
	}
	if clear {
		return ""
	}
	return existing
}

func existingPassword(target *domain.RuntimeTarget) string {
	if target == nil {
		return ""
	}
	return target.Password
}

func existingBearerToken(target *domain.RuntimeTarget) string {
	if target == nil {
		return ""
	}
	return target.BearerToken
}

func existingTLSCAData(target *domain.RuntimeTarget) string {
	if target == nil {
		return ""
	}
	return target.TLSCAData
}

func existingTLSCertData(target *domain.RuntimeTarget) string {
	if target == nil {
		return ""
	}
	return target.TLSCertData
}

func existingTLSKeyData(target *domain.RuntimeTarget) string {
	if target == nil {
		return ""
	}
	return target.TLSKeyData
}

func (h *Handler) syncLinkedAgents(ctx context.Context, target *domain.RuntimeTarget) error {
	agents, err := h.store.ListAgents(ctx, target.OrgID)
	if err != nil {
		return err
	}
	for _, agent := range agents {
		if agent.RuntimeTargetID != target.RuntimeTargetID {
			continue
		}
		applyRuntimeTarget(agent, target)
		if err := h.store.UpdateAgent(ctx, agent); err != nil {
			return err
		}
	}
	return nil
}

func agentFrom(r *http.Request) *domain.Agent {
	return agentFromContext(r.Context())
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
		agent, err := h.store.GetAgentByToken(r.Context(), token)
		if err != nil {
			writeErr(w, http.StatusUnauthorized, "invalid agent token")
			return
		}
		next(w, r.WithContext(contextWithAgent(r.Context(), agent)))
	}
}

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

func runtimeTargetPayload(target *domain.RuntimeTarget) *api.RuntimeTarget {
	return apiRuntimeTarget(target, false)
}

func apiRuntimeTarget(target *domain.RuntimeTarget, includeSecrets bool) *api.RuntimeTarget {
	supported, reason := runtimeTargetWorkerSupport(target)
	payload := &api.RuntimeTarget{
		RuntimeTargetID:          target.RuntimeTargetID,
		OrgID:                    target.OrgID,
		Name:                     target.Name,
		Backend:                  target.Backend,
		Platform:                 target.Platform,
		EndpointURL:              target.EndpointURL,
		Namespace:                target.Namespace,
		RunnerBackend:            runtimeTargetRunnerBackend(target),
		WorkerBootstrapSupported: supported,
		WorkerBootstrapReason:    reason,
		InsecureSkipTLSVerify:    target.InsecureSkipTLSVerify,
		Username:                 target.Username,
		HasPassword:              target.Password != "",
		HasBearerToken:           target.BearerToken != "",
		HasTLSCAData:             target.TLSCAData != "",
		HasTLSCertData:           target.TLSCertData != "",
		HasTLSKeyData:            target.TLSKeyData != "",
		Labels:                   target.Labels,
		CreatedAt:                target.CreatedAt,
		UpdatedAt:                target.UpdatedAt,
	}
	if includeSecrets {
		payload.Password = target.Password
		payload.BearerToken = target.BearerToken
		payload.TLSCAData = target.TLSCAData
		payload.TLSCertData = target.TLSCertData
		payload.TLSKeyData = target.TLSKeyData
	}
	return payload
}

func runtimeTargetRunnerBackend(target *domain.RuntimeTarget) string {
	switch strings.ToLower(strings.TrimSpace(target.Backend)) {
	case "local":
		return "docker"
	default:
		return strings.ToLower(strings.TrimSpace(target.Backend))
	}
}

func runtimeTargetWorkerSupport(target *domain.RuntimeTarget) (bool, string) {
	switch runtimeTargetRunnerBackend(target) {
	case "docker":
		return true, ""
	case "kubernetes":
		return false, "this runtime target is configured as a cluster destination and cannot be assigned to workers until cluster execution is available"
	default:
		return false, "this runtime target cannot be assigned to workers"
	}
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}
