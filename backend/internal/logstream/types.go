package logstream

type Line struct {
	Source    string `json:"source"`
	Timestamp string `json:"timestamp"`
	Level     string `json:"level"`
	Text      string `json:"text"`
}

type Record struct {
	ID          int    `json:"id"`
	ExecutionID string `json:"executionId"`
	Line        Line   `json:"line"`
}
