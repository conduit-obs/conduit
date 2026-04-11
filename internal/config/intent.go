package config

import (
	"errors"
	"fmt"
)

// ReceiverIntent defines a telemetry receiver configuration intent.
type ReceiverIntent struct {
	Type     string         `json:"type"`     // e.g. "otlp", "prometheus", "filelog"
	Protocol string         `json:"protocol"` // e.g. "grpc", "http"
	Endpoint string         `json:"endpoint,omitempty"`
	Settings map[string]any `json:"settings,omitempty"`
}

// ProcessorIntent defines a telemetry processor configuration intent.
type ProcessorIntent struct {
	Type     string         `json:"type"` // e.g. "batch", "filter", "attributes"
	Settings map[string]any `json:"settings,omitempty"`
}

// ExporterIntent defines a telemetry exporter configuration intent.
type ExporterIntent struct {
	Type     string         `json:"type"` // e.g. "otlp", "prometheus", "debug"
	Endpoint string         `json:"endpoint,omitempty"`
	Headers  map[string]string `json:"headers,omitempty"`
	Settings map[string]any `json:"settings,omitempty"`
}

// PipelineIntent defines a complete telemetry pipeline configuration intent.
type PipelineIntent struct {
	Name       string            `json:"name"`
	Signal     string            `json:"signal"` // "traces", "metrics", "logs"
	Receivers  []ReceiverIntent  `json:"receivers"`
	Processors []ProcessorIntent `json:"processors,omitempty"`
	Exporters  []ExporterIntent  `json:"exporters"`
}

// Intent is the top-level intent document.
type Intent struct {
	Version    string           `json:"version"`
	BaseIntent string           `json:"base_intent,omitempty"` // name of base intent to inherit from
	Pipelines  []PipelineIntent `json:"pipelines"`
}

// MergeBase merges a base intent's pipelines into this intent.
// Base pipelines are added first; this intent's pipelines with the same name override.
func (i *Intent) MergeBase(base *Intent) {
	if base == nil {
		return
	}

	existing := make(map[string]bool)
	for _, p := range i.Pipelines {
		existing[p.Name] = true
	}

	// Prepend base pipelines that aren't overridden
	var merged []PipelineIntent
	for _, p := range base.Pipelines {
		if !existing[p.Name] {
			merged = append(merged, p)
		}
	}
	merged = append(merged, i.Pipelines...)
	i.Pipelines = merged
}

// Validate checks the intent document for errors.
func (i *Intent) Validate() error {
	if i.Version == "" {
		return errors.New("version is required")
	}
	if len(i.Pipelines) == 0 {
		return errors.New("at least one pipeline is required")
	}

	pipelineNames := make(map[string]bool)
	for idx, p := range i.Pipelines {
		if p.Name == "" {
			return fmt.Errorf("pipeline[%d]: name is required", idx)
		}
		if pipelineNames[p.Name] {
			return fmt.Errorf("pipeline[%d]: duplicate name %q", idx, p.Name)
		}
		pipelineNames[p.Name] = true

		switch p.Signal {
		case "traces", "metrics", "logs":
		default:
			return fmt.Errorf("pipeline[%d]: signal must be traces, metrics, or logs; got %q", idx, p.Signal)
		}

		if len(p.Receivers) == 0 {
			return fmt.Errorf("pipeline[%d]: at least one receiver is required", idx)
		}
		if len(p.Exporters) == 0 {
			return fmt.Errorf("pipeline[%d]: at least one exporter is required", idx)
		}

		for j, r := range p.Receivers {
			if r.Type == "" {
				return fmt.Errorf("pipeline[%d].receivers[%d]: type is required", idx, j)
			}
		}
		for j, e := range p.Exporters {
			if e.Type == "" {
				return fmt.Errorf("pipeline[%d].exporters[%d]: type is required", idx, j)
			}
		}
	}

	return nil
}
