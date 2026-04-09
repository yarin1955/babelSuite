package execution

import (
	"time"

	"github.com/babelsuite/babelsuite/internal/demofs"
	"github.com/babelsuite/babelsuite/internal/suites"
)

type demoExecutionDocument struct {
	Metadata map[string]suiteRuntimeMeta `json:"metadata"`
	History  []demoExecutionSeed         `json:"history"`
}

type demoExecutionSeed struct {
	ID         string        `json:"id"`
	SuiteID    string        `json:"suiteId"`
	Profile    string        `json:"profile"`
	Trigger    string        `json:"trigger"`
	Status     string        `json:"status"`
	StartedAgo time.Duration `json:"startedAgo"`
}

func (s *Service) seedHistory() {
	if !demofs.Enabled() {
		return
	}

	document, err := loadExecutionDemoDocument()
	if err != nil {
		return
	}

	for _, item := range document.History {
		s.seedHistoricalExecution(item.ID, item.SuiteID, item.Profile, item.Trigger, item.Status, item.StartedAgo)
	}
}

func (s *Service) seedHistoricalExecution(executionID, suiteID, profile, trigger, status string, startedAgo time.Duration) {
	suite, err := s.suiteSource.Get(suiteID)
	if err != nil {
		return
	}
	topology, err := suites.ResolveTopology(*suite, s.suiteSource.List())
	if err != nil {
		return
	}
	suite.Topology = topology
	meta := s.suiteMeta[suiteID]

	startedAt := time.Now().UTC().Add(-startedAgo)
	events := buildHistoricalEvents(suite, topology, status, profile, meta)
	stepStatus, completed := buildStepStatus(topology, events)

	state := &executionState{
		record: ExecutionRecord{
			ID:        executionID,
			Suite:     buildExecutionSuite(*suite),
			Profile:   profile,
			BackendID: "local-docker",
			Backend:   "Local Docker",
			Trigger:   trigger,
			Status:    status,
			StartedAt: startedAt,
			UpdatedAt: startedAt.Add(time.Duration(len(events)) * 2 * time.Second),
			Author:    meta.Author,
			Commit:    buildCommitHash(suite.ID, executionID),
			Branch:    meta.Branch,
			Message:   meta.Message,
			Events:    events,
		},
		total:      len(topology),
		completed:  completed,
		stepStatus: stepStatus,
	}

	s.executions[executionID] = state
	s.order = append(s.order, executionID)
	s.logs.Open(executionID)
	for _, event := range events {
		s.appendLog(executionID, event)
	}
	go s.syncObservers(executionID)
}

func seedExecutionMetadata() map[string]suiteRuntimeMeta {
	if !demofs.Enabled() {
		return map[string]suiteRuntimeMeta{}
	}

	document, err := loadExecutionDemoDocument()
	if err != nil || document.Metadata == nil {
		return map[string]suiteRuntimeMeta{}
	}

	return document.Metadata
}

func loadExecutionDemoDocument() (demoExecutionDocument, error) {
	var document demoExecutionDocument

	manifest, err := demofs.LoadManifest()
	if err != nil {
		return document, err
	}

	return demofs.LoadJSON[demoExecutionDocument](manifest.ExecutionsFile)
}
