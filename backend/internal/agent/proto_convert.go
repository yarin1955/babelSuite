package agent

import (
	"time"

	agentv1 "github.com/babelsuite/babelsuite/gen/proto/go/agent/v1"
	logstreamv1 "github.com/babelsuite/babelsuite/gen/proto/go/logstream/v1"
	"github.com/babelsuite/babelsuite/internal/logstream"
	"github.com/babelsuite/babelsuite/internal/suites"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func stepRequestToProto(r StepRequest) *agentv1.StepRequest {
	p := &agentv1.StepRequest{
		JobId:            r.JobID,
		ExecutionId:      r.ExecutionID,
		SuiteId:          r.SuiteID,
		SuiteTitle:       r.SuiteTitle,
		SuiteRepository:  r.SuiteRepository,
		Profile:          r.Profile,
		RuntimeProfile:   r.RuntimeProfile,
		Env:              r.Env,
		Headers:          r.Headers,
		Trigger:          r.Trigger,
		BackendId:        r.BackendID,
		BackendKind:      r.BackendKind,
		BackendLabel:     r.BackendLabel,
		SourceSuiteId:    r.SourceSuiteID,
		SourceSuiteTitle: r.SourceSuiteTitle,
		SourceRepository: r.SourceRepository,
		SourceVersion:    r.SourceVersion,
		ResolvedRef:      r.ResolvedRef,
		Digest:           r.Digest,
		DependencyAlias:  r.DependencyAlias,
		StepIndex:        int32(r.StepIndex),
		TotalSteps:       int32(r.TotalSteps),
		OnFailure:        r.OnFailure,
		Node:             stepNodeToProto(r.Node),
	}
	if r.LeaseTTL > 0 {
		p.LeaseTtl = durationpb.New(r.LeaseTTL)
	}
	if r.Load != nil {
		p.Load = loadSpecToProto(r.Load)
	}
	if r.Evaluation != nil {
		p.Evaluation = evaluationToProto(r.Evaluation)
	}
	for _, ae := range r.ArtifactExports {
		p.ArtifactExports = append(p.ArtifactExports, &agentv1.ArtifactExport{
			Path:   ae.Path,
			Name:   ae.Name,
			On:     ae.On,
			Format: ae.Format,
		})
	}
	return p
}

func protoToStepRequest(p *agentv1.StepRequest) StepRequest {
	if p == nil {
		return StepRequest{}
	}
	r := StepRequest{
		JobID:            p.JobId,
		ExecutionID:      p.ExecutionId,
		SuiteID:          p.SuiteId,
		SuiteTitle:       p.SuiteTitle,
		SuiteRepository:  p.SuiteRepository,
		Profile:          p.Profile,
		RuntimeProfile:   p.RuntimeProfile,
		Env:              p.Env,
		Headers:          p.Headers,
		Trigger:          p.Trigger,
		BackendID:        p.BackendId,
		BackendKind:      p.BackendKind,
		BackendLabel:     p.BackendLabel,
		SourceSuiteID:    p.SourceSuiteId,
		SourceSuiteTitle: p.SourceSuiteTitle,
		SourceRepository: p.SourceRepository,
		SourceVersion:    p.SourceVersion,
		ResolvedRef:      p.ResolvedRef,
		Digest:           p.Digest,
		DependencyAlias:  p.DependencyAlias,
		StepIndex:        int(p.StepIndex),
		TotalSteps:       int(p.TotalSteps),
		OnFailure:        p.OnFailure,
		Node:             protoToStepNode(p.Node),
	}
	if p.LeaseTtl != nil {
		r.LeaseTTL = p.LeaseTtl.AsDuration()
	}
	if p.Load != nil {
		r.Load = protoToLoadSpec(p.Load)
	}
	if p.Evaluation != nil {
		r.Evaluation = protoToEvaluation(p.Evaluation)
	}
	for _, ae := range p.ArtifactExports {
		r.ArtifactExports = append(r.ArtifactExports, ArtifactExport{
			Path:   ae.Path,
			Name:   ae.Name,
			On:     ae.On,
			Format: ae.Format,
		})
	}
	return r
}

func stepNodeToProto(n StepNode) *agentv1.StepNode {
	return &agentv1.StepNode{
		Id:        n.ID,
		Name:      n.Name,
		Kind:      n.Kind,
		Variant:   n.Variant,
		DependsOn: n.DependsOn,
	}
}

func protoToStepNode(p *agentv1.StepNode) StepNode {
	if p == nil {
		return StepNode{}
	}
	return StepNode{
		ID:        p.Id,
		Name:      p.Name,
		Kind:      p.Kind,
		Variant:   p.Variant,
		DependsOn: p.DependsOn,
	}
}

func loadSpecToProto(s *suites.LoadSpec) *agentv1.LoadSpec {
	if s == nil {
		return nil
	}
	p := &agentv1.LoadSpec{
		Variant:          s.Variant,
		PlanPath:         s.PlanPath,
		Target:           s.Target,
		RequestsPerSecond: s.RequestsPerS,
		ArrivalRate:      s.ArrivalRate,
	}
	for _, u := range s.Users {
		pu := &agentv1.LoadUser{
			Id:     u.ID,
			Name:   u.Name,
			Weight: int32(u.Weight),
			Wait: &agentv1.LoadWait{
				Mode:       u.Wait.Mode,
				Seconds:    u.Wait.Seconds,
				MinSeconds: u.Wait.MinSeconds,
				MaxSeconds: u.Wait.MaxSeconds,
			},
		}
		for _, t := range u.Tasks {
			pt := &agentv1.LoadTask{
				Id:     t.ID,
				Name:   t.Name,
				Weight: int32(t.Weight),
				Request: &agentv1.LoadRequest{
					Method:  t.Request.Method,
					Path:    t.Request.Path,
					Name:    t.Request.Name,
					Headers: t.Request.Headers,
					Body:    t.Request.Body,
				},
			}
			for _, c := range t.Request.Checks {
				pt.Request.Checks = append(pt.Request.Checks, thresholdToProto(c))
			}
			for _, c := range t.Checks {
				pt.Checks = append(pt.Checks, thresholdToProto(c))
			}
			pu.Tasks = append(pu.Tasks, pt)
		}
		p.Users = append(p.Users, pu)
	}
	for _, st := range s.Stages {
		ps := &agentv1.LoadStage{
			Users:     int32(st.Users),
			SpawnRate: st.SpawnRate,
			Stop:      st.Stop,
		}
		if st.Duration > 0 {
			ps.Duration = durationpb.New(st.Duration)
		}
		p.Stages = append(p.Stages, ps)
	}
	for _, t := range s.Thresholds {
		p.Thresholds = append(p.Thresholds, thresholdToProto(t))
	}
	return p
}

func protoToLoadSpec(p *agentv1.LoadSpec) *suites.LoadSpec {
	if p == nil {
		return nil
	}
	s := &suites.LoadSpec{
		Variant:      p.Variant,
		PlanPath:     p.PlanPath,
		Target:       p.Target,
		RequestsPerS: p.RequestsPerSecond,
		ArrivalRate:  p.ArrivalRate,
	}
	for _, pu := range p.Users {
		u := suites.LoadUser{
			ID:     pu.Id,
			Name:   pu.Name,
			Weight: int(pu.Weight),
		}
		if pu.Wait != nil {
			u.Wait = suites.LoadWait{
				Mode:       pu.Wait.Mode,
				Seconds:    pu.Wait.Seconds,
				MinSeconds: pu.Wait.MinSeconds,
				MaxSeconds: pu.Wait.MaxSeconds,
			}
		}
		for _, pt := range pu.Tasks {
			t := suites.LoadTask{
				ID:     pt.Id,
				Name:   pt.Name,
				Weight: int(pt.Weight),
			}
			if pt.Request != nil {
				t.Request = suites.LoadRequest{
					Method:  pt.Request.Method,
					Path:    pt.Request.Path,
					Name:    pt.Request.Name,
					Headers: pt.Request.Headers,
					Body:    pt.Request.Body,
				}
				for _, c := range pt.Request.Checks {
					t.Request.Checks = append(t.Request.Checks, protoToThreshold(c))
				}
			}
			for _, c := range pt.Checks {
				t.Checks = append(t.Checks, protoToThreshold(c))
			}
			u.Tasks = append(u.Tasks, t)
		}
		s.Users = append(s.Users, u)
	}
	for _, ps := range p.Stages {
		st := suites.LoadStage{
			Users:     int(ps.Users),
			SpawnRate: ps.SpawnRate,
			Stop:      ps.Stop,
		}
		if ps.Duration != nil {
			st.Duration = ps.Duration.AsDuration()
		}
		s.Stages = append(s.Stages, st)
	}
	for _, t := range p.Thresholds {
		s.Thresholds = append(s.Thresholds, protoToThreshold(t))
	}
	return s
}

func thresholdToProto(t suites.LoadThreshold) *agentv1.LoadThreshold {
	return &agentv1.LoadThreshold{
		Metric:  t.Metric,
		Op:      t.Op,
		Value:   t.Value,
		Sampler: t.Sampler,
	}
}

func protoToThreshold(p *agentv1.LoadThreshold) suites.LoadThreshold {
	if p == nil {
		return suites.LoadThreshold{}
	}
	return suites.LoadThreshold{
		Metric:  p.Metric,
		Op:      p.Op,
		Value:   p.Value,
		Sampler: p.Sampler,
	}
}

func evaluationToProto(e *suites.StepEvaluation) *agentv1.StepEvaluation {
	if e == nil {
		return nil
	}
	p := &agentv1.StepEvaluation{
		ExpectLogs: e.ExpectLogs,
		FailOnLogs: e.FailOnLogs,
	}
	if e.ExpectExit != nil {
		v := int32(*e.ExpectExit)
		p.ExpectExit = &v
	}
	return p
}

func protoToEvaluation(p *agentv1.StepEvaluation) *suites.StepEvaluation {
	if p == nil {
		return nil
	}
	e := &suites.StepEvaluation{
		ExpectLogs: p.ExpectLogs,
		FailOnLogs: p.FailOnLogs,
	}
	if p.ExpectExit != nil {
		v := int(*p.ExpectExit)
		e.ExpectExit = &v
	}
	return e
}

func lineToProto(l logstream.Line) *logstreamv1.Line {
	return &logstreamv1.Line{
		Source:    l.Source,
		Timestamp: l.Timestamp,
		Level:     l.Level,
		Kind:      l.Kind,
		Text:      l.Text,
	}
}

func protoToLine(p *logstreamv1.Line) logstream.Line {
	if p == nil {
		return logstream.Line{}
	}
	return logstream.Line{
		Source:    p.Source,
		Timestamp: p.Timestamp,
		Level:     p.Level,
		Kind:      p.Kind,
		Text:      p.Text,
	}
}

func registrationToProto(r Registration) *agentv1.Registration {
	return &agentv1.Registration{
		AgentId:         r.AgentID,
		Name:            r.Name,
		HostUrl:         r.HostURL,
		Status:          r.Status,
		Capabilities:    r.Capabilities,
		RegisteredAt:    timestamppb.New(r.RegisteredAt),
		LastHeartbeatAt: timestamppb.New(r.LastHeartbeatAt),
	}
}

func protoToRegistration(p *agentv1.Registration) Registration {
	if p == nil {
		return Registration{}
	}
	r := Registration{
		AgentID:      p.AgentId,
		Name:         p.Name,
		HostURL:      p.HostUrl,
		Status:       p.Status,
		Capabilities: p.Capabilities,
	}
	if p.RegisteredAt != nil {
		r.RegisteredAt = p.RegisteredAt.AsTime()
	}
	if p.LastHeartbeatAt != nil {
		r.LastHeartbeatAt = p.LastHeartbeatAt.AsTime()
	}
	return r
}

func assignmentStatusToProto(s AssignmentStatus) agentv1.AssignmentStatus {
	switch s {
	case AssignmentPending:
		return agentv1.AssignmentStatus_ASSIGNMENT_STATUS_PENDING
	case AssignmentRunning:
		return agentv1.AssignmentStatus_ASSIGNMENT_STATUS_RUNNING
	case AssignmentSucceeded:
		return agentv1.AssignmentStatus_ASSIGNMENT_STATUS_SUCCEEDED
	case AssignmentFailed:
		return agentv1.AssignmentStatus_ASSIGNMENT_STATUS_FAILED
	case AssignmentCanceled:
		return agentv1.AssignmentStatus_ASSIGNMENT_STATUS_CANCELED
	default:
		return agentv1.AssignmentStatus_ASSIGNMENT_STATUS_UNSPECIFIED
	}
}

func protoToAssignmentStatus(s agentv1.AssignmentStatus) AssignmentStatus {
	switch s {
	case agentv1.AssignmentStatus_ASSIGNMENT_STATUS_PENDING:
		return AssignmentPending
	case agentv1.AssignmentStatus_ASSIGNMENT_STATUS_RUNNING:
		return AssignmentRunning
	case agentv1.AssignmentStatus_ASSIGNMENT_STATUS_SUCCEEDED:
		return AssignmentSucceeded
	case agentv1.AssignmentStatus_ASSIGNMENT_STATUS_FAILED:
		return AssignmentFailed
	case agentv1.AssignmentStatus_ASSIGNMENT_STATUS_CANCELED:
		return AssignmentCanceled
	default:
		return ""
	}
}

// unused in production paths but satisfies the compiler for time import
var _ = time.Second
