package agent

import (
	"context"
	"errors"

	"connectrpc.com/connect"
	agentv1 "github.com/babelsuite/babelsuite/gen/proto/go/agent/v1"
	"github.com/babelsuite/babelsuite/gen/proto/go/agent/v1/agentv1connect"
	"google.golang.org/protobuf/types/known/emptypb"
)

type controlServiceServer struct {
	coordinator *Coordinator
}

var _ agentv1connect.AgentControlServiceHandler = (*controlServiceServer)(nil)

func (s *controlServiceServer) ClaimNext(
	_ context.Context,
	req *connect.Request[agentv1.ClaimRequest],
) (*connect.Response[agentv1.ClaimResponse], error) {
	if s.coordinator == nil {
		return nil, connect.NewError(connect.CodeUnavailable, errors.New("coordinator unavailable"))
	}
	assignment, ok := s.coordinator.Claim(req.Msg.AgentId)
	resp := &agentv1.ClaimResponse{}
	if ok {
		resp.HasAssignment = true
		resp.Assignment = stepRequestToProto(*assignment)
	}
	return connect.NewResponse(resp), nil
}

func (s *controlServiceServer) ExtendLease(
	_ context.Context,
	req *connect.Request[agentv1.LeaseRequest],
) (*connect.Response[agentv1.LeaseResponse], error) {
	if s.coordinator == nil {
		return nil, connect.NewError(connect.CodeUnavailable, errors.New("coordinator unavailable"))
	}
	lease, err := s.coordinator.Extend(req.Msg.JobId, req.Msg.AgentId)
	if err != nil {
		if errors.Is(err, ErrAssignmentNotFound) {
			return nil, connect.NewError(connect.CodeNotFound, err)
		}
		return nil, connect.NewError(connect.CodePermissionDenied, err)
	}
	return connect.NewResponse(&agentv1.LeaseResponse{
		Status:          assignmentStatusToProto(lease.Status),
		CancelRequested: lease.CancelRequested,
	}), nil
}

func (s *controlServiceServer) ReportState(
	_ context.Context,
	req *connect.Request[agentv1.StateReport],
) (*connect.Response[emptypb.Empty], error) {
	if s.coordinator == nil {
		return nil, connect.NewError(connect.CodeUnavailable, errors.New("coordinator unavailable"))
	}
	err := s.coordinator.ReportState(req.Msg.JobId, StateReport{
		AgentID: req.Msg.AgentId,
		State:   req.Msg.State,
		Message: req.Msg.Message,
	})
	if err != nil {
		if errors.Is(err, ErrAssignmentNotFound) {
			return nil, connect.NewError(connect.CodeNotFound, err)
		}
		return nil, connect.NewError(connect.CodePermissionDenied, err)
	}
	return connect.NewResponse(&emptypb.Empty{}), nil
}

func (s *controlServiceServer) ReportLog(
	_ context.Context,
	req *connect.Request[agentv1.LogReport],
) (*connect.Response[emptypb.Empty], error) {
	if s.coordinator == nil {
		return nil, connect.NewError(connect.CodeUnavailable, errors.New("coordinator unavailable"))
	}
	err := s.coordinator.ReportLog(req.Msg.JobId, LogReport{
		AgentID: req.Msg.AgentId,
		Line:    protoToLine(req.Msg.Line),
	})
	if err != nil {
		if errors.Is(err, ErrAssignmentNotFound) {
			return nil, connect.NewError(connect.CodeNotFound, err)
		}
		return nil, connect.NewError(connect.CodePermissionDenied, err)
	}
	return connect.NewResponse(&emptypb.Empty{}), nil
}

func (s *controlServiceServer) Complete(
	_ context.Context,
	req *connect.Request[agentv1.CompleteRequest],
) (*connect.Response[emptypb.Empty], error) {
	if s.coordinator == nil {
		return nil, connect.NewError(connect.CodeUnavailable, errors.New("coordinator unavailable"))
	}
	err := s.coordinator.Complete(req.Msg.JobId, CompleteRequest{
		AgentID: req.Msg.AgentId,
		Error:   req.Msg.Error,
	})
	if err != nil {
		if errors.Is(err, ErrAssignmentNotFound) {
			return nil, connect.NewError(connect.CodeNotFound, err)
		}
		return nil, connect.NewError(connect.CodePermissionDenied, err)
	}
	return connect.NewResponse(&emptypb.Empty{}), nil
}

type registryServiceServer struct {
	registry *Registry
}

var _ agentv1connect.AgentRegistryServiceHandler = (*registryServiceServer)(nil)

func (s *registryServiceServer) Register(
	_ context.Context,
	req *connect.Request[agentv1.RegisterRequest],
) (*connect.Response[agentv1.Registration], error) {
	if s.registry == nil {
		return nil, connect.NewError(connect.CodeUnavailable, errors.New("registry unavailable"))
	}
	reg := s.registry.Register(RegisterRequest{
		AgentID:      req.Msg.AgentId,
		Name:         req.Msg.Name,
		HostURL:      req.Msg.HostUrl,
		Capabilities: req.Msg.Capabilities,
	})
	return connect.NewResponse(registrationToProto(reg)), nil
}

func (s *registryServiceServer) Heartbeat(
	_ context.Context,
	req *connect.Request[agentv1.HeartbeatRequest],
) (*connect.Response[agentv1.Registration], error) {
	if s.registry == nil {
		return nil, connect.NewError(connect.CodeUnavailable, errors.New("registry unavailable"))
	}
	reg, ok := s.registry.Heartbeat(req.Msg.AgentId)
	if !ok {
		return nil, connect.NewError(connect.CodeNotFound, errors.New("agent not found"))
	}
	return connect.NewResponse(registrationToProto(reg)), nil
}

func (s *registryServiceServer) Unregister(
	_ context.Context,
	req *connect.Request[agentv1.HeartbeatRequest],
) (*connect.Response[emptypb.Empty], error) {
	if s.registry != nil {
		s.registry.Unregister(req.Msg.AgentId)
	}
	return connect.NewResponse(&emptypb.Empty{}), nil
}

func (s *registryServiceServer) List(
	_ context.Context,
	_ *connect.Request[agentv1.ListAgentsRequest],
) (*connect.Response[agentv1.ListAgentsResponse], error) {
	resp := &agentv1.ListAgentsResponse{}
	if s.registry != nil {
		for _, r := range s.registry.List() {
			resp.Agents = append(resp.Agents, registrationToProto(r))
		}
	}
	return connect.NewResponse(resp), nil
}
