package logstream

type Line struct {
	Source    string `json:"source"`
	Timestamp string `json:"timestamp"`
	Level     string `json:"level"`
	Kind      string `json:"kind"` // "system" = orchestrator event, "output" = raw container stdout/stderr, "metric" = live traffic snapshot
	Text      string `json:"text"`
}

type Record struct {
	ID          int    `json:"id"`
	ExecutionID string `json:"executionId"`
	Line        Line   `json:"line"`
}
