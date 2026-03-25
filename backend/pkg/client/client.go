package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/babelsuite/babelsuite/internal/telemetry"
	"github.com/babelsuite/babelsuite/pkg/api"
)

type Client struct {
	baseURL string
	token   string
	http    *http.Client
}

type Option func(*Client)

type APIError struct {
	StatusCode int
	Message    string
}

func (e *APIError) Error() string {
	if e == nil {
		return ""
	}
	if e.Message == "" {
		return fmt.Sprintf("request failed with status %d", e.StatusCode)
	}
	return fmt.Sprintf("request failed with status %d: %s", e.StatusCode, e.Message)
}

func New(baseURL string, opts ...Option) *Client {
	c := &Client{
		baseURL: strings.TrimRight(baseURL, "/"),
		http:    &http.Client{Timeout: 30 * time.Second},
	}
	for _, opt := range opts {
		opt(c)
	}
	c.http = telemetry.WrapClient(c.http)
	return c
}

func WithHTTPClient(httpClient *http.Client) Option {
	return func(c *Client) {
		if httpClient != nil {
			c.http = httpClient
		}
	}
}

func WithToken(token string) Option {
	return func(c *Client) {
		c.token = token
	}
}

func (c *Client) BaseURL() string {
	return c.baseURL
}

func (c *Client) Token() string {
	return c.token
}

func (c *Client) SetToken(token string) {
	c.token = token
}

func (c *Client) Login(ctx context.Context, req api.LoginRequest) (*api.AuthResponse, error) {
	var resp api.AuthResponse
	if err := c.doJSON(ctx, http.MethodPost, "/auth/login", req, &resp, http.StatusOK); err != nil {
		return nil, err
	}
	return &resp, nil
}

func (c *Client) Me(ctx context.Context) (*api.User, error) {
	var user api.User
	if err := c.doJSON(ctx, http.MethodGet, "/auth/me", nil, &user, http.StatusOK); err != nil {
		return nil, err
	}
	return &user, nil
}

func (c *Client) ListCatalog(ctx context.Context, search string, page, pageSize int) (*api.CatalogListResponse, error) {
	query := url.Values{}
	if search != "" {
		query.Set("q", search)
	}
	if page > 0 {
		query.Set("page", strconv.Itoa(page))
	}
	if pageSize > 0 {
		query.Set("page_size", strconv.Itoa(pageSize))
	}

	var resp api.CatalogListResponse
	if err := c.doJSON(ctx, http.MethodGet, withQuery("/api/catalog", query), nil, &resp, http.StatusOK); err != nil {
		return nil, err
	}
	return &resp, nil
}

func (c *Client) GetPackage(ctx context.Context, packageID string) (*api.CatalogPackage, error) {
	var pkg api.CatalogPackage
	if err := c.doJSON(ctx, http.MethodGet, "/api/catalog/"+url.PathEscape(packageID), nil, &pkg, http.StatusOK); err != nil {
		return nil, err
	}
	return &pkg, nil
}

func (c *Client) ListRuns(ctx context.Context, page int) (*api.RunListResponse, error) {
	query := url.Values{}
	if page > 0 {
		query.Set("page", strconv.Itoa(page))
	}

	var resp api.RunListResponse
	if err := c.doJSON(ctx, http.MethodGet, withQuery("/api/runs", query), nil, &resp, http.StatusOK); err != nil {
		return nil, err
	}
	return &resp, nil
}

func (c *Client) CreateRun(ctx context.Context, packageID, profile string) (*api.Run, error) {
	var run api.Run
	if err := c.doJSON(ctx, http.MethodPost, "/api/runs", api.CreateRunRequest{PackageID: packageID, Profile: profile}, &run, http.StatusCreated); err != nil {
		return nil, err
	}
	return &run, nil
}

func (c *Client) GetRun(ctx context.Context, runID string) (*api.Run, error) {
	var run api.Run
	if err := c.doJSON(ctx, http.MethodGet, "/api/runs/"+url.PathEscape(runID), nil, &run, http.StatusOK); err != nil {
		return nil, err
	}
	return &run, nil
}

func (c *Client) CancelRun(ctx context.Context, runID string) error {
	return c.doJSON(ctx, http.MethodDelete, "/api/runs/"+url.PathEscape(runID), nil, nil, http.StatusNoContent)
}

func (c *Client) ListSteps(ctx context.Context, runID string) ([]*api.Step, error) {
	var steps []*api.Step
	if err := c.doJSON(ctx, http.MethodGet, "/api/runs/"+url.PathEscape(runID)+"/steps", nil, &steps, http.StatusOK); err != nil {
		return nil, err
	}
	if steps == nil {
		steps = []*api.Step{}
	}
	return steps, nil
}

func (c *Client) HistoryLogs(ctx context.Context, runID, stepID string) ([]*api.LogEntry, error) {
	var logs []*api.LogEntry
	path := fmt.Sprintf("/api/runs/%s/logs/%s/history", url.PathEscape(runID), url.PathEscape(stepID))
	if err := c.doJSON(ctx, http.MethodGet, path, nil, &logs, http.StatusOK); err != nil {
		return nil, err
	}
	if logs == nil {
		logs = []*api.LogEntry{}
	}
	return logs, nil
}

func (c *Client) ListAgents(ctx context.Context) ([]*api.Agent, error) {
	var agents []*api.Agent
	if err := c.doJSON(ctx, http.MethodGet, "/api/agents", nil, &agents, http.StatusOK); err != nil {
		return nil, err
	}
	if agents == nil {
		agents = []*api.Agent{}
	}
	return agents, nil
}

