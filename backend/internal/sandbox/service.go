package sandbox

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/babelsuite/babelsuite/internal/eventstream"
)

var (
	ErrNotFound          = errors.New("sandbox not found")
	ErrDockerUnavailable = errors.New("docker unavailable")
)

const managedLabelFilter = "babelsuite.managed=true"
const inventoryStreamKey = "inventory"

type Manager interface {
	Snapshot(ctx context.Context) (*Inventory, error)
	ReapSandbox(ctx context.Context, sandboxID string) (*ReapResult, error)
	ReapAll(ctx context.Context) (*ReapResult, error)
	SubscribeEvents(ctx context.Context, since int) (<-chan StreamEvent, error)
}

type Inventory struct {
	DockerAvailable bool             `json:"dockerAvailable"`
	UpdatedAt       time.Time        `json:"updatedAt"`
	Summary         InventorySummary `json:"summary"`
	Sandboxes       []Sandbox        `json:"sandboxes"`
	Warnings        []string         `json:"warnings"`
}

type InventorySummary struct {
	ActiveSandboxes  int     `json:"activeSandboxes"`
	ZombieSandboxes  int     `json:"zombieSandboxes"`
	Containers       int     `json:"containers"`
	Networks         int     `json:"networks"`
	Volumes          int     `json:"volumes"`
	TotalCPUPercent  float64 `json:"totalCpuPercent"`
	TotalMemoryBytes int64   `json:"totalMemoryBytes"`
}

type Sandbox struct {
	SandboxID         string        `json:"sandboxId"`
	RunID             string        `json:"runId"`
	Suite             string        `json:"suite"`
	Owner             string        `json:"owner"`
	Profile           string        `json:"profile"`
	Status            string        `json:"status"`
	Summary           string        `json:"summary"`
	StartedAt         *time.Time    `json:"startedAt,omitempty"`
	LastHeartbeatAt   *time.Time    `json:"lastHeartbeatAt,omitempty"`
	OrchestratorPID   int           `json:"orchestratorPid,omitempty"`
	OrchestratorState string        `json:"orchestratorState"`
	IsZombie          bool          `json:"isZombie"`
	CanReap           bool          `json:"canReap"`
	ResourceUsage     ResourceUsage `json:"resourceUsage"`
	Containers        []Container   `json:"containers"`
	Networks          []Network     `json:"networks"`
	Volumes           []Volume      `json:"volumes"`
	Warnings          []string      `json:"warnings"`
}

type ResourceUsage struct {
	CPUPercent       float64 `json:"cpuPercent"`
	MemoryBytes      int64   `json:"memoryBytes"`
	MemoryLimitBytes int64   `json:"memoryLimitBytes"`
	MemoryPercent    float64 `json:"memoryPercent"`
}

type Container struct {
	ID        string        `json:"id"`
	Name      string        `json:"name"`
	Image     string        `json:"image"`
	State     string        `json:"state"`
	Status    string        `json:"status"`
	Ports     []string      `json:"ports"`
	StartedAt *time.Time    `json:"startedAt,omitempty"`
	ExitCode  int           `json:"exitCode,omitempty"`
	Usage     ResourceUsage `json:"usage"`
}

type Network struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Driver string `json:"driver"`
	Scope  string `json:"scope"`
}

type Volume struct {
	Name       string `json:"name"`
	Driver     string `json:"driver"`
	Mountpoint string `json:"mountpoint"`
}

type ReapResult struct {
	Scope             string   `json:"scope"`
	Target            string   `json:"target"`
	RemovedContainers int      `json:"removedContainers"`
	RemovedNetworks   int      `json:"removedNetworks"`
	RemovedVolumes    int      `json:"removedVolumes"`
	Warnings          []string `json:"warnings"`
}

type StreamEvent struct {
	ID       int       `json:"id"`
	Reason   string    `json:"reason"`
	Snapshot Inventory `json:"snapshot"`
}

type streamPayload struct {
	Reason   string
	Snapshot Inventory
}

type commandRunner interface {
	Run(ctx context.Context, name string, args ...string) ([]byte, error)
}

type processChecker interface {
	Alive(ctx context.Context, pid int) bool
}

type execRunner struct{}

func (execRunner) Run(ctx context.Context, name string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	return cmd.CombinedOutput()
}

type systemProcessChecker struct {
	runner commandRunner
}

