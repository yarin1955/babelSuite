package agent

import (
	"context"
	"time"

	"github.com/babelsuite/babelsuite/internal/logstream"
)

type StepNode struct {
	ID        string   `json:"id" yaml:"id"`
	Name      string   `json:"name" yaml:"name"`
	Kind      string   `json:"kind" yaml:"kind"`
	DependsOn []string `json:"dependsOn" yaml:"dependsOn"`
}

type StepRequest struct {
	JobID           string        `json:"jobId" yaml:"jobId"`
	ExecutionID     string        `json:"executionId" yaml:"executionId"`
	SuiteID         string        `json:"suiteId" yaml:"suiteId"`
	SuiteTitle      string        `json:"suiteTitle" yaml:"suiteTitle"`
	SuiteRepository string        `json:"suiteRepository,omitempty" yaml:"suiteRepository,omitempty"`
	Profile         string        `json:"profile" yaml:"profile"`
	Trigger         string        `json:"trigger,omitempty" yaml:"trigger,omitempty"`
	BackendID       string        `json:"backendId,omitempty" yaml:"backendId,omitempty"`
	BackendKind     string        `json:"backendKind,omitempty" yaml:"backendKind,omitempty"`
	BackendLabel    string        `json:"backendLabel,omitempty" yaml:"backendLabel,omitempty"`
	StepIndex       int           `json:"stepIndex,omitempty" yaml:"stepIndex,omitempty"`
	TotalSteps      int           `json:"totalSteps,omitempty" yaml:"totalSteps,omitempty"`
	LeaseTTL        time.Duration `json:"leaseTtl,omitempty" yaml:"leaseTtl,omitempty"`
	Node            StepNode      `json:"node" yaml:"node"`
}

type StreamMessage struct {
	Type  string          `json:"type"`
	JobID string          `json:"jobId,omitempty"`
	Line  *logstream.Line `json:"line,omitempty"`
	Error string          `json:"error,omitempty"`
}

type Info struct {
	AgentID         string    `json:"agentId"`
	Name            string    `json:"name"`
	HostURL         string    `json:"hostUrl,omitempty"`
	Status          string    `json:"status"`
	Capabilities    []string  `json:"capabilities,omitempty"`
	LastHeartbeatAt time.Time `json:"lastHeartbeatAt,omitempty"`
}

type RegisterRequest struct {
	AgentID      string   `json:"agentId"`
	Name         string   `json:"name"`
	HostURL      string   `json:"hostUrl"`
	Capabilities []string `json:"capabilities,omitempty"`
}

type Registration struct {
	AgentID         string    `json:"agentId" yaml:"agentId"`
	Name            string    `json:"name" yaml:"name"`
	HostURL         string    `json:"hostUrl" yaml:"hostUrl"`
	Status          string    `json:"status" yaml:"status"`
	Capabilities    []string  `json:"capabilities,omitempty" yaml:"capabilities,omitempty"`
	RegisteredAt    time.Time `json:"registeredAt" yaml:"registeredAt"`
	LastHeartbeatAt time.Time `json:"lastHeartbeatAt" yaml:"lastHeartbeatAt"`
}

type AssignmentStatus string

const (
	AssignmentPending   AssignmentStatus = "pending"
	AssignmentRunning   AssignmentStatus = "running"
	AssignmentSucceeded AssignmentStatus = "succeeded"
	AssignmentFailed    AssignmentStatus = "failed"
	AssignmentCanceled  AssignmentStatus = "canceled"
)

type AssignmentSnapshot struct {
	Request         StepRequest      `json:"request"`
	Status          AssignmentStatus `json:"status"`
	ClaimedBy       string           `json:"claimedBy,omitempty"`
	CancelRequested bool             `json:"cancelRequested,omitempty"`
	LeaseDeadline   time.Time        `json:"leaseDeadline,omitempty"`
	Error           string           `json:"error,omitempty"`
	Completed       bool             `json:"completed,omitempty"`
}

type ClaimRequest struct {
	AgentID string `json:"agentId"`
}

type ClaimResponse struct {
	Assignment      *StepRequest `json:"assignment,omitempty"`
	PollAfterMillis int          `json:"pollAfterMillis,omitempty"`
}

type LeaseRequest struct {
	AgentID string `json:"agentId"`
}

type LeaseResponse struct {
	Status          AssignmentStatus `json:"status"`
	CancelRequested bool             `json:"cancelRequested"`
}

type StateReport struct {
	AgentID string `json:"agentId"`
	State   string `json:"state"`
	Message string `json:"message,omitempty"`
}

type LogReport struct {
	AgentID string         `json:"agentId"`
	Line    logstream.Line `json:"line"`
}

type CompleteRequest struct {
	AgentID string `json:"agentId"`
	Error   string `json:"error,omitempty"`
}

type Executor interface {
	Run(ctx context.Context, request StepRequest, emit func(logstream.Line)) error
}

type ExecutorFunc func(ctx context.Context, request StepRequest, emit func(logstream.Line)) error

func (fn ExecutorFunc) Run(ctx context.Context, request StepRequest, emit func(logstream.Line)) error {
	return fn(ctx, request, emit)
}

type Dispatcher interface {
	IsAvailable(ctx context.Context) bool
	Dispatch(ctx context.Context, request StepRequest, emit func(logstream.Line)) error
}
