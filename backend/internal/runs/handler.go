package runs

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/babelsuite/babelsuite/internal/auth"
	"github.com/babelsuite/babelsuite/internal/domain"
	"github.com/babelsuite/babelsuite/internal/store"
	"github.com/google/uuid"
)

// Handler handles runs, steps, and real-time log streaming.
// It also exposes agent-facing endpoints that agents call during execution.
type Handler struct {
	store    store.Store
	jwt      *auth.JWTService
	logPub   *LogPubSub
	runPub   *RunPubSub
}

func NewHandler(s store.Store, jwt *auth.JWTService) *Handler {
	return &Handler{
		store:  s,
		jwt:    jwt,
		logPub: newLogPubSub(),
		runPub: newRunPubSub(),
	}
}

// LogPubSub returns the log pubsub so the agents handler can publish logs.
func (h *Handler) LogPubSub() *LogPubSub { return h.logPub }

// RunPubSub returns the run pubsub so the agents handler can publish run updates.
func (h *Handler) RunPubSub() *RunPubSub { return h.runPub }

func (h *Handler) Register(mux *http.ServeMux) {
	// User-facing (JWT auth)
	mux.HandleFunc("GET /api/runs",                            h.userMW(h.listRuns))
	mux.HandleFunc("POST /api/runs",                           h.userMW(h.createRun))
	mux.HandleFunc("GET /api/runs/{id}",                       h.userMW(h.getRun))
	mux.HandleFunc("DELETE /api/runs/{id}",                    h.userMW(h.cancelRun))
	mux.HandleFunc("GET /api/runs/{id}/steps",                 h.userMW(h.listSteps))
	mux.HandleFunc("GET /api/runs/{id}/logs/{stepID}",         h.sseMW(h.streamLogs))
	mux.HandleFunc("GET /api/runs/{id}/logs/{stepID}/history", h.userMW(h.historyLogs))

	// Agent-facing (agent token auth — handled in agents handler, but we expose
	// the endpoints here so they sit alongside the run domain)
	mux.HandleFunc("GET /api/agent/runs/next",                         h.agentMW(h.agentNext))
	mux.HandleFunc("PATCH /api/agent/runs/{id}",                       h.agentMW(h.agentUpdateRun))
	mux.HandleFunc("POST /api/agent/runs/{id}/steps",                  h.agentMW(h.agentCreateStep))
	mux.HandleFunc("PATCH /api/agent/runs/{id}/steps/{stepID}",        h.agentMW(h.agentUpdateStep))
	mux.HandleFunc("POST /api/agent/runs/{id}/steps/{stepID}/logs",    h.agentMW(h.agentAppendLogs))
}

// ── user endpoints ────────────────────────────────────────────────────────────

func (h *Handler) listRuns(w http.ResponseWriter, r *http.Request) {
	claims := auth.ClaimsFrom(r)
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	if page < 1 {
		page = 1
	}
	runs, total, err := h.store.ListRuns(r.Context(), claims.OrgID, page, 20)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "internal error")
		return
	}
	if runs == nil {
		runs = []*domain.Run{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"runs": runs, "total": total, "page": page})
}