func (c systemProcessChecker) Alive(ctx context.Context, pid int) bool {
	if pid <= 0 {
		return false
	}

	switch runtime.GOOS {
	case "windows":
		out, err := c.runner.Run(ctx, "tasklist", "/FI", fmt.Sprintf("PID eq %d", pid), "/FO", "CSV", "/NH")
		if err != nil {
			return false
		}
		text := strings.ToLower(strings.TrimSpace(string(out)))
		return text != "" && !strings.Contains(text, "no tasks are running")
	default:
		out, err := c.runner.Run(ctx, "ps", "-p", strconv.Itoa(pid), "-o", "pid=")
		if err != nil {
			return false
		}
		return strings.TrimSpace(string(out)) != ""
	}
}

type Service struct {
	runner  commandRunner
	checker processChecker
	now     func() time.Time
	ctx     context.Context
	cancel  context.CancelFunc
	events  *eventstream.Hub[streamPayload]
	stream  struct {
		mu          sync.Mutex
		lastSig     string
		hasSnapshot bool
	}
}

func NewService() *Service {
	runner := execRunner{}
	ctx, cancel := context.WithCancel(context.Background())
	service := &Service{
		runner:  runner,
		checker: systemProcessChecker{runner: runner},
		now:     func() time.Time { return time.Now().UTC() },
		ctx:     ctx,
		cancel:  cancel,
		events:  eventstream.NewHub[streamPayload](),
	}
	service.events.Open(inventoryStreamKey)
	go service.watchInventory()
	return service
}

func newServiceForTest(runner commandRunner, checker processChecker, now func() time.Time) *Service {
	service := &Service{
		runner:  runner,
		checker: checker,
		now:     now,
		events:  eventstream.NewHub[streamPayload](),
	}
	service.events.Open(inventoryStreamKey)
	return service
}

func (s *Service) Close() {
	if s.cancel != nil {
		s.cancel()
	}
}

type dockerContainer struct {
	ID        string
	Name      string
	Image     string
	Labels    map[string]string
	State     string
	StartedAt *time.Time
	ExitCode  int
	Ports     []string
}

type dockerNetwork struct {
	ID     string
	Name   string
	Driver string
	Scope  string
	Labels map[string]string
}

type dockerVolume struct {
	Name       string
	Driver     string
	Mountpoint string
	Labels     map[string]string
}

type dockerContainerInspect struct {
	ID     string `json:"Id"`
	Name   string `json:"Name"`
	Config struct {
		Image  string            `json:"Image"`
		Labels map[string]string `json:"Labels"`
	} `json:"Config"`
	State struct {
		Status    string `json:"Status"`
		StartedAt string `json:"StartedAt"`
		ExitCode  int    `json:"ExitCode"`
	} `json:"State"`
	NetworkSettings struct {
		Ports map[string][]dockerPortBinding `json:"Ports"`
	} `json:"NetworkSettings"`
}

type dockerPortBinding struct {
	HostIP   string `json:"HostIp"`
	HostPort string `json:"HostPort"`
}

type dockerNetworkInspect struct {
	ID     string            `json:"Id"`
	Name   string            `json:"Name"`
	Driver string            `json:"Driver"`
	Scope  string            `json:"Scope"`
	Labels map[string]string `json:"Labels"`
}

type dockerVolumeInspect struct {
	Name       string            `json:"Name"`
	Driver     string            `json:"Driver"`
	Mountpoint string            `json:"Mountpoint"`
	Labels     map[string]string `json:"Labels"`
}

func (s *Service) Snapshot(ctx context.Context) (*Inventory, error) {
	containers, networks, volumes, dockerAvailable, warnings, err := s.collectResources(ctx)
	if err != nil {
		return nil, err
	}

	if !dockerAvailable {
		return &Inventory{
			DockerAvailable: false,
			UpdatedAt:       s.now(),
			Summary:         InventorySummary{},
			Sandboxes:       []Sandbox{},
			Warnings:        warnings,
		}, nil
	}

	stats, statWarnings := s.collectStats(ctx, containers)
	warnings = append(warnings, statWarnings...)

	inventory := s.buildInventory(ctx, containers, networks, volumes, stats)
	inventory.DockerAvailable = true
	inventory.UpdatedAt = s.now()
	inventory.Warnings = compactStrings(warnings)
	return inventory, nil
}

func (s *Service) SubscribeEvents(ctx context.Context, since int) (<-chan StreamEvent, error) {
	if err := s.publishLatestSnapshot(ctx, "initial", false); err != nil {
		return nil, err
	}

	stream, err := s.events.Subscribe(ctx, inventoryStreamKey, since)
	if err != nil {
		return nil, err
	}

	result := make(chan StreamEvent, 32)
	go func() {
		defer close(result)
		for {
			select {
			case <-ctx.Done():
				return
			case record, ok := <-stream:
				if !ok {
					return
				}
				select {
				case result <- StreamEvent{
					ID:       record.ID,
					Reason:   record.Payload.Reason,
					Snapshot: record.Payload.Snapshot,
				}:
				case <-ctx.Done():
					return
				}
			}
		}
	}()

	return result, nil
}

