package alerting

import (
	"bytes"
	"encoding/json"
	"net/http"
	"sync"
	"time"
)

// AlertRule defines a condition that triggers an alert.
type AlertRule struct {
	ID        string   `json:"id"`
	TenantID  string   `json:"tenant_id"`
	Name      string   `json:"name"`
	Condition string   `json:"condition"` // e.g., "agent_disconnect_count", "api_error_rate"
	Threshold float64  `json:"threshold"`
	Channels  []string `json:"channels"` // webhook URLs
	Enabled   bool     `json:"enabled"`
}

// Alert represents a triggered alert.
type Alert struct {
	RuleName  string    `json:"rule_name"`
	Condition string    `json:"condition"`
	Value     float64   `json:"value"`
	Threshold float64   `json:"threshold"`
	FiredAt   time.Time `json:"fired_at"`
}

// AlertEngine evaluates rules against metrics and fires alerts.
type AlertEngine struct {
	mu      sync.RWMutex
	rules   []AlertRule
	active  []Alert
	metrics map[string]float64
}

// NewEngine creates a new AlertEngine.
func NewEngine() *AlertEngine {
	return &AlertEngine{
		metrics: make(map[string]float64),
	}
}

// SetRules replaces the current rule set.
func (e *AlertEngine) SetRules(rules []AlertRule) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.rules = rules
}

// AddRule adds a single rule.
func (e *AlertEngine) AddRule(rule AlertRule) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.rules = append(e.rules, rule)
}

// UpdateMetric sets a metric value for evaluation.
func (e *AlertEngine) UpdateMetric(name string, value float64) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.metrics[name] = value
}

// EvaluateAll checks all rules against current metrics and fires alerts.
func (e *AlertEngine) EvaluateAll() []Alert {
	e.mu.Lock()
	defer e.mu.Unlock()

	var fired []Alert
	for _, rule := range e.rules {
		if !rule.Enabled {
			continue
		}
		val, ok := e.metrics[rule.Condition]
		if !ok {
			continue
		}
		if val > rule.Threshold {
			alert := Alert{
				RuleName:  rule.Name,
				Condition: rule.Condition,
				Value:     val,
				Threshold: rule.Threshold,
				FiredAt:   time.Now(),
			}
			fired = append(fired, alert)

			// Fire to webhook channels
			for _, url := range rule.Channels {
				go fireWebhook(url, alert)
			}
		}
	}

	e.active = fired
	return fired
}

// ActiveAlerts returns currently active alerts.
func (e *AlertEngine) ActiveAlerts() []Alert {
	e.mu.RLock()
	defer e.mu.RUnlock()
	result := make([]Alert, len(e.active))
	copy(result, e.active)
	return result
}

func fireWebhook(url string, alert Alert) {
	body, _ := json.Marshal(alert)
	http.Post(url, "application/json", bytes.NewReader(body))
}