func (h *Handler) createRun(w http.ResponseWriter, r *http.Request) {
	claims := auth.ClaimsFrom(r)
	var req struct {
		PackageID string `json:"package_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	if req.PackageID == "" {
		writeErr(w, http.StatusBadRequest, "package_id is required")
		return
	}

	pkg, err := h.store.GetPackage(r.Context(), req.PackageID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeErr(w, http.StatusNotFound, "package not found")
		} else {
			writeErr(w, http.StatusInternalServerError, "internal error")
		}
		return
	}

	run := &domain.Run{
		RunID:     uuid.NewString(),
		OrgID:     claims.OrgID,
		PackageID: pkg.PackageID,
		ImageRef:  pkg.ImageRef,
		Status:    domain.RunPending,
		CreatedAt: time.Now().UTC(),
	}
	if err := h.store.CreateRun(r.Context(), run); err != nil {
		writeErr(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusCreated, run)
}

func (h *Handler) getRun(w http.ResponseWriter, r *http.Request) {
	claims := auth.ClaimsFrom(r)
	run, err := h.store.GetRun(r.Context(), r.PathValue("id"))
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeErr(w, http.StatusNotFound, "not found")
		} else {
			writeErr(w, http.StatusInternalServerError, "internal error")
		}
		return
	}
	if run.OrgID != claims.OrgID {
		writeErr(w, http.StatusForbidden, "forbidden")
		return
	}
	writeJSON(w, http.StatusOK, run)
}

func (h *Handler) cancelRun(w http.ResponseWriter, r *http.Request) {
	claims := auth.ClaimsFrom(r)
	run, err := h.store.GetRun(r.Context(), r.PathValue("id"))
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeErr(w, http.StatusNotFound, "not found")
		} else {
			writeErr(w, http.StatusInternalServerError, "internal error")
		}
		return
	}
	if run.OrgID != claims.OrgID {
		writeErr(w, http.StatusForbidden, "forbidden")
		return
	}
	run.Status = domain.RunCanceled
	now := time.Now().UTC()
	run.FinishedAt = &now
	if err := h.store.UpdateRun(r.Context(), run); err != nil {
		writeErr(w, http.StatusInternalServerError, "internal error")
		return
	}
	h.runPub.Publish(run)
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) listSteps(w http.ResponseWriter, r *http.Request) {
	claims := auth.ClaimsFrom(r)
	run, err := h.store.GetRun(r.Context(), r.PathValue("id"))
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeErr(w, http.StatusNotFound, "not found")
		} else {
			writeErr(w, http.StatusInternalServerError, "internal error")
		}
		return
	}
	if run.OrgID != claims.OrgID {
		writeErr(w, http.StatusForbidden, "forbidden")
		return
	}
	steps, err := h.store.ListSteps(r.Context(), run.RunID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "internal error")
		return
	}
	if steps == nil {
		steps = []*domain.Step{}
	}
	writeJSON(w, http.StatusOK, steps)
}

// streamLogs opens an SSE stream that sends historical logs first, then
// tails live logs until the step finishes or the client disconnects.
func (h *Handler) streamLogs(w http.ResponseWriter, r *http.Request) {
	claims := auth.ClaimsFrom(r)
	run, err := h.store.GetRun(r.Context(), r.PathValue("id"))
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeErr(w, http.StatusNotFound, "not found")
		} else {
			writeErr(w, http.StatusInternalServerError, "internal error")
		}
		return
	}
	if run.OrgID != claims.OrgID {
		writeErr(w, http.StatusForbidden, "forbidden")
		return
	}
	stepID := r.PathValue("stepID")

	// Determine last line seen (for reconnection support via Last-Event-ID).
	lastLine := -1
	if raw := r.Header.Get("Last-Event-ID"); raw != "" {
		lastLine, _ = strconv.Atoi(raw)
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")

	flusher, ok := w.(http.Flusher)
	if !ok {
		writeErr(w, http.StatusInternalServerError, "streaming not supported")
		return
	}

	// Send historical logs.
	historical, _ := h.store.GetLogs(r.Context(), stepID)
	for _, e := range historical {
		if e.Line <= lastLine {
			continue
		}
		sendLogSSE(w, flusher, []*domain.LogEntry{e})
	}

	// If the step is already done, close immediately.
	step := h.findStep(r, run.RunID, stepID)
	if step != nil && isTerminal(step.Status) {
		fmt.Fprintf(w, "event: eof\ndata: {}\n\n")
		flusher.Flush()
		return
	}

	// Tail live logs via pubsub.
	ch, cancel := h.logPub.Subscribe(stepID)
	defer cancel()

	runCh, cancelRun := h.runPub.Subscribe(run.RunID)
	defer cancelRun()

	keepalive := time.NewTicker(25 * time.Second)
	defer keepalive.Stop()

	for {
		select {
		case <-r.Context().Done():
			return

		case <-keepalive.C:
			fmt.Fprintf(w, ": ping\n\n")
			flusher.Flush()

		case entries, open := <-ch:
			if !open {
				fmt.Fprintf(w, "event: eof\ndata: {}\n\n")
				flusher.Flush()
				return
			}
			sendLogSSE(w, flusher, entries)

		case updatedRun, open := <-runCh:
			if !open || isTerminal(updatedRun.Status) {
				fmt.Fprintf(w, "event: eof\ndata: {}\n\n")
				flusher.Flush()
				return
			}
		}
	}
}

// historyLogs returns all stored logs for a step as JSON (for finished steps).
func (h *Handler) historyLogs(w http.ResponseWriter, r *http.Request) {
	claims := auth.ClaimsFrom(r)
	run, err := h.store.GetRun(r.Context(), r.PathValue("id"))
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeErr(w, http.StatusNotFound, "not found")
		} else {
			writeErr(w, http.StatusInternalServerError, "internal error")
		}
		return
	}
	if run.OrgID != claims.OrgID {
		writeErr(w, http.StatusForbidden, "forbidden")
		return
	}
	logs, err := h.store.GetLogs(r.Context(), r.PathValue("stepID"))
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "internal error")
		return
	}
	if logs == nil {
		logs = []*domain.LogEntry{}
	}
	writeJSON(w, http.StatusOK, logs)
}

// ── agent endpoints ───────────────────────────────────────────────────────────

// agentNext returns the next pending run for the agent's org (atomic claim).
func (h *Handler) agentNext(w http.ResponseWriter, r *http.Request) {
	a := agentFromCtx(r)
	if a.NoSchedule {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	run, err := h.store.NextPendingRun(r.Context(), a.OrgID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			w.WriteHeader(http.StatusNoContent)
		} else {
			writeErr(w, http.StatusInternalServerError, "internal error")
		}
		return
	}
	// Assign agent
	run.AgentID = a.AgentID
	_ = h.store.UpdateRun(r.Context(), run)
	h.runPub.Publish(run)
	writeJSON(w, http.StatusOK, run)
}

// agentUpdateRun lets the agent report the final status of a run.
func (h *Handler) agentUpdateRun(w http.ResponseWriter, r *http.Request) {
	a := agentFromCtx(r)
	run, err := h.store.GetRun(r.Context(), r.PathValue("id"))
	if err != nil || run.OrgID != a.OrgID {
		writeErr(w, http.StatusNotFound, "not found")
		return
	}
	var req struct {
		Status RunStatusReq `json:"status"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	run.Status = domain.RunStatus(req.Status)
	now := time.Now().UTC()
	run.FinishedAt = &now
	if err := h.store.UpdateRun(r.Context(), run); err != nil {
		writeErr(w, http.StatusInternalServerError, "internal error")
		return
	}
	h.runPub.Publish(run)
	writeJSON(w, http.StatusOK, run)
}

// agentCreateStep is called by the agent when it starts a new container/step.
func (h *Handler) agentCreateStep(w http.ResponseWriter, r *http.Request) {
	a := agentFromCtx(r)
	run, err := h.store.GetRun(r.Context(), r.PathValue("id"))
	if err != nil || run.OrgID != a.OrgID {
		writeErr(w, http.StatusNotFound, "not found")
		return
	}
	var req struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	now := time.Now().UTC()
	step := &domain.Step{
		StepID:    uuid.NewString(),
		RunID:     run.RunID,
		Name:      req.Name,
		Status:    domain.RunRunning,
		StartedAt: &now,
	}
	if err := h.store.CreateStep(r.Context(), step); err != nil {
		writeErr(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusCreated, step)
}

// agentUpdateStep lets the agent report a step's completion.
func (h *Handler) agentUpdateStep(w http.ResponseWriter, r *http.Request) {
	a := agentFromCtx(r)
	run, err := h.store.GetRun(r.Context(), r.PathValue("id"))
	if err != nil || run.OrgID != a.OrgID {
		writeErr(w, http.StatusNotFound, "not found")
		return
	}
	steps, _ := h.store.ListSteps(r.Context(), run.RunID)
	var step *domain.Step
	for _, s := range steps {
		if s.StepID == r.PathValue("stepID") {
			step = s
			break
		}
	}
	if step == nil {
		writeErr(w, http.StatusNotFound, "step not found")
		return
	}
	var req struct {
		Status   RunStatusReq `json:"status"`
		ExitCode int          `json:"exit_code"`
		Error    string       `json:"error"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	now := time.Now().UTC()
	step.Status = domain.RunStatus(req.Status)
	step.ExitCode = req.ExitCode
	step.Error = req.Error
	step.FinishedAt = &now
	if err := h.store.UpdateStep(r.Context(), step); err != nil {
		writeErr(w, http.StatusInternalServerError, "internal error")
		return
	}
	// Close the log stream for this step so SSE clients get an EOF.
	h.logPub.Publish(step.StepID, nil)
	writeJSON(w, http.StatusOK, step)
}

// agentAppendLogs receives a batch of log entries from the agent,
// persists them, and fans them out to any SSE subscribers.
func (h *Handler) agentAppendLogs(w http.ResponseWriter, r *http.Request) {
	a := agentFromCtx(r)
	run, err := h.store.GetRun(r.Context(), r.PathValue("id"))
	if err != nil || run.OrgID != a.OrgID {
		writeErr(w, http.StatusNotFound, "not found")
		return
	}
	stepID := r.PathValue("stepID")

	var lines []struct {
		Line int    `json:"line"`
		Data string `json:"data"`
		Time int64  `json:"time"`
		Type int    `json:"type"`
	}
	if err := json.NewDecoder(r.Body).Decode(&lines); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}

	entries := make([]*domain.LogEntry, len(lines))
	for i, l := range lines {
		entries[i] = &domain.LogEntry{
			LogID:  uuid.NewString(),
			RunID:  run.RunID,
			StepID: stepID,
			Line:   l.Line,
			Data:   l.Data,
			Time:   l.Time,
			Type:   domain.LogEntryType(l.Type),
		}
	}
	if err := h.store.AppendLogs(r.Context(), entries); err != nil {
		writeErr(w, http.StatusInternalServerError, "internal error")
		return
	}
	h.logPub.Publish(stepID, entries)
	w.WriteHeader(http.StatusNoContent)
}

// ── middleware ────────────────────────────────────────────────────────────────

func (h *Handler) userMW(next http.HandlerFunc) http.HandlerFunc {
	return auth.Middleware(h.jwt)(http.HandlerFunc(next)).ServeHTTP
}

// sseMW is like userMW but also accepts the JWT via ?token= query param
// since EventSource in browsers cannot send custom headers.
func (h *Handler) sseMW(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") == "" {
			if qt := r.URL.Query().Get("token"); qt != "" {
				r.Header.Set("Authorization", "Bearer "+qt)
			}
		}
		auth.Middleware(h.jwt)(http.HandlerFunc(next)).ServeHTTP(w, r)
	}
}

// agentMW validates the agent bearer token and stores the agent in context.
// It duplicates the agents package logic here to avoid import cycles, using
// the store directly.
func (h *Handler) agentMW(next http.HandlerFunc) http.HandlerFunc {
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

func sendLogSSE(w http.ResponseWriter, f http.Flusher, entries []*domain.LogEntry) {
	if len(entries) == 0 {
		return
	}
	data, err := json.Marshal(entries)
	if err != nil {
		return
	}
	lastLine := entries[len(entries)-1].Line
	fmt.Fprintf(w, "id: %d\ndata: %s\n\n", lastLine, data)
	f.Flush()
}

func (h *Handler) findStep(r *http.Request, runID, stepID string) *domain.Step {
	steps, _ := h.store.ListSteps(r.Context(), runID)
	for _, s := range steps {
		if s.StepID == stepID {
			return s
		}
	}
	return nil
}

func isTerminal(s domain.RunStatus) bool {
	return s == domain.RunSuccess || s == domain.RunFailure ||
		s == domain.RunCanceled || s == domain.RunError
}

// RunStatusReq is a string alias so JSON can decode directly into RunStatus.
type RunStatusReq = domain.RunStatus

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}
