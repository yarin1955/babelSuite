package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/cenkalti/backoff/v5"
)

const (
	defaultTimeout    = 30 * time.Second
	defaultMaxRetry   = 5 * time.Minute
	userAgent         = "babelsuite-mcp/1.0"
	watchPollInitial  = 2 * time.Second
	watchPollMax      = 30 * time.Second
	watchDefaultLimit = 30 * time.Minute
)

type clientOption func(*client) error

func withTimeout(d time.Duration) clientOption {
	return func(c *client) error {
		c.http.Timeout = d
		return nil
	}
}

type client struct {
	baseURL string
	token   string
	http    *http.Client
}

func newClient(baseURL, token string, opts ...clientOption) *client {
	c := &client{
		baseURL: strings.TrimRight(baseURL, "/"),
		token:   token,
		http:    &http.Client{Timeout: defaultTimeout},
	}
	for _, opt := range opts {
		_ = opt(c)
	}
	return c
}

func (c *client) get(ctx context.Context, path string, queryParams map[string]string) (json.RawMessage, error) {
	u, err := url.Parse(c.baseURL + path)
	if err != nil {
		return nil, backoff.Permanent(err)
	}
	if len(queryParams) > 0 {
		q := u.Query()
		for k, v := range queryParams {
			q.Set(k, v)
		}
		u.RawQuery = q.Encode()
	}
	return backoff.Retry(ctx, func() (json.RawMessage, error) {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
		if err != nil {
			return nil, backoff.Permanent(err)
		}
		return c.do(req)
	}, backoff.WithBackOff(newBackoff()), backoff.WithMaxElapsedTime(defaultMaxRetry))
}

func (c *client) post(ctx context.Context, path string, body any) (json.RawMessage, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(body); err != nil {
		return nil, err
	}
	raw := buf.Bytes()

	return backoff.Retry(ctx, func() (json.RawMessage, error) {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+path, bytes.NewReader(raw))
		if err != nil {
			return nil, backoff.Permanent(err)
		}
		req.Header.Set("Content-Type", "application/json")
		return c.do(req)
	}, backoff.WithBackOff(newBackoff()), backoff.WithMaxElapsedTime(defaultMaxRetry))
}

func (c *client) do(req *http.Request) (json.RawMessage, error) {
	req.Header.Set("User-Agent", userAgent)
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}

	resp, err := c.http.Do(req)
	if err != nil {
		// Network errors are transient — let the caller retry.
		return nil, err
	}
	defer resp.Body.Close()

	var buf bytes.Buffer
	tee := io.TeeReader(resp.Body, &buf)

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		data, _ := io.ReadAll(tee)
		msg := extractErrorMessage(data, resp.StatusCode)
		apiErr := fmt.Errorf(msg)
		// 4xx are permanent: retrying won't help.
		if resp.StatusCode >= 400 && resp.StatusCode < 500 {
			return nil, backoff.Permanent(apiErr)
		}
		return nil, apiErr
	}

	var result json.RawMessage
	if err := json.NewDecoder(tee).Decode(&result); err != nil && err != io.EOF {
		return nil, backoff.Permanent(fmt.Errorf("decode response: %s: %s", err, buf.String()))
	}
	return result, nil
}

// watchExecution polls GET /api/v1/executions/{id} until the execution reaches
// a terminal status (healthy or failed) or the context deadline is exceeded.
// The poll interval grows with exponential backoff starting at watchPollInitial.
func (c *client) watchExecution(ctx context.Context, id string, timeout time.Duration) (json.RawMessage, error) {
	if timeout <= 0 {
		timeout = watchDefaultLimit
	}
	deadline := time.Now().Add(timeout)

	b := backoff.NewExponentialBackOff()
	b.InitialInterval = watchPollInitial
	b.MaxInterval = watchPollMax
	b.Reset()

	for {
		data, err := c.get(ctx, "/api/v1/executions/"+id, nil)
		if err != nil {
			return nil, err
		}

		var record struct {
			Status string `json:"status"`
		}
		if json.Unmarshal(data, &record) == nil {
			switch record.Status {
			case "healthy", "failed":
				return data, nil
			}
		}

		if time.Now().After(deadline) {
			return data, fmt.Errorf("watch timed out after %s", timeout)
		}

		wait := b.NextBackOff()
		if wait == backoff.Stop {
			return data, fmt.Errorf("watch backoff exhausted")
		}

		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(wait):
		}
	}
}

func (c *client) signIn(ctx context.Context, email, password string) (string, error) {
	data, err := c.post(ctx, "/api/v1/auth/sign-in", map[string]string{"email": email, "password": password})
	if err != nil {
		return "", err
	}
	var resp struct {
		Token string `json:"token"`
	}
	if err := json.Unmarshal(data, &resp); err != nil {
		return "", err
	}
	return resp.Token, nil
}

func newBackoff() *backoff.ExponentialBackOff {
	b := backoff.NewExponentialBackOff()
	b.InitialInterval = 100 * time.Millisecond
	b.MaxInterval = 10 * time.Second
	b.Reset()
	return b
}

func extractErrorMessage(data []byte, statusCode int) string {
	var raw map[string]any
	if json.Unmarshal(data, &raw) == nil {
		if msg, ok := raw["error"].(string); ok && msg != "" {
			return fmt.Sprintf("API error %d: %s", statusCode, msg)
		}
		if msg, ok := raw["message"].(string); ok && msg != "" {
			return fmt.Sprintf("API error %d: %s", statusCode, msg)
		}
	}
	body := strings.TrimSpace(string(data))
	if body == "" {
		return fmt.Sprintf("API error %d", statusCode)
	}
	return fmt.Sprintf("API error %d: %s", statusCode, body)
}