func (s *Service) ReapSandbox(ctx context.Context, sandboxID string) (*ReapResult, error) {
	sandboxID = strings.TrimSpace(sandboxID)
	if sandboxID == "" {
		return nil, ErrNotFound
	}

	containers, networks, volumes, dockerAvailable, _, err := s.collectResources(ctx)
	if err != nil {
		return nil, err
	}
	if !dockerAvailable {
		return nil, ErrDockerUnavailable
	}

	inventory := s.buildInventory(ctx, containers, networks, volumes, nil)
	var target *Sandbox
	for index := range inventory.Sandboxes {
		if inventory.Sandboxes[index].SandboxID == sandboxID {
			target = &inventory.Sandboxes[index]
			break
		}
	}
	if target == nil {
		return nil, ErrNotFound
	}

	result, err := s.reapResources(ctx, "sandbox", sandboxID, target.Containers, target.Networks, target.Volumes)
	if err != nil {
		return nil, err
	}

	_ = s.publishLatestSnapshot(ctx, "sandbox-reap", false)
	return result, nil
}

func (s *Service) ReapAll(ctx context.Context) (*ReapResult, error) {
	containers, networks, volumes, dockerAvailable, _, err := s.collectResources(ctx)
	if err != nil {
		return nil, err
	}
	if !dockerAvailable {
		return nil, ErrDockerUnavailable
	}

	allContainers := make([]Container, 0, len(containers))
	for _, item := range containers {
		allContainers = append(allContainers, Container{ID: item.ID, Name: item.Name})
	}
	allNetworks := make([]Network, 0, len(networks))
	for _, item := range networks {
		allNetworks = append(allNetworks, Network{ID: item.ID, Name: item.Name})
	}
	allVolumes := make([]Volume, 0, len(volumes))
	for _, item := range volumes {
		allVolumes = append(allVolumes, Volume{Name: item.Name})
	}

	result, err := s.reapResources(ctx, "global", "all-managed-sandboxes", allContainers, allNetworks, allVolumes)
	if err != nil {
		return nil, err
	}

	_ = s.publishLatestSnapshot(ctx, "global-reap", false)
	return result, nil
}

func (s *Service) reapResources(ctx context.Context, scope, target string, containers []Container, networks []Network, volumes []Volume) (*ReapResult, error) {
	result := &ReapResult{
		Scope:             scope,
		Target:            target,
		RemovedContainers: len(containers),
		RemovedNetworks:   len(networks),
		RemovedVolumes:    len(volumes),
		Warnings:          []string{},
	}

	if len(containers) > 0 {
		ids := make([]string, 0, len(containers))
		for _, item := range containers {
			if item.ID != "" {
				ids = append(ids, item.ID)
			}
		}
		if len(ids) > 0 {
			if err := s.runDocker(ctx, append([]string{"rm", "-f"}, ids...)...); err != nil {
				result.Warnings = append(result.Warnings, fmt.Sprintf("Container cleanup reported an error: %s", humanizeDockerError(err)))
			}
		}
	}

	if len(networks) > 0 {
		ids := make([]string, 0, len(networks))
		for _, item := range networks {
			if item.ID != "" {
				ids = append(ids, item.ID)
			}
		}
		if len(ids) > 0 {
			if err := s.runDocker(ctx, append([]string{"network", "rm"}, ids...)...); err != nil {
				result.Warnings = append(result.Warnings, fmt.Sprintf("Network cleanup reported an error: %s", humanizeDockerError(err)))
			}
		}
	}

	if len(volumes) > 0 {
		names := make([]string, 0, len(volumes))
		for _, item := range volumes {
			if item.Name != "" {
				names = append(names, item.Name)
			}
		}
		if len(names) > 0 {
			if err := s.runDocker(ctx, append([]string{"volume", "rm", "-f"}, names...)...); err != nil {
				result.Warnings = append(result.Warnings, fmt.Sprintf("Volume cleanup reported an error: %s", humanizeDockerError(err)))
			}
		}
	}

	result.Warnings = compactStrings(result.Warnings)
	return result, nil
}

