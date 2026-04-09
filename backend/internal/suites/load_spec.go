package suites

import "time"

type LoadSpec struct {
	Variant      string          `json:"variant,omitempty"`
	PlanPath     string          `json:"planPath,omitempty"`
	Target       string          `json:"target,omitempty"`
	RequestsPerS float64         `json:"requestsPerSecond,omitempty"`
	ArrivalRate  float64         `json:"arrivalRate,omitempty"`
	Users        []LoadUser      `json:"users,omitempty"`
	Stages       []LoadStage     `json:"stages,omitempty"`
	Thresholds   []LoadThreshold `json:"thresholds,omitempty"`
}

type LoadUser struct {
	ID     string     `json:"id,omitempty"`
	Name   string     `json:"name,omitempty"`
	Weight int        `json:"weight,omitempty"`
	Wait   LoadWait   `json:"wait,omitempty"`
	Tasks  []LoadTask `json:"tasks,omitempty"`
}

type LoadWait struct {
	Mode       string  `json:"mode,omitempty"`
	Seconds    float64 `json:"seconds,omitempty"`
	MinSeconds float64 `json:"minSeconds,omitempty"`
	MaxSeconds float64 `json:"maxSeconds,omitempty"`
}

type LoadTask struct {
	ID      string          `json:"id,omitempty"`
	Name    string          `json:"name,omitempty"`
	Weight  int             `json:"weight,omitempty"`
	Request LoadRequest     `json:"request"`
	Checks  []LoadThreshold `json:"checks,omitempty"`
}

type LoadRequest struct {
	Method  string            `json:"method,omitempty"`
	Path    string            `json:"path,omitempty"`
	Name    string            `json:"name,omitempty"`
	Headers map[string]string `json:"headers,omitempty"`
	Body    string            `json:"body,omitempty"`
	Checks  []LoadThreshold   `json:"checks,omitempty"`
}

type LoadStage struct {
	Duration  time.Duration `json:"duration,omitempty"`
	Users     int           `json:"users,omitempty"`
	SpawnRate float64       `json:"spawnRate,omitempty"`
	Stop      bool          `json:"stop,omitempty"`
}

type LoadThreshold struct {
	Metric  string  `json:"metric,omitempty"`
	Op      string  `json:"op,omitempty"`
	Value   float64 `json:"value,omitempty"`
	Sampler string  `json:"sampler,omitempty"`
}

func cloneLoadSpec(input *LoadSpec) *LoadSpec {
	if input == nil {
		return nil
	}

	output := *input
	output.Users = make([]LoadUser, len(input.Users))
	for index, user := range input.Users {
		output.Users[index] = user
		output.Users[index].Tasks = make([]LoadTask, len(user.Tasks))
		for taskIndex, task := range user.Tasks {
			output.Users[index].Tasks[taskIndex] = task
			output.Users[index].Tasks[taskIndex].Checks = append([]LoadThreshold{}, task.Checks...)
			output.Users[index].Tasks[taskIndex].Request.Headers = cloneStringMap(task.Request.Headers)
			output.Users[index].Tasks[taskIndex].Request.Checks = append([]LoadThreshold{}, task.Request.Checks...)
		}
	}
	output.Stages = append([]LoadStage{}, input.Stages...)
	output.Thresholds = append([]LoadThreshold{}, input.Thresholds...)
	return &output
}
