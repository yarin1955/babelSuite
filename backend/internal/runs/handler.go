package runs

import (
	"context"
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
	"github.com/babelsuite/babelsuite/internal/telemetry"
	"github.com/google/uuid"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

type Handler struct {
	store  store.Store
	jwt    *auth.JWTService
	logPub *LogPubSub
	runPub *RunPubSub
}

func NewHandler(s store.Store, jwt *auth.JWTService) *Handler {
	return &Handler{
		store:  s,
		jwt:    jwt,
		logPub: newLogPubSub(),
		runPub: newRunPubSub(),
	}
}

func (h *Handler) LogPubSub() *LogPubSub { return h.logPub }

func (h *Handler) RunPubSub() *RunPubSub { return h.runPub }

func (h *Handler) Register(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/runs", h.userMW(h.listRuns))
	mux.HandleFunc("POST /api/runs", h.userMW(h.createRun))
	mux.HandleFunc("GET /api/runs/{id}", h.userMW(h.getRun))
	mux.HandleFunc("DELETE /api/runs/{id}", h.userMW(h.cancelRun))
	mux.HandleFunc("GET /api/runs/{id}/steps", h.userMW(h.listSteps))
	mux.HandleFunc("GET /api/runs/{id}/logs/{stepID}", h.sseMW(h.streamLogs))
	mux.HandleFunc("GET /api/runs/{id}/logs/{stepID}/history", h.userMW(h.historyLogs))

	mux.HandleFunc("GET /api/agent/runs/next", h.agentMW(h.agentNext))
	mux.HandleFunc("GET /api/agent/runs/{id}/wait", h.agentMW(h.agentWaitRun))
	mux.HandleFunc("PATCH /api/agent/runs/{id}", h.agentMW(h.agentUpdateRun))
	mux.HandleFunc("POST /api/agent/runs/{id}/steps", h.agentMW(h.agentCreateStep))
	mux.HandleFunc("PATCH /api/agent/runs/{id}/steps/{stepID}", h.agentMW(h.agentUpdateStep))
	mux.HandleFunc("POST /api/agent/runs/{id}/steps/{stepID}/logs", h.agentMW(h.agentAppendLogs))
}

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
	ctx, span := h.startSpan(r, "runs.create_run")
	defer span.End()

	claims := auth.ClaimsFrom(r)
	var req struct {
		PackageID string `json:"package_id"`
		Profile   string `json:"profile"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		span.RecordError(err)
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	if req.PackageID == "" {
		writeErr(w, http.StatusBadRequest, "package_id is required")
		return
	}
	span.SetAttributes(attribute.String("catalog.package_id", req.PackageID))

	pkg, err := h.store.GetPackage(ctx, req.PackageID)
	if err != nil {
		span.RecordError(err)
		if errors.Is(err, store.ErrNotFound) {
			writeErr(w, http.StatusNotFound, "package not found")
		} else {
			writeErr(w, http.StatusInternalServerError, "internal error")
		}
		return
	}
	if !pkg.Enabled {
		writeErr(w, http.StatusForbidden, "package is not enabled")
		return
	}

	selectedProfile := strings.TrimSpace(req.Profile)
	if selectedProfile == "" {
		selectedProfile = strings.TrimSpace(pkg.DefaultProfile)
	}
	if len(pkg.Profiles) > 0 && selectedProfile != "" {
		matched := false
		for _, profile := range pkg.Profiles {
			if strings.EqualFold(strings.TrimSpace(profile), selectedProfile) {
				selectedProfile = strings.TrimSpace(profile)
				matched = true
				break
			}
		}
		if !matched {
			writeErr(w, http.StatusBadRequest, "profile is not published by this suite")
			return
		}
	}

	run := &domain.Run{
		RunID:     uuid.NewString(),
		OrgID:     claims.OrgID,
		PackageID: pkg.PackageID,
		ImageRef:  pkg.ImageRef,
		Profile:   selectedProfile,
		Status:    domain.RunPending,
		CreatedAt: time.Now().UTC(),
	}
	if err := h.store.CreateRun(ctx, run); err != nil {
		span.RecordError(err)
		writeErr(w, http.StatusInternalServerError, "internal error")
		return
	}
	span.SetAttributes(attribute.String("run.id", run.RunID))
	writeJSON(w, http.StatusCreated, run)
}

func (h *Handler) getRun(w http.ResponseWriter, r *http.Request) {
	ctx, span := h.startSpan(r, "runs.get_run", attribute.String("run.id", r.PathValue("id")))
	defer span.End()

	claims := auth.ClaimsFrom(r)
	run, err := h.store.GetRun(ctx, r.PathValue("id"))
	if err != nil {
		span.RecordError(err)
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
	domain.ApplyRunStatus(run, domain.RunCanceled, time.Now().UTC())
	if err := h.store.UpdateRun(r.Context(), run); err != nil {
		writeErr(w, http.StatusInternalServerError, "internal error")
		return
	}
	h.runPub.Publish(run)
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) listSteps(w http.ResponseWriter, r *http.Request) {
	ctx, span := h.startSpan(r, "runs.list_steps", attribute.String("run.id", r.PathValue("id")))
	defer span.End()

	claims := auth.ClaimsFrom(r)
	run, err := h.store.GetRun(ctx, r.PathValue("id"))
	if err != nil {
		span.RecordError(err)
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
	steps, err := h.store.ListSteps(ctx, run.RunID)
	if err != nil {
		span.RecordError(err)
		writeErr(w, http.StatusInternalServerError, "internal error")
		return
	}
	if steps == nil {
		steps = []*domain.Step{}
	}
	writeJSON(w, http.StatusOK, steps)
}

func (h *Handler) streamLogs(w http.ResponseWriter, r *http.Request) {
	ctx, span := h.startSpan(r, "runs.stream_logs",
		attribute.String("run.id", r.PathValue("id")),
		attribute.String("step.id", r.PathValue("stepID")),
	)
	defer span.End()

	claims := auth.ClaimsFrom(r)
	run, err := h.store.GetRun(ctx, r.PathValue("id"))
	if err != nil {
		span.RecordError(err)
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

	historical, err := h.store.GetLogs(ctx, stepID)
	if err != nil {
		span.RecordError(err)
		writeErr(w, http.StatusInternalServerError, "internal error")
		return
	}
	for _, e := range historical {
		if e.Line <= lastLine {
			continue
		}
		sendLogSSE(w, flusher, []*domain.LogEntry{e})
	}

	step := h.findStep(r.WithContext(ctx), run.RunID, stepID)
	if step != nil && step.Status.IsTerminal() {
		fmt.Fprintf(w, "event: eof\ndata: {}\n\n")
		flusher.Flush()
		return
	}

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
			if !open || entries == nil {
				fmt.Fprintf(w, "event: eof\ndata: {}\n\n")
				flusher.Flush()
				return
			}
			sendLogSSE(w, flusher, entries)
		case updatedRun, open := <-runCh:
			if !open || updatedRun.Status.IsTerminal() {
				fmt.Fprintf(w, "event: eof\ndata: {}\n\n")
				flusher.Flush()
				return
			}
		}
	}
}

func (h *Handler) historyLogs(w http.ResponseWriter, r *http.Request) {
	ctx, span := h.startSpan(r, "runs.history_logs",
		attribute.String("run.id", r.PathValue("id")),
		attribute.String("step.id", r.PathValue("stepID")),
	)
	defer span.End()

	claims := auth.ClaimsFrom(r)
	run, err := h.store.GetRun(ctx, r.PathValue("id"))
	if err != nil {
		span.RecordError(err)
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
	logs, err := h.store.GetLogs(ctx, r.PathValue("stepID"))
	if err != nil {
		span.RecordError(err)
		writeErr(w, http.StatusInternalServerError, "internal error")
		return
	}
	if logs == nil {
		logs = []*domain.LogEntry{}
	}
	writeJSON(w, http.StatusOK, logs)
}

func (h *Handler) agentNext(w http.ResponseWriter, r *http.Request) {
	ctx, span := h.startSpan(r, "runs.agent_next")
	defer span.End()

	a := agentFromCtx(r)
	if !agentReadyForScheduling(a) {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	span.SetAttributes(attribute.String("agent.id", a.AgentID))
	run, err := h.store.NextPendingRun(ctx, a.OrgID, a.AgentID)
	if err != nil {
		span.RecordError(err)
		if errors.Is(err, store.ErrNotFound) {
			w.WriteHeader(http.StatusNoContent)
		} else {
			writeErr(w, http.StatusInternalServerError, "internal error")
		}
		return
	}
	span.SetAttributes(attribute.String("run.id", run.RunID))
	h.touchAgent(ctx, a, true)
	h.runPub.Publish(run)
	writeJSON(w, http.StatusOK, run)
}

func (h *Handler) agentWaitRun(w http.ResponseWriter, r *http.Request) {
	ctx, span := h.startSpan(r, "runs.agent_wait",
		attribute.String("run.id", r.PathValue("id")),
	)
	defer span.End()

	a := agentFromCtx(r)
	span.SetAttributes(attribute.String("agent.id", a.AgentID))
	run, err := h.store.GetRun(ctx, r.PathValue("id"))
	if err != nil || run.OrgID != a.OrgID {
		if err != nil {
			span.RecordError(err)
		}
		writeErr(w, http.StatusNotFound, "not found")
		return
	}
	if run.AgentID != "" && run.AgentID != a.AgentID {
		writeErr(w, http.StatusForbidden, "forbidden")
		return
	}
	if run.Status.IsTerminal() {
		writeJSON(w, http.StatusOK, map[string]any{
			"canceled": run.Status == domain.RunCanceled,
			"status":   run.Status,
		})
		return
	}

	ch, cancel := h.runPub.Subscribe(run.RunID)
	defer cancel()

	timer := time.NewTimer(25 * time.Second)
	defer timer.Stop()

	for {
		select {
		case <-r.Context().Done():
			return
		case <-timer.C:
			w.WriteHeader(http.StatusNoContent)
			return
		case updatedRun, open := <-ch:
			if !open {
				w.WriteHeader(http.StatusNoContent)
				return
			}
			if updatedRun.Status.IsTerminal() {
				writeJSON(w, http.StatusOK, map[string]any{
					"canceled": updatedRun.Status == domain.RunCanceled,
					"status":   updatedRun.Status,
				})
				return
			}
		}
	}
}

func (h *Handler) agentUpdateRun(w http.ResponseWriter, r *http.Request) {
	ctx, span := h.startSpan(r, "runs.agent_update_run",
		attribute.String("run.id", r.PathValue("id")),
	)
	defer span.End()

	a := agentFromCtx(r)
	span.SetAttributes(attribute.String("agent.id", a.AgentID))
	run, err := h.store.GetRun(ctx, r.PathValue("id"))
	if err != nil || run.OrgID != a.OrgID {
		if err != nil {
			span.RecordError(err)
		}
		writeErr(w, http.StatusNotFound, "not found")
		return
	}
	var req struct {
		Status RunStatusReq `json:"status"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		span.RecordError(err)
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	domain.ApplyRunStatus(run, domain.RunStatus(req.Status), time.Now().UTC())
	if err := h.store.UpdateRun(ctx, run); err != nil {
		span.RecordError(err)
		writeErr(w, http.StatusInternalServerError, "internal error")
		return
	}
	h.touchAgent(ctx, a, true)
	h.runPub.Publish(run)
	writeJSON(w, http.StatusOK, run)
}

func (h *Handler) agentCreateStep(w http.ResponseWriter, r *http.Request) {
	ctx, span := h.startSpan(r, "runs.agent_create_step",
		attribute.String("run.id", r.PathValue("id")),
	)
	defer span.End()

	a := agentFromCtx(r)
	span.SetAttributes(attribute.String("agent.id", a.AgentID))
	run, err := h.store.GetRun(ctx, r.PathValue("id"))
	if err != nil || run.OrgID != a.OrgID {
		if err != nil {
			span.RecordError(err)
		}
		writeErr(w, http.StatusNotFound, "not found")
		return
	}
	var req struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		span.RecordError(err)
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	span.SetAttributes(attribute.String("step.name", req.Name))
	steps, err := h.store.ListSteps(ctx, run.RunID)
	if err != nil {
		span.RecordError(err)
		writeErr(w, http.StatusInternalServerError, "internal error")
		return
	}
	step := &domain.Step{
		StepID:   uuid.NewString(),
		RunID:    run.RunID,
		Name:     req.Name,
		Position: len(steps),
		Type:     "commands",
	}
	domain.ApplyStepStatus(step, domain.RunRunning, 0, "", time.Now().UTC())
	if err := h.store.CreateStep(ctx, step); err != nil {
		span.RecordError(err)
		writeErr(w, http.StatusInternalServerError, "internal error")
		return
	}
	span.SetAttributes(attribute.String("step.id", step.StepID))
	h.touchAgent(ctx, a, true)
	writeJSON(w, http.StatusCreated, step)
}

func (h *Handler) agentUpdateStep(w http.ResponseWriter, r *http.Request) {
	ctx, span := h.startSpan(r, "runs.agent_update_step",
		attribute.String("run.id", r.PathValue("id")),
		attribute.String("step.id", r.PathValue("stepID")),
	)
	defer span.End()

	a := agentFromCtx(r)
	span.SetAttributes(attribute.String("agent.id", a.AgentID))
	run, err := h.store.GetRun(ctx, r.PathValue("id"))
	if err != nil || run.OrgID != a.OrgID {
		if err != nil {
			span.RecordError(err)
		}
		writeErr(w, http.StatusNotFound, "not found")
		return
	}
	steps, err := h.store.ListSteps(ctx, run.RunID)
	if err != nil {
		span.RecordError(err)
		writeErr(w, http.StatusInternalServerError, "internal error")
		return
	}
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
		span.RecordError(err)
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	domain.ApplyStepStatus(step, domain.RunStatus(req.Status), req.ExitCode, req.Error, time.Now().UTC())
	if err := h.store.UpdateStep(ctx, step); err != nil {
		span.RecordError(err)
		writeErr(w, http.StatusInternalServerError, "internal error")
		return
	}
	if err := h.syncRunStatus(r.WithContext(ctx), run); err != nil {
		span.RecordError(err)
		writeErr(w, http.StatusInternalServerError, "internal error")
		return
	}
	h.touchAgent(ctx, a, true)
	h.logPub.Publish(step.StepID, nil)
	writeJSON(w, http.StatusOK, step)
}

func (h *Handler) agentAppendLogs(w http.ResponseWriter, r *http.Request) {
	ctx, span := h.startSpan(r, "runs.agent_append_logs",
		attribute.String("run.id", r.PathValue("id")),
		attribute.String("step.id", r.PathValue("stepID")),
	)
	defer span.End()

	a := agentFromCtx(r)
	span.SetAttributes(attribute.String("agent.id", a.AgentID))
	run, err := h.store.GetRun(ctx, r.PathValue("id"))
	if err != nil || run.OrgID != a.OrgID {
		if err != nil {
			span.RecordError(err)
		}
		writeErr(w, http.StatusNotFound, "not found")
		return
	}
	stepID := r.PathValue("stepID")

	var lines []struct {
		Line    int    `json:"line"`
		Data    string `json:"data"`
		Time    int64  `json:"time"`
		Type    int    `json:"type"`
		TraceID string `json:"trace_id"`
		SpanID  string `json:"span_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&lines); err != nil {
		span.RecordError(err)
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	span.SetAttributes(attribute.Int("log.batch_size", len(lines)))

	entries := make([]*domain.LogEntry, len(lines))
	for i, l := range lines {
		entries[i] = &domain.LogEntry{
			LogID:   uuid.NewString(),
			RunID:   run.RunID,
			StepID:  stepID,
			Line:    l.Line,
			Data:    l.Data,
			Time:    l.Time,
			Type:    domain.LogEntryType(l.Type),
			TraceID: l.TraceID,
			SpanID:  l.SpanID,
		}
	}
	if err := h.store.AppendLogs(ctx, entries); err != nil {
		span.RecordError(err)
		writeErr(w, http.StatusInternalServerError, "internal error")
		return
	}
	h.logPub.Publish(stepID, entries)
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) userMW(next http.HandlerFunc) http.HandlerFunc {
	return auth.Middleware(h.jwt)(http.HandlerFunc(next)).ServeHTTP
}

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

func (h *Handler) syncRunStatus(r *http.Request, run *domain.Run) error {
	if run.Status.IsTerminal() {
		return nil
	}

	steps, err := h.store.ListSteps(r.Context(), run.RunID)
	if err != nil {
		return err
	}

	nextStatus := domain.AggregateRunStatus(steps)
	if nextStatus == domain.RunPending && run.StartedAt != nil {
		nextStatus = domain.RunRunning
	}
	if nextStatus == run.Status && nextStatus != domain.RunRunning {
		return nil
	}

	domain.ApplyRunStatus(run, nextStatus, time.Now().UTC())
	if err := h.store.UpdateRun(r.Context(), run); err != nil {
		return err
	}
	h.runPub.Publish(run)
	return nil
}

func (h *Handler) touchAgent(ctx context.Context, agent *domain.Agent, didWork bool) {
	now := time.Now().UTC()
	agent.LastContact = now
	if didWork {
		agent.LastWork = &now
	}
	_ = h.store.UpdateAgent(ctx, agent)
}

type RunStatusReq = domain.RunStatus

func agentReadyForScheduling(agent *domain.Agent) bool {
	if agent.NoSchedule {
		return false
	}

	if desired := strings.TrimSpace(strings.ToLower(agent.DesiredBackend)); desired != "" {
		actual := strings.TrimSpace(strings.ToLower(agent.Backend))
		if actual == "" || actual != desired {
			return false
		}
	}

	if desired := strings.TrimSpace(strings.ToLower(agent.DesiredPlatform)); desired != "" {
		actual := strings.TrimSpace(strings.ToLower(agent.Platform))
		if actual == "" {
			return false
		}
		if actual != desired && !strings.HasPrefix(actual, desired+"/") {
			return false
		}
	}

	if desired := strings.TrimSpace(strings.ToLower(agent.DesiredTargetName)); desired != "" {
		actual := strings.TrimSpace(strings.ToLower(agent.TargetName))
		if actual == "" || actual != desired {
			return false
		}
	}

	if desired := strings.TrimSpace(agent.DesiredTargetURL); desired != "" {
		actual := strings.TrimSpace(agent.TargetURL)
		if actual == "" || actual != desired {
			return false
		}
	}

	return true
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

func (h *Handler) startSpan(r *http.Request, name string, attrs ...attribute.KeyValue) (context.Context, trace.Span) {
	return telemetry.Start(r.Context(), "babelsuite.runs", name, trace.WithAttributes(attrs...))
}