func (s *Service) collectResources(ctx context.Context) ([]dockerContainer, []dockerNetwork, []dockerVolume, bool, []string, error) {
	warnings := []string{}

	containers, err := s.listContainers(ctx)
	if err != nil {
		if errors.Is(err, ErrDockerUnavailable) {
			return nil, nil, nil, false, []string{humanizeDockerError(err)}, nil
		}
		warnings = append(warnings, fmt.Sprintf("Containers could not be inspected: %s", humanizeDockerError(err)))
	}

	networks, err := s.listNetworks(ctx)
	if err != nil {
		if errors.Is(err, ErrDockerUnavailable) {
			return nil, nil, nil, false, []string{humanizeDockerError(err)}, nil
		}
		warnings = append(warnings, fmt.Sprintf("Networks could not be inspected: %s", humanizeDockerError(err)))
	}

	volumes, err := s.listVolumes(ctx)
	if err != nil {
		if errors.Is(err, ErrDockerUnavailable) {
			return nil, nil, nil, false, []string{humanizeDockerError(err)}, nil
		}
		warnings = append(warnings, fmt.Sprintf("Volumes could not be inspected: %s", humanizeDockerError(err)))
	}

	return containers, networks, volumes, true, compactStrings(warnings), nil
}

func (s *Service) buildInventory(ctx context.Context, containers []dockerContainer, networks []dockerNetwork, volumes []dockerVolume, stats map[string]ResourceUsage) *Inventory {
	groups := map[string]*Sandbox{}

	for _, item := range containers {
		sandbox := ensureSandbox(groups, sandboxKey(item.Labels, "container", item.Name))
		hydrateSandbox(sandbox, item.Labels)
		container := Container{
			ID:        item.ID,
			Name:      item.Name,
			Image:     item.Image,
			State:     item.State,
			Status:    item.State,
			Ports:     append([]string{}, item.Ports...),
			StartedAt: item.StartedAt,
			ExitCode:  item.ExitCode,
			Usage:     lookupUsage(stats, item.ID, item.Name),
		}
		sandbox.Containers = append(sandbox.Containers, container)
		addUsage(&sandbox.ResourceUsage, container.Usage)
	}

	for _, item := range networks {
		sandbox := ensureSandbox(groups, sandboxKey(item.Labels, "network", item.Name))
		hydrateSandbox(sandbox, item.Labels)
		sandbox.Networks = append(sandbox.Networks, Network{
			ID:     item.ID,
			Name:   item.Name,
			Driver: item.Driver,
			Scope:  item.Scope,
		})
	}

	for _, item := range volumes {
		sandbox := ensureSandbox(groups, sandboxKey(item.Labels, "volume", item.Name))
		hydrateSandbox(sandbox, item.Labels)
		sandbox.Volumes = append(sandbox.Volumes, Volume{
			Name:       item.Name,
			Driver:     item.Driver,
			Mountpoint: item.Mountpoint,
		})
	}

	sandboxes := make([]Sandbox, 0, len(groups))
	for _, sandbox := range groups {
		finalizeSandbox(ctx, s.checker, s.now(), sandbox)
		sandboxes = append(sandboxes, *sandbox)
	}

	sort.Slice(sandboxes, func(i, j int) bool {
		leftRank := sandboxStatusRank(sandboxes[i].Status)
		rightRank := sandboxStatusRank(sandboxes[j].Status)
		if leftRank != rightRank {
			return leftRank < rightRank
		}
		leftTime := zeroTime(sandboxes[i].StartedAt)
		rightTime := zeroTime(sandboxes[j].StartedAt)
		if !leftTime.Equal(rightTime) {
			return leftTime.After(rightTime)
		}
		return sandboxes[i].SandboxID < sandboxes[j].SandboxID
	})

	inventory := &Inventory{
		Sandboxes: sandboxes,
	}
	for index := range sandboxes {
		item := sandboxes[index]
		inventory.Summary.ActiveSandboxes++
		if item.IsZombie {
			inventory.Summary.ZombieSandboxes++
		}
		inventory.Summary.Containers += len(item.Containers)
		inventory.Summary.Networks += len(item.Networks)
		inventory.Summary.Volumes += len(item.Volumes)
		inventory.Summary.TotalCPUPercent += item.ResourceUsage.CPUPercent
		inventory.Summary.TotalMemoryBytes += item.ResourceUsage.MemoryBytes
	}

	return inventory
}

