package mocking

import (
	"strconv"
	"strings"

	"github.com/babelsuite/babelsuite/internal/suites"
)

func (s *Service) loadState(key string, config *suites.MockState) map[string]string {
	state := cloneStringMap(nil)
	if config != nil {
		state = cloneStringMap(config.Defaults)
	}
	if strings.TrimSpace(key) == "" {
		return state
	}

	s.mu.RLock()
	current, ok := s.state[key]
	s.mu.RUnlock()
	if !ok {
		return state
	}

	merged := cloneStringMap(state)
	if merged == nil {
		merged = make(map[string]string, len(current))
	}
	for field, value := range current {
		merged[field] = value
	}
	return merged
}

func (s *Service) applyStateTransition(config *suites.MockState, lookupKey, exampleName string, suite suites.Definition, surface suites.APISurface, operation suites.APIOperation, snapshot requestSnapshot, state map[string]string, result *Result) {
	if config == nil {
		return
	}

	responseBodyJSON := parseJSONMap(string(result.Body))
	contextMap := buildTemplateContext(suite, surface, operation, snapshot, state, responseBodyJSON, exampleName)

	for _, transition := range config.Transitions {
		if strings.TrimSpace(transition.OnExample) != "" && transition.OnExample != exampleName {
			continue
		}

		key := renderTemplate(firstNonEmpty(transition.MutationKeyTemplate, config.MutationKeyTemplate, lookupKey), contextMap)
		if strings.TrimSpace(key) == "" {
			continue
		}

		nextState := cloneStringMap(state)
		if nextState == nil {
			nextState = make(map[string]string)
		}
		for field, value := range transition.Set {
			nextState[field] = renderTemplate(value, contextMap)
		}
		for _, field := range transition.Delete {
			delete(nextState, field)
		}
		for field, delta := range transition.Increment {
			current, _ := strconv.Atoi(nextState[field])
			nextState[field] = strconv.Itoa(current + delta)
		}
		s.storeState(key, nextState)
		result.Headers.Set("X-Babelsuite-State-Key", key)
		return
	}
}

func (s *Service) storeState(key string, value map[string]string) {
	if strings.TrimSpace(key) == "" {
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	s.state[key] = cloneStringMap(value)
}

func cloneStringMap(input map[string]string) map[string]string {
	if len(input) == 0 {
		return nil
	}
	output := make(map[string]string, len(input))
	for key, value := range input {
		output[key] = value
	}
	return output
}
