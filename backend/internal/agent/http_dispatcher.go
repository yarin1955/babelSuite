package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/babelsuite/babelsuite/internal/logstream"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel/trace"
)

type HTTPDispatcher struct {
	baseURL string
	client  *http.Client
	secret  string
}

func NewHTTPDispatcher(baseURL string, client *http.Client, secret string) *HTTPDispatcher {
	if client == nil {
		client = &http.Client{
			Timeout:   30 * time.Second,
			Transport: otelhttp.NewTransport(http.DefaultTransport),
		}
	}
	return &HTTPDispatcher{
		baseURL: strings.TrimRight(strings.TrimSpace(baseURL), "/"),
		client:  client,
		secret:  secret,
	}
}

func (d *HTTPDispatcher) addAuth(r *http.Request) {
	if d.secret != "" {
		r.Header.Set("Authorization", "Bearer "+d.secret)
	}
}

func (d *HTTPDispatcher) IsAvailable(ctx context.Context) bool {
	if d == nil || d.baseURL == "" || d.client == nil {
		return false
	}

	request, err := http.NewRequestWithContext(ctx, http.MethodGet, d.baseURL+"/healthz", nil)
	if err != nil {
		return false
	}

	response, err := d.client.Do(request)
	if err != nil {
		return false
	}
	defer response.Body.Close()

	return response.StatusCode == http.StatusOK
}

func (d *HTTPDispatcher) Dispatch(ctx context.Context, request StepRequest, emit func(logstream.Line)) error {
	if d == nil || d.baseURL == "" || d.client == nil {
		return errors.New("remote dispatcher is not configured")
	}

	spanCtx, span := agentMetrics.tracer.Start(ctx, "agent.dispatch",
		trace.WithAttributes(jobAttributes(request)...),
	)
	defer span.End()

	request.JobID = firstNonEmpty(request.JobID, request.ExecutionID+":"+request.Node.ID)
	payload, err := json.Marshal(request)
	if err != nil {
		return err
	}

	requestCtx, cancelRequest := context.WithCancel(spanCtx)
	defer cancelRequest()

	done := make(chan struct{})
	go func() {
		select {
		case <-ctx.Done():
			d.cancelJob(request.JobID)
			cancelRequest()
		case <-done:
		}
	}()
	defer close(done)

	httpRequest, err := http.NewRequestWithContext(requestCtx, http.MethodPost, d.baseURL+"/api/v1/agent/run", bytes.NewReader(payload))
	if err != nil {
		return err
	}
	httpRequest.Header.Set("Content-Type", "application/json")
	d.addAuth(httpRequest)

	response, err := d.client.Do(httpRequest)
	if err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return err
	}
	defer response.Body.Close()

	if response.StatusCode >= http.StatusBadRequest {
		return fmt.Errorf("remote worker returned status %d", response.StatusCode)
	}

	decoder := json.NewDecoder(response.Body)
	for {
		var message StreamMessage
		if err := decoder.Decode(&message); err != nil {
			if errors.Is(err, io.EOF) {
				if ctx.Err() != nil {
					return ctx.Err()
				}
				return nil
			}
			if ctx.Err() != nil {
				return ctx.Err()
			}
			return err
		}

		switch message.Type {
		case "log":
			if message.Line != nil && emit != nil {
				emit(*message.Line)
			}
		case "done":
			if strings.TrimSpace(message.Error) != "" {
				_ = d.cleanupJob(message.JobID)
				return errors.New(message.Error)
			}
			_ = d.cleanupJob(message.JobID)
			return nil
		}
	}
}

func (d *HTTPDispatcher) cancelJob(jobID string) {
	if jobID == "" || d.baseURL == "" || d.client == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, d.baseURL+"/api/v1/agent/jobs/"+url.PathEscape(jobID)+"/cancel", nil)
	if err != nil {
		return
	}
	d.addAuth(request)
	response, err := d.client.Do(request)
	if err == nil && response != nil {
		response.Body.Close()
	}
}

func (d *HTTPDispatcher) cleanupJob(jobID string) error {
	if jobID == "" || d.baseURL == "" || d.client == nil {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, d.baseURL+"/api/v1/agent/jobs/"+url.PathEscape(jobID)+"/cleanup", nil)
	if err != nil {
		return err
	}
	d.addAuth(request)
	response, err := d.client.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode >= http.StatusBadRequest {
		return fmt.Errorf("cleanup returned status %d", response.StatusCode)
	}
	return nil
}