func (s *Service) collectStats(ctx context.Context, containers []dockerContainer) (map[string]ResourceUsage, []string) {
	runningIDs := []string{}
	for _, item := range containers {
		if item.State == "running" {
			runningIDs = append(runningIDs, item.ID)
		}
	}
	if len(runningIDs) == 0 {
		return map[string]ResourceUsage{}, nil
	}

	output, err := s.runDockerOutput(ctx, append([]string{"stats", "--no-stream", "--format", "{{json .}}"}, runningIDs...)...)
	if err != nil {
		return map[string]ResourceUsage{}, []string{fmt.Sprintf("Live CPU and memory stats are unavailable: %s", humanizeDockerError(err))}
	}

	stats := map[string]ResourceUsage{}
	for _, line := range splitLines(output) {
		var row struct {
			ID       string `json:"ID"`
			Name     string `json:"Name"`
			CPUPerc  string `json:"CPUPerc"`
			MemUsage string `json:"MemUsage"`
			MemPerc  string `json:"MemPerc"`
		}
		if err := json.Unmarshal([]byte(line), &row); err != nil {
			continue
		}

		used, limit := parseMemoryUsage(row.MemUsage)
		usage := ResourceUsage{
			CPUPercent:       parsePercent(row.CPUPerc),
			MemoryBytes:      used,
			MemoryLimitBytes: limit,
			MemoryPercent:    parsePercent(row.MemPerc),
		}
		if row.ID != "" {
			stats[row.ID] = usage
		}
		if row.Name != "" {
			stats[row.Name] = usage
		}
	}

	return stats, nil
}

func (s *Service) listContainers(ctx context.Context) ([]dockerContainer, error) {
	idsOutput, err := s.runDockerOutput(ctx, "ps", "-aq", "--filter", "label="+managedLabelFilter)
	if err != nil {
		return nil, err
	}
	ids := splitLines(idsOutput)
	if len(ids) == 0 {
		return []dockerContainer{}, nil
	}

	output, err := s.runDockerOutput(ctx, append([]string{"inspect"}, ids...)...)
	if err != nil {
		return nil, err
	}

	var raw []dockerContainerInspect
	if err := json.Unmarshal(output, &raw); err != nil {
		return nil, err
	}

	containers := make([]dockerContainer, 0, len(raw))
	for _, item := range raw {
		container := dockerContainer{
			ID:       item.ID,
			Name:     strings.TrimPrefix(item.Name, "/"),
			Image:    item.Config.Image,
			Labels:   cloneLabels(item.Config.Labels),
			State:    strings.TrimSpace(item.State.Status),
			ExitCode: item.State.ExitCode,
			Ports:    formatPorts(item.NetworkSettings.Ports),
		}
		if startedAt := parseDockerTime(item.State.StartedAt); startedAt != nil {
			container.StartedAt = startedAt
		}
		containers = append(containers, container)
	}

	sort.Slice(containers, func(i, j int) bool { return containers[i].Name < containers[j].Name })
	return containers, nil
}

func (s *Service) listNetworks(ctx context.Context) ([]dockerNetwork, error) {
	idsOutput, err := s.runDockerOutput(ctx, "network", "ls", "-q", "--filter", "label="+managedLabelFilter)
	if err != nil {
		return nil, err
	}
	ids := splitLines(idsOutput)
	if len(ids) == 0 {
		return []dockerNetwork{}, nil
	}

	output, err := s.runDockerOutput(ctx, append([]string{"network", "inspect"}, ids...)...)
	if err != nil {
		return nil, err
	}

	var raw []dockerNetworkInspect
	if err := json.Unmarshal(output, &raw); err != nil {
		return nil, err
	}

	networks := make([]dockerNetwork, 0, len(raw))
	for _, item := range raw {
		networks = append(networks, dockerNetwork{
			ID:     item.ID,
			Name:   item.Name,
			Driver: item.Driver,
			Scope:  item.Scope,
			Labels: cloneLabels(item.Labels),
		})
	}

	sort.Slice(networks, func(i, j int) bool { return networks[i].Name < networks[j].Name })
	return networks, nil
}

func (s *Service) listVolumes(ctx context.Context) ([]dockerVolume, error) {
	namesOutput, err := s.runDockerOutput(ctx, "volume", "ls", "-q", "--filter", "label="+managedLabelFilter)
	if err != nil {
		return nil, err
	}
	names := splitLines(namesOutput)
	if len(names) == 0 {
		return []dockerVolume{}, nil
	}

	output, err := s.runDockerOutput(ctx, append([]string{"volume", "inspect"}, names...)...)
	if err != nil {
		return nil, err
	}

	var raw []dockerVolumeInspect
	if err := json.Unmarshal(output, &raw); err != nil {
		return nil, err
	}

	volumes := make([]dockerVolume, 0, len(raw))
	for _, item := range raw {
		volumes = append(volumes, dockerVolume{
			Name:       item.Name,
			Driver:     item.Driver,
			Mountpoint: item.Mountpoint,
			Labels:     cloneLabels(item.Labels),
		})
	}

	sort.Slice(volumes, func(i, j int) bool { return volumes[i].Name < volumes[j].Name })
	return volumes, nil
}

