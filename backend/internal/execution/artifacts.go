package execution

import (
	"encoding/xml"
	"path"
	"strings"
	"time"

	"github.com/babelsuite/babelsuite/internal/suites"
)

func (s *Service) registerStepArtifacts(executionID string, node topologyNode, status string, collected map[string][]byte) {
	artifacts := materializeExecutionArtifacts(node, status, collected)
	if len(artifacts) == 0 {
		return
	}

	s.mu.Lock()
	item := s.executions[executionID]
	if item == nil {
		s.mu.Unlock()
		return
	}

	added := false
	for _, artifact := range artifacts {
		if executionArtifactExists(item.record.Artifacts, artifact.ID) {
			continue
		}
		item.record.Artifacts = append(item.record.Artifacts, artifact)
		added = true
	}
	if added {
		item.record.UpdatedAt = time.Now().UTC()
	}
	s.mu.Unlock()

	if added {
		s.persistExecutionRuntime()
		s.syncObservers(executionID)
	}
}

func materializeExecutionArtifacts(node topologyNode, status string, collected map[string][]byte) []ExecutionArtifact {
	if len(node.ArtifactExports) == 0 {
		return nil
	}

	result := make([]ExecutionArtifact, 0, len(node.ArtifactExports))
	for _, export := range node.ArtifactExports {
		if !artifactExportMatchesStatus(export.On, status) {
			continue
		}
		result = append(result, buildExecutionArtifact(node, export, status, collected))
	}
	return result
}

func buildExecutionArtifact(node topologyNode, export suites.ArtifactExport, status string, collected map[string][]byte) ExecutionArtifact {
	format := normalizeArtifactFormat(export.Format)
	artifact := ExecutionArtifact{
		ID:       executionArtifactID(node.ID, export),
		StepID:   node.ID,
		StepName: node.Name,
		Path:     strings.TrimSpace(export.Path),
		Name:     executionArtifactName(export),
		On:       firstNonEmpty(strings.TrimSpace(export.On), "success"),
		Format:   format,
	}

	realContent := collected[strings.TrimSpace(export.Path)]

	switch format {
	case "junit":
		if len(realContent) > 0 {
			if summary, err := parseJUnitSummary(realContent); err == nil {
				artifact.TestSummary = summary
			}
			artifact.Content = string(realContent)
		} else {
			raw := syntheticJUnitReport(node, status)
			if summary, err := parseJUnitSummary(raw); err == nil {
				artifact.TestSummary = summary
			}
			artifact.Content = string(raw)
		}
	case "ctrf":
		if len(realContent) > 0 {
			if summary, err := parseCTRFSummary(realContent); err == nil {
				artifact.TestSummary = summary
			}
			artifact.Content = string(realContent)
		} else {
			artifact.TestSummary = &ExecutionTestSummary{}
		}
	case "cobertura":
		if len(realContent) > 0 {
			if summary, err := parseCoberturaSummary(realContent); err == nil {
				artifact.CoverageSummary = summary
			}
			artifact.Content = string(realContent)
		} else {
			artifact.CoverageSummary = &ExecutionCoverageSummary{}
		}
	default:
		if len(realContent) > 0 {
			artifact.Content = string(realContent)
		}
	}

	return artifact
}

func executionArtifactExists(items []ExecutionArtifact, targetID string) bool {
	for _, item := range items {
		if item.ID == targetID {
			return true
		}
	}
	return false
}

func artifactExportMatchesStatus(trigger, status string) bool {
	switch strings.TrimSpace(trigger) {
	case "", "success":
		return status == "healthy"
	case "failure":
		return status == "failed"
	case "always":
		return status == "healthy" || status == "failed"
	default:
		return false
	}
}

func executionArtifactName(export suites.ArtifactExport) string {
	if name := strings.TrimSpace(export.Name); name != "" {
		return name
	}
	base := strings.TrimSpace(path.Base(strings.TrimSpace(export.Path)))
	if base == "." || base == "/" || base == "" {
		return strings.TrimSpace(export.Path)
	}
	return base
}

func executionArtifactID(stepID string, export suites.ArtifactExport) string {
	replacer := strings.NewReplacer("/", "-", "\\", "-", " ", "-", "*", "wildcard", ":", "-")
	format := normalizeArtifactFormat(export.Format)
	if format == "" {
		format = "raw"
	}
	return replacer.Replace(stepID + "-" + executionArtifactName(export) + "-" + format)
}

type syntheticJUnitSuite struct {
	XMLName  xml.Name                 `xml:"testsuite"`
	Name     string                   `xml:"name,attr"`
	Tests    int                      `xml:"tests,attr"`
	Failures int                      `xml:"failures,attr"`
	Errors   int                      `xml:"errors,attr"`
	Skipped  int                      `xml:"skipped,attr"`
	Time     string                   `xml:"time,attr"`
	Cases    []syntheticJUnitTestCase `xml:"testcase"`
}

type syntheticJUnitTestCase struct {
	Classname string                 `xml:"classname,attr,omitempty"`
	Name      string                 `xml:"name,attr"`
	Time      string                 `xml:"time,attr,omitempty"`
	Failure   *syntheticJUnitFailure `xml:"failure,omitempty"`
}

type syntheticJUnitFailure struct {
	Message string `xml:"message,attr,omitempty"`
	Text    string `xml:",chardata"`
}

func syntheticJUnitReport(node topologyNode, status string) []byte {
	suite := syntheticJUnitSuite{
		Name:  node.Name,
		Tests: 1,
		Time:  "0",
		Cases: []syntheticJUnitTestCase{
			{
				Classname: node.Kind,
				Name:      node.Name,
				Time:      "0",
			},
		},
	}
	if status == "failed" {
		suite.Failures = 1
		suite.Cases[0].Failure = &syntheticJUnitFailure{
			Message: "step failed",
			Text:    node.Name + " ended in failed state.",
		}
	}
	report, err := xml.Marshal(suite)
	if err != nil {
		return nil
	}
	return report
}
