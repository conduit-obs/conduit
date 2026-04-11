package slo

import (
	"sync"
	"time"
)

// SLO represents a Service Level Objective.
type SLO struct {
	Name      string  `json:"name"`
	Target    float64 `json:"target"`    // e.g., 99.9
	Window    string  `json:"window"`    // e.g., "30d"
	Indicator string  `json:"indicator"` // e.g., "api_availability"
}

// SLOStatus represents the current status of an SLO.
type SLOStatus struct {
	Name           string  `json:"name"`
	Target         float64 `json:"target"`
	Current        float64 `json:"current"`
	ErrorBudget    float64 `json:"error_budget_remaining"` // percentage
	Window         string  `json:"window"`
	TotalRequests  int64   `json:"total_requests"`
	FailedRequests int64   `json:"failed_requests"`
}

// Tracker tracks SLO metrics.
type Tracker struct {
	mu            sync.RWMutex
	slos          []SLO
	totalRequests int64
	failedRequests int64
	windowStart   time.Time
}

// NewTracker creates a new SLO Tracker with default SLOs.
func NewTracker() *Tracker {
	return &Tracker{
		slos: []SLO{
			{Name: "api-availability", Target: 99.9, Window: "30d", Indicator: "api_availability"},
			{Name: "config-delivery", Target: 99.5, Window: "30d", Indicator: "config_delivery_success"},
			{Name: "api-latency-p99", Target: 95.0, Window: "30d", Indicator: "api_latency_under_500ms"},
		},
		windowStart: time.Now(),
	}
}

// RecordRequest records a request (success or failure).
func (t *Tracker) RecordRequest(success bool) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.totalRequests++
	if !success {
		t.failedRequests++
	}
}

// Status returns the current SLO status for all objectives.
func (t *Tracker) Status() []SLOStatus {
	t.mu.RLock()
	defer t.mu.RUnlock()

	var statuses []SLOStatus
	for _, slo := range t.slos {
		current := 100.0
		if t.totalRequests > 0 {
			current = float64(t.totalRequests-t.failedRequests) / float64(t.totalRequests) * 100
		}
		errorBudget := current - slo.Target
		if errorBudget < 0 {
			errorBudget = 0
		}
		statuses = append(statuses, SLOStatus{
			Name:           slo.Name,
			Target:         slo.Target,
			Current:        current,
			ErrorBudget:    errorBudget,
			Window:         slo.Window,
			TotalRequests:  t.totalRequests,
			FailedRequests: t.failedRequests,
		})
	}
	return statuses
}
