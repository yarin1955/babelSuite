package agent

import (
	"encoding/json"
	"net/http"
)

func NewGateway(registry *Registry, coordinator *Coordinator) http.Handler {
	mux := http.NewServeMux()
	RegisterGateway(mux, registry, coordinator)
	return mux
}

func RegisterGateway(mux *http.ServeMux, registry *Registry, coordinator *Coordinator) {
	if mux == nil {
		return
	}

	mux.HandleFunc("GET /api/v1/agents", func(w http.ResponseWriter, _ *http.Request) {
		if registry == nil {
			writeGatewayJSON(w, http.StatusOK, map[string]any{"agents": []Registration{}})
			return
		}
		writeGatewayJSON(w, http.StatusOK, map[string]any{"agents": registry.List()})
	})
	mux.HandleFunc("POST /api/v1/agents/register", func(w http.ResponseWriter, r *http.Request) {
		if registry == nil {
			writeGatewayJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "registry unavailable"})
			return
		}
		var request RegisterRequest
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			writeGatewayJSON(w, http.StatusBadRequest, map[string]string{"error": "registration payload is invalid"})
			return
		}
		writeGatewayJSON(w, http.StatusCreated, registry.Register(request))
	})
	mux.HandleFunc("POST /api/v1/agents/{agentId}/heartbeat", func(w http.ResponseWriter, r *http.Request) {
		if registry == nil {
			writeGatewayJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "registry unavailable"})
			return
		}
		record, ok := registry.Heartbeat(r.PathValue("agentId"))
		if !ok {
			writeGatewayJSON(w, http.StatusNotFound, map[string]string{"error": "agent not found"})
			return
		}
		writeGatewayJSON(w, http.StatusOK, record)
	})
	mux.HandleFunc("DELETE /api/v1/agents/{agentId}", func(w http.ResponseWriter, r *http.Request) {
		if registry != nil {
			registry.Unregister(r.PathValue("agentId"))
		}
		writeGatewayJSON(w, http.StatusOK, map[string]string{"status": "removed"})
	})
	mux.HandleFunc("POST /api/v1/agent-control/claims/next", func(w http.ResponseWriter, r *http.Request) {
		if coordinator == nil {
			writeGatewayJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "coordinator unavailable"})
			return
		}
		var request ClaimRequest
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			writeGatewayJSON(w, http.StatusBadRequest, map[string]string{"error": "claim payload is invalid"})
			return
		}
		assignment, ok := coordinator.Claim(request.AgentID)
		if !ok {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		writeGatewayJSON(w, http.StatusOK, ClaimResponse{Assignment: assignment})
	})
	mux.HandleFunc("POST /api/v1/agent-control/jobs/{jobId}/lease", func(w http.ResponseWriter, r *http.Request) {
		if coordinator == nil {
			writeGatewayJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "coordinator unavailable"})
			return
		}
		var request LeaseRequest
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			writeGatewayJSON(w, http.StatusBadRequest, map[string]string{"error": "lease payload is invalid"})
			return
		}
		response, err := coordinator.Extend(r.PathValue("jobId"), request.AgentID)
		if err != nil {
			writeGatewayJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
			return
		}
		writeGatewayJSON(w, http.StatusOK, response)
	})
	mux.HandleFunc("POST /api/v1/agent-control/jobs/{jobId}/state", func(w http.ResponseWriter, r *http.Request) {
		if coordinator == nil {
			writeGatewayJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "coordinator unavailable"})
			return
		}
		var report StateReport
		if err := json.NewDecoder(r.Body).Decode(&report); err != nil {
			writeGatewayJSON(w, http.StatusBadRequest, map[string]string{"error": "state payload is invalid"})
			return
		}
		if err := coordinator.ReportState(r.PathValue("jobId"), report); err != nil {
			writeGatewayJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
			return
		}
		writeGatewayJSON(w, http.StatusOK, map[string]string{"status": "recorded"})
	})
	mux.HandleFunc("POST /api/v1/agent-control/jobs/{jobId}/logs", func(w http.ResponseWriter, r *http.Request) {
		if coordinator == nil {
			writeGatewayJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "coordinator unavailable"})
			return
		}
		var report LogReport
		if err := json.NewDecoder(r.Body).Decode(&report); err != nil {
			writeGatewayJSON(w, http.StatusBadRequest, map[string]string{"error": "log payload is invalid"})
			return
		}
		if err := coordinator.ReportLog(r.PathValue("jobId"), report); err != nil {
			writeGatewayJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
			return
		}
		writeGatewayJSON(w, http.StatusOK, map[string]string{"status": "recorded"})
	})
	mux.HandleFunc("POST /api/v1/agent-control/jobs/{jobId}/complete", func(w http.ResponseWriter, r *http.Request) {
		if coordinator == nil {
			writeGatewayJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "coordinator unavailable"})
			return
		}
		var request CompleteRequest
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			writeGatewayJSON(w, http.StatusBadRequest, map[string]string{"error": "complete payload is invalid"})
			return
		}
		if err := coordinator.Complete(r.PathValue("jobId"), request); err != nil {
			writeGatewayJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
			return
		}
		writeGatewayJSON(w, http.StatusOK, map[string]string{"status": "completed"})
	})
}

func writeGatewayJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}