func (c *Client) GetAgent(ctx context.Context, agentID string) (*api.Agent, error) {
	var agent api.Agent
	if err := c.doJSON(ctx, http.MethodGet, "/api/agents/"+url.PathEscape(agentID), nil, &agent, http.StatusOK); err != nil {
		return nil, err
	}
	return &agent, nil
}

func (c *Client) CreateAgent(ctx context.Context, req api.CreateAgentRequest) (*api.CreateAgentResponse, error) {
	var resp api.CreateAgentResponse
	if err := c.doJSON(ctx, http.MethodPost, "/api/agents", req, &resp, http.StatusCreated); err != nil {
		return nil, err
	}
	return &resp, nil
}

func (c *Client) UpdateAgent(ctx context.Context, agentID string, req api.UpdateAgentRequest) (*api.Agent, error) {
	var agent api.Agent
	if err := c.doJSON(ctx, http.MethodPatch, "/api/agents/"+url.PathEscape(agentID), req, &agent, http.StatusOK); err != nil {
		return nil, err
	}
	return &agent, nil
}

func (c *Client) DeleteAgent(ctx context.Context, agentID string) error {
	return c.doJSON(ctx, http.MethodDelete, "/api/agents/"+url.PathEscape(agentID), nil, nil, http.StatusNoContent)
}

func (c *Client) AgentRegister(ctx context.Context, req api.AgentRegisterRequest) (*api.AgentRegisterResponse, error) {
	var resp api.AgentRegisterResponse
	if err := c.doJSON(ctx, http.MethodPost, "/api/agent/register", req, &resp, http.StatusOK); err != nil {
		return nil, err
	}
	return &resp, nil
}

func (c *Client) AgentHealth(ctx context.Context) error {
	return c.doJSON(ctx, http.MethodPost, "/api/agent/health", nil, nil, http.StatusNoContent)
}

func (c *Client) AgentNextRun(ctx context.Context) (*api.Run, error) {
	resp, err := c.do(ctx, http.MethodGet, "/api/agent/runs/next", nil, http.StatusOK, http.StatusNoContent)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNoContent {
		return nil, nil
	}

	var run api.Run
	if err := json.NewDecoder(resp.Body).Decode(&run); err != nil {
		return nil, err
	}
	return &run, nil
}

func (c *Client) AgentWaitRun(ctx context.Context, runID string) (*api.WaitRunResponse, error) {
	resp, err := c.do(ctx, http.MethodGet, "/api/agent/runs/"+url.PathEscape(runID)+"/wait", nil, http.StatusOK, http.StatusNoContent)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNoContent {
		return nil, nil
	}

	var waitResp api.WaitRunResponse
	if err := json.NewDecoder(resp.Body).Decode(&waitResp); err != nil {
		return nil, err
	}
	return &waitResp, nil
}

func (c *Client) AgentUpdateRun(ctx context.Context, runID string, req api.UpdateRunRequest) (*api.Run, error) {
	var run api.Run
	if err := c.doJSON(ctx, http.MethodPatch, "/api/agent/runs/"+url.PathEscape(runID), req, &run, http.StatusOK); err != nil {
		return nil, err
	}
	return &run, nil
}

func (c *Client) AgentCreateStep(ctx context.Context, runID string, req api.CreateStepRequest) (*api.Step, error) {
	var step api.Step
	if err := c.doJSON(ctx, http.MethodPost, "/api/agent/runs/"+url.PathEscape(runID)+"/steps", req, &step, http.StatusCreated); err != nil {
		return nil, err
	}
	return &step, nil
}

func (c *Client) AgentUpdateStep(ctx context.Context, runID, stepID string, req api.UpdateStepRequest) (*api.Step, error) {
	var step api.Step
	path := fmt.Sprintf("/api/agent/runs/%s/steps/%s", url.PathEscape(runID), url.PathEscape(stepID))
	if err := c.doJSON(ctx, http.MethodPatch, path, req, &step, http.StatusOK); err != nil {
		return nil, err
	}
	return &step, nil
}

func (c *Client) AgentAppendLogs(ctx context.Context, runID, stepID string, lines []api.StepLogLine) error {
	path := fmt.Sprintf("/api/agent/runs/%s/steps/%s/logs", url.PathEscape(runID), url.PathEscape(stepID))
	return c.doJSON(ctx, http.MethodPost, path, lines, nil, http.StatusNoContent)
}

func (c *Client) doJSON(ctx context.Context, method, path string, reqBody any, out any, okStatuses ...int) error {
	resp, err := c.do(ctx, method, path, reqBody, okStatuses...)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if out == nil || resp.StatusCode == http.StatusNoContent {
		return nil
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

func (c *Client) do(ctx context.Context, method, path string, reqBody any, okStatuses ...int) (*http.Response, error) {
	var body io.Reader
	if reqBody != nil {
		data, err := json.Marshal(reqBody)
		if err != nil {
			return nil, err
		}
		body = bytes.NewReader(data)
	}

	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, body)
	if err != nil {
		return nil, err
	}
	if reqBody != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}

	for _, status := range okStatuses {
		if resp.StatusCode == status {
			return resp, nil
		}
	}

	defer resp.Body.Close()
	return nil, decodeAPIError(resp)
}

func decodeAPIError(resp *http.Response) error {
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 8192))
	if len(body) > 0 {
		var payload api.ErrorResponse
		if err := json.Unmarshal(body, &payload); err == nil && payload.Error != "" {
			return &APIError{StatusCode: resp.StatusCode, Message: payload.Error}
		}
	}

	msg := strings.TrimSpace(string(body))
	if msg == "" {
		msg = resp.Status
	}
	return &APIError{StatusCode: resp.StatusCode, Message: msg}
}

func withQuery(path string, query url.Values) string {
	if len(query) == 0 {
		return path
	}
	return path + "?" + query.Encode()
}
