package execution

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"strconv"
	"strings"
)

type junitSuiteDocument struct {
	XMLName  xml.Name             `xml:"testsuite"`
	Name     string               `xml:"name,attr"`
	Tests    string               `xml:"tests,attr"`
	Failures string               `xml:"failures,attr"`
	Errors   string               `xml:"errors,attr"`
	Skipped  string               `xml:"skipped,attr"`
	Time     string               `xml:"time,attr"`
	Suites   []junitSuiteDocument `xml:"testsuite"`
	Cases    []junitCaseDocument  `xml:"testcase"`
}

type junitSuitesDocument struct {
	XMLName xml.Name             `xml:"testsuites"`
	Suites  []junitSuiteDocument `xml:"testsuite"`
}

type junitCaseDocument struct {
	Time    string                `xml:"time,attr"`
	Failure *junitFailureDocument `xml:"failure"`
	Error   *junitFailureDocument `xml:"error"`
	Skipped *junitSkippedDocument `xml:"skipped"`
}

type junitFailureDocument struct{}

type junitSkippedDocument struct{}

type coberturaDocument struct {
	XMLName         xml.Name `xml:"coverage"`
	LineRate        string   `xml:"line-rate,attr"`
	BranchRate      string   `xml:"branch-rate,attr"`
	LinesCovered    string   `xml:"lines-covered,attr"`
	LinesValid      string   `xml:"lines-valid,attr"`
	BranchesCovered string   `xml:"branches-covered,attr"`
	BranchesValid   string   `xml:"branches-valid,attr"`
}

func normalizeArtifactFormat(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

func parseJUnitSummary(document []byte) (*ExecutionTestSummary, error) {
	trimmed := bytes.TrimSpace(document)
	if len(trimmed) == 0 {
		return nil, fmt.Errorf("empty junit document")
	}

	var probe struct {
		XMLName xml.Name
	}
	if err := xml.Unmarshal(trimmed, &probe); err != nil {
		return nil, err
	}

	switch probe.XMLName.Local {
	case "testsuite":
		var suite junitSuiteDocument
		if err := xml.Unmarshal(trimmed, &suite); err != nil {
			return nil, err
		}
		summary := summarizeJUnitSuite(suite)
		return &summary, nil
	case "testsuites":
		var suites junitSuitesDocument
		if err := xml.Unmarshal(trimmed, &suites); err != nil {
			return nil, err
		}
		summary := ExecutionTestSummary{}
		for _, suite := range suites.Suites {
			child := summarizeJUnitSuite(suite)
			summary.Total += child.Total
			summary.Passed += child.Passed
			summary.Failures += child.Failures
			summary.Errors += child.Errors
			summary.Skipped += child.Skipped
			summary.DurationSeconds += child.DurationSeconds
		}
		return &summary, nil
	default:
		return nil, fmt.Errorf("unsupported junit root element %q", probe.XMLName.Local)
	}
}

func summarizeJUnitSuite(suite junitSuiteDocument) ExecutionTestSummary {
	summary := ExecutionTestSummary{
		Total:           parseXMLInt(suite.Tests),
		Failures:        parseXMLInt(suite.Failures),
		Errors:          parseXMLInt(suite.Errors),
		Skipped:         parseXMLInt(suite.Skipped),
		DurationSeconds: parseXMLFloat(suite.Time),
	}

	if len(suite.Suites) > 0 {
		summary = ExecutionTestSummary{}
		for _, childSuite := range suite.Suites {
			child := summarizeJUnitSuite(childSuite)
			summary.Total += child.Total
			summary.Passed += child.Passed
			summary.Failures += child.Failures
			summary.Errors += child.Errors
			summary.Skipped += child.Skipped
			summary.DurationSeconds += child.DurationSeconds
		}
		return summary
	}

	if len(suite.Cases) > 0 && summary.Total == 0 {
		summary.Total = len(suite.Cases)
	}
	if len(suite.Cases) > 0 && summary.Failures == 0 && summary.Errors == 0 && summary.Skipped == 0 {
		for _, testCase := range suite.Cases {
			switch {
			case testCase.Failure != nil:
				summary.Failures++
			case testCase.Error != nil:
				summary.Errors++
			case testCase.Skipped != nil:
				summary.Skipped++
			}
			summary.DurationSeconds += parseXMLFloat(testCase.Time)
		}
	}

	summary.Passed = summary.Total - summary.Failures - summary.Errors - summary.Skipped
	if summary.Passed < 0 {
		summary.Passed = 0
	}
	return summary
}

func parseCoberturaSummary(document []byte) (*ExecutionCoverageSummary, error) {
	trimmed := bytes.TrimSpace(document)
	if len(trimmed) == 0 {
		return nil, fmt.Errorf("empty cobertura document")
	}

	var coverage coberturaDocument
	if err := xml.Unmarshal(trimmed, &coverage); err != nil {
		return nil, err
	}
	if coverage.XMLName.Local != "coverage" {
		return nil, fmt.Errorf("unsupported cobertura root element %q", coverage.XMLName.Local)
	}

	return &ExecutionCoverageSummary{
		LineRate:        parseXMLFloat(coverage.LineRate),
		BranchRate:      parseXMLFloat(coverage.BranchRate),
		LinesCovered:    parseXMLInt(coverage.LinesCovered),
		LinesValid:      parseXMLInt(coverage.LinesValid),
		BranchesCovered: parseXMLInt(coverage.BranchesCovered),
		BranchesValid:   parseXMLInt(coverage.BranchesValid),
	}, nil
}

func parseXMLInt(value string) int {
	parsed, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil {
		return 0
	}
	return parsed
}

func parseXMLFloat(value string) float64 {
	parsed, err := strconv.ParseFloat(strings.TrimSpace(value), 64)
	if err != nil {
		return 0
	}
	return parsed
}
