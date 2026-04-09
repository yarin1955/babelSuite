package main

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHealthServiceReadinessAggregatesSubsystems(t *testing.T) {
	service := newHealthService("mongo", []subsystemProbe{
		{
			Name:     "database",
			Required: true,
			Check: func(context.Context) (probeCheck, error) {
				return probeCheck{Status: probeReady, Detail: "connected"}, nil
			},
		},
		{
			Name:     "cache",
			Required: false,
			Check: func(context.Context) (probeCheck, error) {
				return probeCheck{Status: probeDisabled, Detail: "not configured"}, nil
			},
		},
	})

	request := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	response := httptest.NewRecorder()
	service.readiness(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", response.Code)
	}
}

func TestHealthServiceReadinessFailsForRequiredSubsystem(t *testing.T) {
	service := newHealthService("mongo", []subsystemProbe{
		{
			Name:     "database",
			Required: true,
			Check: func(context.Context) (probeCheck, error) {
				return probeCheck{}, errors.New("database unavailable")
			},
		},
	})

	request := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	response := httptest.NewRecorder()
	service.readiness(response, request)

	if response.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d", response.Code)
	}
}
