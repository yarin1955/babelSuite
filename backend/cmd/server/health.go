package main

import (
	"context"
	"encoding/json"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/babelsuite/babelsuite/internal/httpserver"
)

const (
	probeReady    = "ready"
	probeNotReady = "not_ready"
	probeDegraded = "degraded"
	probeDisabled = "disabled"
)

type probeCheck struct {
	Status string
	Detail string
}

type subsystemProbe struct {
	Name     string
	Required bool
	Check    func(context.Context) (probeCheck, error)
}

type subsystemStatus struct {
	Name       string    `json:"name"`
	Status     string    `json:"status"`
	Ready      bool      `json:"ready"`
	Required   bool      `json:"required"`
	Detail     string    `json:"detail,omitempty"`
	CheckedAt  time.Time `json:"checkedAt"`
	DurationMS int64     `json:"durationMs"`
}

type readinessReport struct {
	Status     string            `json:"status"`
	CheckedAt  time.Time         `json:"checkedAt"`
	Components []subsystemStatus `json:"components"`
}

type healthService struct {
	dbDriver string
	probes   map[string]subsystemProbe
	order    []string
}

func newHealthService(dbDriver string, probes []subsystemProbe) *healthService {
	service := &healthService{
		dbDriver: strings.TrimSpace(dbDriver),
		probes:   make(map[string]subsystemProbe, len(probes)),
		order:    make([]string, 0, len(probes)),
	}
	for _, probe := range probes {
		name := strings.TrimSpace(probe.Name)
		if name == "" {
			continue
		}
		probe.Name = name
		service.probes[name] = probe
		service.order = append(service.order, name)
	}
	sort.Strings(service.order)
	return service
}

func (s *healthService) Register(mux *http.ServeMux) {
	httpserver.HandleFunc(mux, "GET /healthz", s.liveness)
	httpserver.HandleFunc(mux, "GET /readyz", s.readiness)
	httpserver.HandleFunc(mux, "GET /readyz/{subsystem}", s.readinessSubsystem)
	httpserver.HandleFunc(mux, "GET /api/v1/system/healthz", s.liveness)
	httpserver.HandleFunc(mux, "GET /api/v1/system/readyz", s.readiness)
	httpserver.HandleFunc(mux, "GET /api/v1/system/readyz/{subsystem}", s.readinessSubsystem)
}

func (s *healthService) liveness(w http.ResponseWriter, _ *http.Request) {
	writeHealthJSON(w, http.StatusOK, map[string]any{
		"status":   "ok",
		"dbDriver": s.dbDriver,
		"time":     time.Now().UTC(),
	})
}

func (s *healthService) readiness(w http.ResponseWriter, r *http.Request) {
	report, statusCode := s.run(r.Context())
	writeHealthJSON(w, statusCode, report)
}

func (s *healthService) readinessSubsystem(w http.ResponseWriter, r *http.Request) {
	name := strings.TrimSpace(r.PathValue("subsystem"))
	if name == "" {
		writeHealthJSON(w, http.StatusBadRequest, map[string]string{"error": "subsystem is required"})
		return
	}

	probe, ok := s.probes[name]
	if !ok {
		writeHealthJSON(w, http.StatusNotFound, map[string]string{"error": "subsystem not found"})
		return
	}

	status := s.runProbe(r.Context(), probe)
	statusCode := http.StatusOK
	if probe.Required && !status.Ready {
		statusCode = http.StatusServiceUnavailable
	}
	writeHealthJSON(w, statusCode, status)
}

func (s *healthService) run(ctx context.Context) (readinessReport, int) {
	report := readinessReport{
		Status:     probeReady,
		CheckedAt:  time.Now().UTC(),
		Components: make([]subsystemStatus, 0, len(s.order)),
	}

	statusCode := http.StatusOK
	for _, name := range s.order {
		status := s.runProbe(ctx, s.probes[name])
		report.Components = append(report.Components, status)
		if status.Required && !status.Ready {
			report.Status = probeNotReady
			statusCode = http.StatusServiceUnavailable
		}
	}

	return report, statusCode
}

func (s *healthService) runProbe(parent context.Context, probe subsystemProbe) subsystemStatus {
	ctx, cancel := context.WithTimeout(parent, 2*time.Second)
	defer cancel()

	startedAt := time.Now()
	result := subsystemStatus{
		Name:      probe.Name,
		Required:  probe.Required,
		CheckedAt: startedAt.UTC(),
		Status:    probeReady,
		Ready:     true,
	}

	check, err := probe.Check(ctx)
	result.DurationMS = time.Since(startedAt).Milliseconds()
	if err != nil {
		result.Ready = false
		if probe.Required {
			result.Status = probeNotReady
		} else {
			result.Status = probeDegraded
		}
		result.Detail = err.Error()
		return result
	}

	if strings.TrimSpace(check.Status) != "" {
		result.Status = check.Status
	}
	result.Detail = strings.TrimSpace(check.Detail)
	result.Ready = result.Status == probeReady || result.Status == probeDisabled
	return result
}

func writeHealthJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}