func (s *Service) runDocker(ctx context.Context, args ...string) error {
	_, err := s.runDockerOutput(ctx, args...)
	return err
}

func (s *Service) runDockerOutput(ctx context.Context, args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()

	out, err := s.runner.Run(ctx, "docker", args...)
	if err != nil {
		return nil, classifyDockerError(err, out)
	}
	return out, nil
}

func (s *Service) watchInventory() {
	if s.ctx == nil {
		return
	}

	_ = s.publishLatestSnapshot(s.ctx, "initial", false)

	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-s.ctx.Done():
			return
		case <-ticker.C:
			_ = s.publishLatestSnapshot(s.ctx, "poll", true)
		}
	}
}

func (s *Service) publishLatestSnapshot(ctx context.Context, reason string, onlyOnChange bool) error {
	inventory, err := s.Snapshot(ctx)
	if err != nil {
		return err
	}

	signature := snapshotSignature(inventory)

	s.stream.mu.Lock()
	if onlyOnChange && s.stream.hasSnapshot && s.stream.lastSig == signature {
		s.stream.mu.Unlock()
		return nil
	}
	s.stream.lastSig = signature
	s.stream.hasSnapshot = true
	s.stream.mu.Unlock()

	s.events.Append(inventoryStreamKey, streamPayload{
		Reason:   reason,
		Snapshot: *inventory,
	})
	return nil
}

func snapshotSignature(inventory *Inventory) string {
	if inventory == nil {
		return ""
	}

	clone := *inventory
	clone.UpdatedAt = time.Time{}
	data, err := json.Marshal(clone)
	if err != nil {
		return ""
	}
	return string(data)
}

func ensureSandbox(groups map[string]*Sandbox, key string) *Sandbox {
	existing, ok := groups[key]
	if ok {
		return existing
	}

	sandbox := &Sandbox{
		SandboxID:         key,
		OrchestratorState: "unknown",
		Containers:        []Container{},
		Networks:          []Network{},
		Volumes:           []Volume{},
		Warnings:          []string{},
	}
	groups[key] = sandbox
	return sandbox
}

func hydrateSandbox(sandbox *Sandbox, labels map[string]string) {
	if labels == nil {
		return
	}

	if sandbox.RunID == "" {
		sandbox.RunID = firstNonEmpty(labels["babelsuite.run_id"], labels["babelsuite.sandbox_id"], labels["babelsuite.execution_id"])
	}
	if sandbox.Suite == "" {
		sandbox.Suite = firstNonEmpty(labels["babelsuite.suite"], labels["babelsuite.suite_name"], labels["babelsuite.suite_id"])
	}
	if sandbox.Owner == "" {
		sandbox.Owner = firstNonEmpty(labels["babelsuite.owner"], labels["babelsuite.user"], labels["babelsuite.initiator"])
	}
	if sandbox.Profile == "" {
		sandbox.Profile = labels["babelsuite.profile"]
	}
	if sandbox.OrchestratorPID == 0 {
		sandbox.OrchestratorPID = parseInt(labels["babelsuite.orchestrator_pid"])
	}
	if sandbox.StartedAt == nil {
		sandbox.StartedAt = parseDockerTime(firstNonEmpty(labels["babelsuite.started_at"], labels["babelsuite.run_started_at"]))
	}
	if sandbox.LastHeartbeatAt == nil {
		sandbox.LastHeartbeatAt = parseDockerTime(firstNonEmpty(labels["babelsuite.last_heartbeat"], labels["babelsuite.heartbeat_at"]))
	}
	if labelIsTrue(labels["babelsuite.zombie"]) {
		sandbox.IsZombie = true
	}
}

