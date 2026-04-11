package config

import (
	"strings"
	"testing"
)

func TestCompiler_BasicPipeline(t *testing.T) {
	compiler := NewCompiler()

	intent := &Intent{
		Version: "1.0",
		Pipelines: []PipelineIntent{
			{
				Name:   "default",
				Signal: "traces",
				Receivers: []ReceiverIntent{
					{Type: "otlp", Protocol: "grpc", Endpoint: "0.0.0.0:4317"},
				},
				Processors: []ProcessorIntent{
					{Type: "batch", Settings: map[string]any{"timeout": "5s"}},
				},
				Exporters: []ExporterIntent{
					{Type: "otlp", Endpoint: "tempo:4317"},
				},
			},
		},
	}

	yamlOut, err := compiler.Compile(intent)
	if err != nil {
		t.Fatal(err)
	}

	if !IsValidYAML(yamlOut) {
		t.Error("output is not valid YAML")
	}

	// Check key parts are present
	if !strings.Contains(yamlOut, "receivers:") {
		t.Error("missing receivers section")
	}
	if !strings.Contains(yamlOut, "exporters:") {
		t.Error("missing exporters section")
	}
	if !strings.Contains(yamlOut, "service:") {
		t.Error("missing service section")
	}
	if !strings.Contains(yamlOut, "traces/default") {
		t.Error("missing pipeline key traces/default")
	}
}

func TestCompiler_MultiplePipelines(t *testing.T) {
	compiler := NewCompiler()

	intent := &Intent{
		Version: "1.0",
		Pipelines: []PipelineIntent{
			{
				Name:   "traces-pipeline",
				Signal: "traces",
				Receivers: []ReceiverIntent{
					{Type: "otlp", Protocol: "grpc"},
				},
				Exporters: []ExporterIntent{
					{Type: "otlp", Endpoint: "tempo:4317"},
				},
			},
			{
				Name:   "metrics-pipeline",
				Signal: "metrics",
				Receivers: []ReceiverIntent{
					{Type: "prometheus", Endpoint: "0.0.0.0:9090"},
				},
				Exporters: []ExporterIntent{
					{Type: "prometheus", Endpoint: "0.0.0.0:8889"},
				},
			},
		},
	}

	yamlOut, err := compiler.Compile(intent)
	if err != nil {
		t.Fatal(err)
	}

	if !IsValidYAML(yamlOut) {
		t.Error("output is not valid YAML")
	}

	if !strings.Contains(yamlOut, "traces/traces-pipeline") {
		t.Error("missing traces pipeline")
	}
	if !strings.Contains(yamlOut, "metrics/metrics-pipeline") {
		t.Error("missing metrics pipeline")
	}
}

func TestCompiler_ValidationErrors(t *testing.T) {
	compiler := NewCompiler()

	tests := []struct {
		name   string
		intent Intent
	}{
		{"no version", Intent{Pipelines: []PipelineIntent{{Name: "x", Signal: "traces", Receivers: []ReceiverIntent{{Type: "otlp"}}, Exporters: []ExporterIntent{{Type: "otlp"}}}}}},
		{"no pipelines", Intent{Version: "1.0"}},
		{"bad signal", Intent{Version: "1.0", Pipelines: []PipelineIntent{{Name: "x", Signal: "invalid", Receivers: []ReceiverIntent{{Type: "otlp"}}, Exporters: []ExporterIntent{{Type: "otlp"}}}}}},
		{"no receivers", Intent{Version: "1.0", Pipelines: []PipelineIntent{{Name: "x", Signal: "traces", Exporters: []ExporterIntent{{Type: "otlp"}}}}}},
		{"no exporters", Intent{Version: "1.0", Pipelines: []PipelineIntent{{Name: "x", Signal: "traces", Receivers: []ReceiverIntent{{Type: "otlp"}}}}}},
		{"duplicate name", Intent{Version: "1.0", Pipelines: []PipelineIntent{
			{Name: "x", Signal: "traces", Receivers: []ReceiverIntent{{Type: "otlp"}}, Exporters: []ExporterIntent{{Type: "otlp"}}},
			{Name: "x", Signal: "metrics", Receivers: []ReceiverIntent{{Type: "otlp"}}, Exporters: []ExporterIntent{{Type: "otlp"}}},
		}}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := compiler.Compile(&tt.intent)
			if err == nil {
				t.Error("expected validation error")
			}
		})
	}
}

func TestIsValidYAML(t *testing.T) {
	if !IsValidYAML("key: value") {
		t.Error("should be valid")
	}
	if !IsValidYAML("- a\n- b\n") {
		t.Error("should be valid")
	}
	// Even malformed YAML is often parsed leniently by yaml.v3
	// but we can check empty is valid
	if !IsValidYAML("") {
		t.Error("empty should be valid")
	}
}
