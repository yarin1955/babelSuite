package apiclient

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

type Error struct {
	StatusCode int
	Message    string
}

func (e *Error) Error() string {
	if strings.TrimSpace(e.Message) != "" {
		return e.Message
	}
	return fmt.Sprintf("BabelSuite API returned HTTP %d", e.StatusCode)
}

type errorEnvelope struct {
	Error string `json:"error"`
}

type Client struct {
	BaseURL    string
	Token      string
	HTTPClient *http.Client
}

func New(baseURL, token string) *Client {
	return &Client{
		BaseURL: normalizeServerURL(baseURL),
		Token:   strings.TrimSpace(token),
		HTTPClient: &http.Client{
			Timeout: 15 * time.Second,
		},
	}
}

func (c *Client) SignIn(ctx context.Context, email, password string) (*AuthResponse, error) {
	var response AuthResponse
	err := c.doJSON(ctx, http.MethodPost, "/api/v1/auth/sign-in", map[string]string{
		"email":    strings.TrimSpace(email),
		"password": password,
	}, &response)
	if err != nil {
		return nil, err
	}
	return &response, nil
}

func (c *Client) Me(ctx context.Context) (*User, error) {
	var user User
	if err := c.doJSON(ctx, http.MethodGet, "/api/v1/auth/me", nil, &user); err != nil {
		return nil, err
	}
	return &user, nil
}

func (c *Client) ListCatalog(ctx context.Context) ([]CatalogPackage, error) {
	var response struct {
		Packages []CatalogPackage `json:"packages"`
	}
	if err := c.doJSON(ctx, http.MethodGet, "/api/v1/catalog/packages", nil, &response); err != nil {
		return nil, err
	}
	return response.Packages, nil
}

func (c *Client) GetCatalogPackage(ctx context.Context, packageID string) (*CatalogPackage, error) {
	var item CatalogPackage
	if err := c.doJSON(ctx, http.MethodGet, "/api/v1/catalog/packages/"+url.PathEscape(strings.TrimSpace(packageID)), nil, &item); err != nil {
		return nil, err
	}
	return &item, nil
}

func (c *Client) ListSuites(ctx context.Context) ([]SuiteDefinition, error) {
	var response struct {
		Suites []SuiteDefinition `json:"suites"`
	}
	if err := c.doJSON(ctx, http.MethodGet, "/api/v1/suites", nil, &response); err != nil {
		return nil, err
	}
	return response.Suites, nil
}

func (c *Client) GetSuite(ctx context.Context, suiteID string) (*SuiteDefinition, error) {
	var suite SuiteDefinition
	if err := c.doJSON(ctx, http.MethodGet, "/api/v1/suites/"+url.PathEscape(strings.TrimSpace(suiteID)), nil, &suite); err != nil {
		return nil, err
	}
	return &suite, nil
}

func (c *Client) ListProfiles(ctx context.Context, suiteID string) (*SuiteProfilesResponse, error) {
	var response SuiteProfilesResponse
	if err := c.doJSON(ctx, http.MethodGet, "/api/v1/profiles/suites/"+url.PathEscape(strings.TrimSpace(suiteID)), nil, &response); err != nil {
		return nil, err
	}
	return &response, nil
}

func (c *Client) ListLaunchSuites(ctx context.Context) ([]LaunchSuite, error) {
	var response struct {
		Suites []LaunchSuite `json:"suites"`
	}
	if err := c.doJSON(ctx, http.MethodGet, "/api/v1/executions/launch-suites", nil, &response); err != nil {
		return nil, err
	}
	return response.Suites, nil
}

func (c *Client) ListRuns(ctx context.Context) ([]ExecutionSummary, error) {
	var response struct {
		Executions []ExecutionSummary `json:"executions"`
	}
	if err := c.doJSON(ctx, http.MethodGet, "/api/v1/executions", nil, &response); err != nil {
		return nil, err
	}
	return response.Executions, nil
}

func (c *Client) GetRun(ctx context.Context, executionID string) (*ExecutionRecord, error) {
	var execution ExecutionRecord
	if err := c.doJSON(ctx, http.MethodGet, "/api/v1/executions/"+url.PathEscape(strings.TrimSpace(executionID)), nil, &execution); err != nil {
		return nil, err
	}
	return &execution, nil
}

func (c *Client) CreateRun(ctx context.Context, suiteID, profile string) (*ExecutionSummary, error) {
	var execution ExecutionSummary
	payload := map[string]string{
		"suiteId": strings.TrimSpace(suiteID),
		"profile": strings.TrimSpace(profile),
	}
	if err := c.doJSON(ctx, http.MethodPost, "/api/v1/executions", payload, &execution); err != nil {
		return nil, err
	}
	return &execution, nil
}

func (c *Client) ListEnvironments(ctx context.Context) (*SandboxesResponse, error) {
	var response SandboxesResponse
	if err := c.doJSON(ctx, http.MethodGet, "/api/v1/sandboxes", nil, &response); err != nil {
		return nil, err
	}
	return &response, nil
}

func (c *Client) ReapEnvironment(ctx context.Context, sandboxID string) (*ReapResult, error) {
	var response ReapResult
	if err := c.doJSON(ctx, http.MethodPost, "/api/v1/sandboxes/"+url.PathEscape(strings.TrimSpace(sandboxID))+"/reap", nil, &response); err != nil {
		return nil, err
	}
	return &response, nil
}

func (c *Client) ReapAllEnvironments(ctx context.Context) (*ReapResult, error) {
	var response ReapResult
	if err := c.doJSON(ctx, http.MethodPost, "/api/v1/sandboxes/reap-all", nil, &response); err != nil {
		return nil, err
	}
	return &response, nil
}

func (c *Client) doJSON(ctx context.Context, method, path string, body any, out any) error {
	var payload io.Reader
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			return err
		}
		payload = bytes.NewReader(encoded)
	}

	req, err := http.NewRequestWithContext(ctx, method, strings.TrimRight(c.BaseURL, "/")+path, payload)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if strings.TrimSpace(c.Token) != "" {
		req.Header.Set("Authorization", "Bearer "+strings.TrimSpace(c.Token))
	}

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return parseError(resp)
	}

	if out == nil || resp.StatusCode == http.StatusNoContent {
		return nil
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

func parseError(resp *http.Response) error {
	body, _ := io.ReadAll(resp.Body)

	var envelope errorEnvelope
	if json.Unmarshal(body, &envelope) == nil && strings.TrimSpace(envelope.Error) != "" {
		return &Error{StatusCode: resp.StatusCode, Message: envelope.Error}
	}

	message := strings.TrimSpace(string(body))
	if message == "" {
		message = resp.Status
	}
	return &Error{StatusCode: resp.StatusCode, Message: message}
}

func normalizeServerURL(raw string) string {
	value := firstNonEmpty(strings.TrimSpace(raw), strings.TrimSpace(os.Getenv("BABELSUITE_SERVER")), "http://localhost:8090")
	if !strings.Contains(value, "://") {
		value = "http://" + value
	}
	return strings.TrimRight(value, "/")
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