func finalizeSandbox(ctx context.Context, checker processChecker, now time.Time, sandbox *Sandbox) {
	sort.Slice(sandbox.Containers, func(i, j int) bool { return sandbox.Containers[i].Name < sandbox.Containers[j].Name })
	sort.Slice(sandbox.Networks, func(i, j int) bool { return sandbox.Networks[i].Name < sandbox.Networks[j].Name })
	sort.Slice(sandbox.Volumes, func(i, j int) bool { return sandbox.Volumes[i].Name < sandbox.Volumes[j].Name })

	for index := range sandbox.Containers {
		if sandbox.StartedAt == nil || (sandbox.Containers[index].StartedAt != nil && sandbox.Containers[index].StartedAt.Before(*sandbox.StartedAt)) {
			sandbox.StartedAt = sandbox.Containers[index].StartedAt
		}
	}

	running := 0
	failed := 0
	for _, item := range sandbox.Containers {
		if item.State == "running" {
			running++
		}
		if item.State == "exited" || item.State == "dead" || item.State == "restarting" {
			failed++
		}
	}

	switch {
	case sandbox.OrchestratorPID > 0 && checker.Alive(ctx, sandbox.OrchestratorPID):
		sandbox.OrchestratorState = "alive"
	case sandbox.OrchestratorPID > 0:
		sandbox.OrchestratorState = "dead"
	case sandbox.LastHeartbeatAt != nil && now.Sub(*sandbox.LastHeartbeatAt) > 2*time.Minute:
		sandbox.OrchestratorState = "stale"
	default:
		sandbox.OrchestratorState = "unknown"
	}

	if !sandbox.IsZombie && running > 0 && (sandbox.OrchestratorState == "dead" || sandbox.OrchestratorState == "stale") {
		sandbox.IsZombie = true
	}

	switch {
	case sandbox.IsZombie:
		sandbox.Status = "Zombie"
	case failed > 0:
		sandbox.Status = "Degraded"
	case running > 0:
		sandbox.Status = "Running"
	case len(sandbox.Containers)+len(sandbox.Networks)+len(sandbox.Volumes) > 0:
		sandbox.Status = "Residual"
	default:
		sandbox.Status = "Unknown"
	}

	if sandbox.Suite == "" {
		sandbox.Suite = "Unattributed sandbox"
	}
	if sandbox.Owner == "" {
		sandbox.Owner = "Unknown owner"
	}
	if sandbox.Profile == "" {
		sandbox.Profile = "No profile label"
	}
	if sandbox.RunID == "" {
		sandbox.Warnings = append(sandbox.Warnings, "Resources are missing babelsuite.run_id, so cleanup is grouped by fallback labels.")
	}
	if sandbox.IsZombie {
		sandbox.Warnings = append(sandbox.Warnings, "The orchestrator appears to be gone while managed containers are still alive.")
	}

	sandbox.CanReap = len(sandbox.Containers)+len(sandbox.Networks)+len(sandbox.Volumes) > 0
	sandbox.Warnings = compactStrings(sandbox.Warnings)
	sandbox.ResourceUsage.CPUPercent = roundFloat(sandbox.ResourceUsage.CPUPercent)
	sandbox.ResourceUsage.MemoryPercent = roundFloat(sandbox.ResourceUsage.MemoryPercent)
	sandbox.Summary = fmt.Sprintf("%d containers, %d networks, %d volumes", len(sandbox.Containers), len(sandbox.Networks), len(sandbox.Volumes))
}

func lookupUsage(stats map[string]ResourceUsage, id, name string) ResourceUsage {
	if usage, ok := stats[id]; ok {
		return usage
	}
	if usage, ok := stats[name]; ok {
		return usage
	}
	for key, usage := range stats {
		if strings.HasPrefix(id, key) || strings.HasPrefix(key, id) {
			return usage
		}
	}
	return ResourceUsage{}
}

func addUsage(total *ResourceUsage, next ResourceUsage) {
	total.CPUPercent += next.CPUPercent
	total.MemoryBytes += next.MemoryBytes
	total.MemoryLimitBytes += next.MemoryLimitBytes
	if total.MemoryLimitBytes > 0 {
		total.MemoryPercent = roundFloat(float64(total.MemoryBytes) / float64(total.MemoryLimitBytes) * 100)
	}
}

func sandboxKey(labels map[string]string, kind, fallback string) string {
	runID := firstNonEmpty(labels["babelsuite.run_id"], labels["babelsuite.sandbox_id"], labels["babelsuite.execution_id"])
	if runID != "" {
		return runID
	}
	return "orphan-" + kind + "-" + slugify(fallback)
}

func sandboxStatusRank(status string) int {
	switch status {
	case "Zombie":
		return 0
	case "Degraded":
		return 1
	case "Running":
		return 2
	case "Residual":
		return 3
	default:
		return 4
	}
}

func classifyDockerError(err error, output []byte) error {
	if err == nil {
		return nil
	}

	if errors.Is(err, exec.ErrNotFound) {
		return fmt.Errorf("%w: docker CLI was not found in PATH", ErrDockerUnavailable)
	}

	text := strings.ToLower(string(output) + " " + err.Error())
	if strings.Contains(text, "cannot connect to the docker daemon") ||
		strings.Contains(text, "error during connect") ||
		strings.Contains(text, "daemon is not running") ||
		strings.Contains(text, "docker desktop") {
		return fmt.Errorf("%w: %s", ErrDockerUnavailable, strings.TrimSpace(string(output)))
	}

	return fmt.Errorf("%w: %s", err, strings.TrimSpace(string(output)))
}

