package agent

import (
	"sort"
	"strings"
	"sync"
	"time"
)

type Registry struct {
	store RuntimeStore

	mu     sync.RWMutex
	agents map[string]Registration
}

func NewRegistry(store RuntimeStore) *Registry {
	return &Registry{
		store:  store,
		agents: make(map[string]Registration),
	}
}

func (r *Registry) Register(request RegisterRequest) Registration {
	now := time.Now().UTC()
	if settings, err := r.loadSettings(); err == nil {
		record := upsertAgentRegistration(settings, request, now)
		if err := r.store.Save(settings); err == nil {
			return record
		}
	}

	record := Registration{
		AgentID:         request.AgentID,
		Name:            request.Name,
		HostURL:         request.HostURL,
		Status:          "online",
		Capabilities:    append([]string{}, request.Capabilities...),
		RegisteredAt:    now,
		LastHeartbeatAt: now,
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	if existing, ok := r.agents[request.AgentID]; ok {
		record.RegisteredAt = existing.RegisteredAt
	}
	r.agents[request.AgentID] = record
	return record
}

func (r *Registry) Heartbeat(agentID string) (Registration, bool) {
	now := time.Now().UTC()
	if settings, err := r.loadSettings(); err == nil {
		record, ok := updateAgentHeartbeat(settings, agentID, now)
		if !ok {
			return Registration{}, false
		}
		if err := r.store.Save(settings); err == nil {
			return record, true
		}
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	record, ok := r.agents[agentID]
	if !ok {
		return Registration{}, false
	}
	record.LastHeartbeatAt = now
	record.Status = "online"
	r.agents[agentID] = record
	return record, true
}

func (r *Registry) Unregister(agentID string) {
	if settings, err := r.loadSettings(); err == nil {
		if markAgentOffline(settings, agentID) {
			_ = r.store.Save(settings)
			return
		}
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	if record, ok := r.agents[agentID]; ok {
		record.Status = "offline"
		r.agents[agentID] = record
	}
}

func (r *Registry) List() []Registration {
	if settings, err := r.loadSettings(); err == nil {
		records := append([]Registration{}, settings.Agents...)
		sortRegistrations(records)
		return records
	}

	r.mu.RLock()
	defer r.mu.RUnlock()
	records := make([]Registration, 0, len(r.agents))
	for _, record := range r.agents {
		records = append(records, record)
	}
	sortRegistrations(records)
	return records
}

func (r *Registry) Get(agentID string) (Registration, bool) {
	if settings, err := r.loadSettings(); err == nil {
		for _, agent := range settings.Agents {
			if strings.TrimSpace(agent.AgentID) == strings.TrimSpace(agentID) {
				return agent, true
			}
		}
		return Registration{}, false
	}

	r.mu.RLock()
	defer r.mu.RUnlock()
	record, ok := r.agents[agentID]
	return record, ok
}

func (r *Registry) IsAvailable(agentID string) bool {
	record, ok := r.Get(agentID)
	if !ok {
		return false
	}
	status := strings.ToLower(strings.TrimSpace(record.Status))
	return status == "online" || status == "ready"
}

func (r *Registry) loadSettings() (*RuntimeState, error) {
	if r.store == nil {
		return nil, ErrRuntimeNotFound
	}
	return r.store.Load()
}

func upsertAgentRegistration(settings *RuntimeState, request RegisterRequest, now time.Time) Registration {
	for index := range settings.Agents {
		if strings.TrimSpace(settings.Agents[index].AgentID) != strings.TrimSpace(request.AgentID) {
			continue
		}

		record := &settings.Agents[index]
		record.Name = firstNonEmpty(request.Name, record.Name)
		record.HostURL = firstNonEmpty(request.HostURL, record.HostURL)
		record.Status = "online"
		record.Capabilities = append([]string{}, request.Capabilities...)
		if record.RegisteredAt.IsZero() {
			record.RegisteredAt = now
		}
		record.LastHeartbeatAt = now
		return *record
	}

	record := Registration{
		AgentID:         request.AgentID,
		Name:            firstNonEmpty(request.Name, request.AgentID),
		HostURL:         request.HostURL,
		Status:          "online",
		Capabilities:    append([]string{}, request.Capabilities...),
		RegisteredAt:    now,
		LastHeartbeatAt: now,
	}
	settings.Agents = append(settings.Agents, record)
	return record
}

func updateAgentHeartbeat(settings *RuntimeState, agentID string, now time.Time) (Registration, bool) {
	for index := range settings.Agents {
		if strings.TrimSpace(settings.Agents[index].AgentID) != strings.TrimSpace(agentID) {
			continue
		}
		record := &settings.Agents[index]
		record.LastHeartbeatAt = now
		record.Status = "online"
		return *record, true
	}
	return Registration{}, false
}

func markAgentOffline(settings *RuntimeState, agentID string) bool {
	for index := range settings.Agents {
		if strings.TrimSpace(settings.Agents[index].AgentID) != strings.TrimSpace(agentID) {
			continue
		}
		settings.Agents[index].Status = "offline"
		return true
	}
	return false
}

func sortRegistrations(records []Registration) {
	sort.Slice(records, func(i, j int) bool {
		return records[i].Name < records[j].Name
	})
}
