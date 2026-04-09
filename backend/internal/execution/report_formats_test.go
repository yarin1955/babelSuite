package execution

import "testing"

func TestParseJUnitSummarySupportsTestsuitesRoot(t *testing.T) {
	summary, err := parseJUnitSummary([]byte(`
<testsuites>
  <testsuite name="alpha" tests="3" failures="1" errors="0" skipped="1" time="1.5">
    <testcase name="a" time="0.2" />
    <testcase name="b" time="0.6"><failure message="boom">boom</failure></testcase>
    <testcase name="c" time="0.7"><skipped /></testcase>
  </testsuite>
</testsuites>`))
	if err != nil {
		t.Fatalf("parse junit: %v", err)
	}
	if summary.Total != 3 || summary.Passed != 1 || summary.Failures != 1 || summary.Errors != 0 || summary.Skipped != 1 {
		t.Fatalf("unexpected junit summary: %+v", summary)
	}
}

func TestParseCoberturaSummarySupportsCoverageRoot(t *testing.T) {
	summary, err := parseCoberturaSummary([]byte(`
<coverage line-rate="0.82" branch-rate="0.65" lines-covered="82" lines-valid="100" branches-covered="13" branches-valid="20"></coverage>`))
	if err != nil {
		t.Fatalf("parse cobertura: %v", err)
	}
	if summary.LineRate != 0.82 || summary.BranchRate != 0.65 {
		t.Fatalf("unexpected coverage rates: %+v", summary)
	}
	if summary.LinesCovered != 82 || summary.LinesValid != 100 || summary.BranchesCovered != 13 || summary.BranchesValid != 20 {
		t.Fatalf("unexpected coverage counts: %+v", summary)
	}
}