func humanizeDockerError(err error) string {
	if err == nil {
		return ""
	}
	return strings.TrimSpace(err.Error())
}

func formatPorts(ports map[string][]dockerPortBinding) []string {
	if len(ports) == 0 {
		return []string{}
	}

	result := []string{}
	for containerPort, bindings := range ports {
		if len(bindings) == 0 {
			result = append(result, containerPort)
			continue
		}
		for _, binding := range bindings {
			if binding.HostPort == "" {
				result = append(result, containerPort)
				continue
			}
			result = append(result, fmt.Sprintf("%s -> %s", binding.HostPort, containerPort))
		}
	}

	sort.Strings(result)
	return result
}

func parseDockerTime(value string) *time.Time {
	value = strings.TrimSpace(value)
	if value == "" || value == "0001-01-01T00:00:00Z" {
		return nil
	}

	for _, layout := range []string{time.RFC3339Nano, time.RFC3339} {
		if parsed, err := time.Parse(layout, value); err == nil {
			utc := parsed.UTC()
			return &utc
		}
	}
	return nil
}

func parseInt(value string) int {
	number, _ := strconv.Atoi(strings.TrimSpace(value))
	return number
}

func parsePercent(value string) float64 {
	value = strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(value), "%"))
	number, err := strconv.ParseFloat(value, 64)
	if err != nil {
		return 0
	}
	return roundFloat(number)
}

var sizePattern = regexp.MustCompile(`(?i)^\s*([0-9]+(?:\.[0-9]+)?)\s*([kmgtp]?i?b)\s*$`)

func parseMemoryUsage(value string) (int64, int64) {
	parts := strings.Split(value, "/")
	if len(parts) != 2 {
		return 0, 0
	}
	return parseHumanSize(parts[0]), parseHumanSize(parts[1])
}

func parseHumanSize(value string) int64 {
	match := sizePattern.FindStringSubmatch(strings.TrimSpace(value))
	if len(match) != 3 {
		return 0
	}

	number, err := strconv.ParseFloat(match[1], 64)
	if err != nil {
		return 0
	}

	unit := strings.ToLower(match[2])
	multiplier := float64(1)
	switch unit {
	case "kb":
		multiplier = 1000
	case "mb":
		multiplier = 1000 * 1000
	case "gb":
		multiplier = 1000 * 1000 * 1000
	case "tb":
		multiplier = 1000 * 1000 * 1000 * 1000
	case "kib":
		multiplier = 1024
	case "mib":
		multiplier = 1024 * 1024
	case "gib":
		multiplier = 1024 * 1024 * 1024
	case "tib":
		multiplier = 1024 * 1024 * 1024 * 1024
	}

	return int64(number * multiplier)
}

func splitLines(value []byte) []string {
	lines := strings.Split(strings.ReplaceAll(string(value), "\r\n", "\n"), "\n")
	result := make([]string, 0, len(lines))
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed != "" {
			result = append(result, trimmed)
		}
	}
	return result
}

func compactStrings(values []string) []string {
	seen := map[string]struct{}{}
	result := make([]string, 0, len(values))
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			continue
		}
		if _, ok := seen[trimmed]; ok {
			continue
		}
		seen[trimmed] = struct{}{}
		result = append(result, trimmed)
	}
	return result
}

func roundFloat(value float64) float64 {
	parsed, _ := strconv.ParseFloat(fmt.Sprintf("%.2f", value), 64)
	return parsed
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func labelIsTrue(value string) bool {
	normalized := strings.ToLower(strings.TrimSpace(value))
	return normalized == "true" || normalized == "1" || normalized == "yes"
}

func slugify(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "" {
		return "unknown"
	}
	var builder strings.Builder
	lastDash := false
	for _, char := range value {
		if (char >= 'a' && char <= 'z') || (char >= '0' && char <= '9') {
			builder.WriteRune(char)
			lastDash = false
			continue
		}
		if !lastDash {
			builder.WriteByte('-')
			lastDash = true
		}
	}
	result := strings.Trim(builder.String(), "-")
	if result == "" {
		return "unknown"
	}
	return result
}

func zeroTime(value *time.Time) time.Time {
	if value == nil {
		return time.Time{}
	}
	return *value
}

func cloneLabels(source map[string]string) map[string]string {
	if len(source) == 0 {
		return map[string]string{}
	}
	result := make(map[string]string, len(source))
	for key, value := range source {
		result[key] = value
	}
	return result
}
