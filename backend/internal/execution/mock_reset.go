package execution

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/babelsuite/babelsuite/internal/suites"
)

func (s *Service) resetMockState(ctx context.Context, executionID string, suite *suites.Definition, node topologyNode) error {
	if len(node.ResetMocks) == 0 {
		return nil
	}

	s.mu.Lock()
	resetter := s.mockResetter
	s.mu.Unlock()
	if resetter == nil {
		return nil
	}

	suiteIDs, labels, err := resetMockSuiteTargets(suite, node.ResetMocks)
	if err != nil {
		return err
	}
	if len(suiteIDs) == 0 {
		return nil
	}

	s.appendEvent(executionID, ExecutionEvent{
		ID:        node.ID + "-reset-mocks",
		Source:    node.ID,
		Timestamp: s.nextTimestamp(executionID),
		Text:      fmt.Sprintf("[%s] Resetting mock state for %s before the test starts.", node.Name, strings.Join(labels, ", ")),
		Status:    "running",
		Level:     "info",
	})

	for _, suiteID := range suiteIDs {
		if err := resetter.ResetSuiteState(ctx, suiteID); err != nil {
			return err
		}
	}
	return nil
}

func resetMockSuiteTargets(suite *suites.Definition, targets []string) ([]string, []string, error) {
	if suite == nil || len(targets) == 0 {
		return nil, nil, nil
	}

	seen := make(map[string]struct{}, len(targets))
	suiteIDs := make([]string, 0, len(targets))
	labels := make([]string, 0, len(targets))
	for _, target := range targets {
		mockNode := topologyNodeByID(suite, target)
		if mockNode == nil {
			return nil, nil, fmt.Errorf("reset_mocks target %q was not found", target)
		}
		if mockNode.Kind != suites.NodeKindMock {
			return nil, nil, fmt.Errorf("reset_mocks target %q is not a mock node", target)
		}

		targetSuiteID := firstNonEmpty(mockNode.SourceSuiteID, suite.ID)
		if _, ok := seen[targetSuiteID]; ok {
			continue
		}
		seen[targetSuiteID] = struct{}{}
		suiteIDs = append(suiteIDs, targetSuiteID)
		labels = append(labels, mockNode.Name)
	}

	sort.Strings(suiteIDs)
	sort.Strings(labels)
	return suiteIDs, labels, nil
}
