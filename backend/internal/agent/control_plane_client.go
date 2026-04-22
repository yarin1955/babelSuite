package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/babelsuite/babelsuite/internal/logstream"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
)

type ControlPlaneClient struct {
	baseURL string
	client  *http.Client
	secret  string
}

func NewControlPlaneClient(baseURL string, client *http.Client, secret string) *ControlPlaneClient {
	if client == nil {
		client = &http.Client{
			Timeout:   5 * time.Second,
			Transport: otelhttp.NewTransport(http.DefaultTransport),
		}
	}
	return &ControlPlaneClient{
		baseURL: strings.TrimRight(strings.TrimSpace(baseURL), "/"),
		client:  client,
		secret:  secret,
	}
}

func (c *ControlPlaneClient) addAuth(r *http.Request) {
	if c.secret != "" {
		r.Header.Set("Authorization", "Bearer "+c.secret)
	}
}

func (c *ControlPlaneClient) Register(ctx context.Context, request RegisterRequest) error {
	return c.postJSON(ctx, c.baseURL+"/api/v1/agents/register", request)
}

func (c *ControlPlaneClient) Heartbeat(ctx context.Context, agentID string) error {
	return c.postJSON(ctx, c.baseURL+"/api/v1/agents/"+agentID+"/heartbeat", nil)
}

func (c *ControlPlaneClient) ClaimNext(ctx context.Context, agentID string) (*StepRequest, error) {
	if c == nil || c.baseURL == "" || c.client == nil {
		return nil, nil
	}

	payload, err := json.Marshal(ClaimRequest{AgentID: agentID})
	if err != nil {
		return nil, err
	}

	request, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/api/v1/agent-control/claims/next", bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	request.Header.Set("Content-Type", "application/json")
	c.addAuth(request)

	response, err := c.client.Do(request)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	if response.StatusCode == http.StatusNoContent {
		return nil, nil
	}
	if response.StatusCode >= http.StatusBadRequest {
		return nil, fmt.Errorf("claim returned status %d", response.StatusCode)
	}

	var claim ClaimResponse
	if err := json.NewDecoder(response.Body).Decode(&claim); err != nil {
		return nil, err
	}
	return claim.Assignment, nil
}

func (c *ControlPlaneClient) ExtendLease(ctx context.Context, jobID, agentID string) (LeaseResponse, error) {
	var response LeaseResponse
	err := c.postJSONInto(ctx, c.baseURL+"/api/v1/agent-control/jobs/"+jobID+"/lease", LeaseRequest{AgentID: agentID}, &response)
	return response, err
}

func (c *ControlPlaneClient) ReportState(ctx context.Context, jobID string, report StateReport) error {
	return c.postJSON(ctx, c.baseURL+"/api/v1/agent-control/jobs/"+jobID+"/state", report)
}

func (c *ControlPlaneClient) ReportLog(ctx context.Context, jobID, agentID string, line logstream.Line) error {
	return c.postJSON(ctx, c.baseURL+"/api/v1/agent-control/jobs/"+jobID+"/logs", LogReport{
		AgentID: agentID,
		Line:    line,
	})
}

func (c *ControlPlaneClient) Complete(ctx context.Context, jobID string, request CompleteRequest) error {
	return c.postJSON(ctx, c.baseURL+"/api/v1/agent-control/jobs/"+jobID+"/complete", request)
}

func (c *ControlPlaneClient) Unregister(ctx context.Context, agentID string) error {
	if c == nil || c.baseURL == "" || c.client == nil {
		return nil
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodDelete, c.baseURL+"/api/v1/agents/"+agentID, nil)
	if err != nil {
		return err
	}
	c.addAuth(request)
	response, err := c.client.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode >= http.StatusBadRequest {
		return fmt.Errorf("unregister returned status %d", response.StatusCode)
	}
	return nil
}

func (c *ControlPlaneClient) postJSON(ctx context.Context, url string, payload any) error {
	if c == nil || c.baseURL == "" || c.client == nil {
		return nil
	}

	var body []byte
	if payload != nil {
		encoded, err := json.Marshal(payload)
		if err != nil {
			return err
		}
		body = encoded
	}

	request, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return err
	}
	request.Header.Set("Content-Type", "application/json")
	c.addAuth(request)

	response, err := c.client.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()

	if response.StatusCode >= http.StatusBadRequest {
		return fmt.Errorf("request returned status %d", response.StatusCode)
	}
	return nil
}

func (c *ControlPlaneClient) postJSONInto(ctx context.Context, url string, payload any, target any) error {
	if c == nil || c.baseURL == "" || c.client == nil {
		return nil
	}

	encoded, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	request, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(encoded))
	if err != nil {
		return err
	}
	request.Header.Set("Content-Type", "application/json")
	c.addAuth(request)

	response, err := c.client.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()

	if response.StatusCode >= http.StatusBadRequest {
		return fmt.Errorf("request returned status %d", response.StatusCode)
	}
	if target == nil {
		return nil
	}
	return json.NewDecoder(response.Body).Decode(target)
}
