package template

func ptr[T any](v T) *T { return &v }

// BuiltinTemplates returns all built-in pipeline templates.
func BuiltinTemplates() []PipelineTemplate {
	return []PipelineTemplate{
		otlpIngestionTemplate(),
		k8sClusterTelemetryTemplate(),
		redactPIITemplate(),
		dropSensitiveAttrsTemplate(),
		traceSamplingTemplate(),
		logRoutingTemplate(),
	}
}

func otlpIngestionTemplate() PipelineTemplate {
	return PipelineTemplate{
		Metadata: TemplateMetadata{
			Name:        "otlp-ingestion",
			Version:     "1.0.0",
			Description: "Standard OTLP ingestion pipeline with batch processing and OTLP export",
			Category:    "ingestion",
			Labels:      map[string]string{"signal": "all"},
		},
		Parameters: []ParameterDef{
			{Name: "grpc_port", Type: "number", Description: "OTLP gRPC receiver port", Default: float64(4317), Validation: &ParameterValidation{Minimum: ptr(1.0), Maximum: ptr(65535.0)}},
			{Name: "http_port", Type: "number", Description: "OTLP HTTP receiver port", Default: float64(4318), Validation: &ParameterValidation{Minimum: ptr(1.0), Maximum: ptr(65535.0)}},
			{Name: "batch_size", Type: "number", Description: "Batch processor max batch size", Default: float64(1000), Validation: &ParameterValidation{Minimum: ptr(1.0), Maximum: ptr(100000.0)}},
			{Name: "batch_timeout_ms", Type: "number", Description: "Batch processor flush interval in ms", Default: float64(5000)},
			{Name: "export_endpoint", Type: "string", Description: "OTLP exporter target endpoint", Required: true, Validation: &ParameterValidation{Pattern: `^https?://`}, ErrorMessage: "Must be a valid HTTP/HTTPS URL"},
			{Name: "export_timeout_ms", Type: "number", Description: "Export timeout in ms", Default: float64(10000)},
		},
		Intent: TemplateIntent{
			Signal: "all",
			Receivers: []TemplateReceiverSpec{
				{Type: "otlp", Config: map[string]any{"protocols": map[string]any{"grpc": map[string]any{"endpoint": "0.0.0.0:{{.grpc_port}}"}, "http": map[string]any{"endpoint": "0.0.0.0:{{.http_port}}"}}}},
			},
			Processors: []TemplateProcessorSpec{
				{Type: "batch", Config: map[string]any{"send_batch_size": "{{.batch_size}}", "timeout": "{{.batch_timeout_ms}}ms"}},
			},
			Exporters: []TemplateExporterSpec{
				{Type: "otlp", Config: map[string]any{"endpoint": "{{.export_endpoint}}", "timeout": "{{.export_timeout_ms}}ms"}},
			},
		},
	}
}

func k8sClusterTelemetryTemplate() PipelineTemplate {
	return PipelineTemplate{
		Metadata: TemplateMetadata{
			Name:        "k8s-cluster-telemetry",
			Version:     "1.0.0",
			Description: "Comprehensive Kubernetes cluster telemetry collection (events, metrics, logs)",
			Category:    "kubernetes",
			Labels:      map[string]string{"signal": "all", "platform": "kubernetes"},
		},
		Parameters: []ParameterDef{
			{Name: "collect_events", Type: "boolean", Description: "Collect Kubernetes events", Default: true},
			{Name: "collect_metrics", Type: "boolean", Description: "Collect pod and node metrics", Default: true},
			{Name: "collect_logs", Type: "boolean", Description: "Collect container logs", Default: true},
			{Name: "enrich_metadata", Type: "boolean", Description: "Enrich with K8s metadata (labels, annotations)", Default: true},
			{Name: "export_endpoint", Type: "string", Description: "OTLP exporter target endpoint", Required: true},
			{Name: "namespace_filter", Type: "string", Description: "Filter by namespace (empty = all)", Default: ""},
		},
		Intent: TemplateIntent{
			Signal: "all",
			Receivers: []TemplateReceiverSpec{
				{Type: "k8s_events", Config: map[string]any{"namespaces": "{{.namespace_filter}}"}},
				{Type: "kubeletstats", Config: map[string]any{"collection_interval": "30s"}},
				{Type: "filelog", Config: map[string]any{"include": []string{"/var/log/containers/*.log"}}},
			},
			Processors: []TemplateProcessorSpec{
				{Type: "k8sattributes", Config: map[string]any{"extract": map[string]any{"metadata": []string{"k8s.pod.name", "k8s.namespace.name", "k8s.node.name"}}}},
				{Type: "batch", Config: map[string]any{"send_batch_size": 1000}},
			},
			Exporters: []TemplateExporterSpec{
				{Type: "otlp", Config: map[string]any{"endpoint": "{{.export_endpoint}}"}},
			},
		},
	}
}

func redactPIITemplate() PipelineTemplate {
	return PipelineTemplate{
		Metadata: TemplateMetadata{
			Name:        "redact-pii",
			Version:     "1.0.0",
			Description: "Automatically redact personally identifiable information (PII) from telemetry",
			Category:    "security",
			Labels:      map[string]string{"compliance": "gdpr", "signal": "all"},
		},
		Parameters: []ParameterDef{
			{Name: "redact_patterns", Type: "array", Description: "PII types to redact", Default: []any{"email", "phone", "credit_card", "ssn"}},
			{Name: "redaction_char", Type: "string", Description: "Character used for redaction", Default: "*"},
			{Name: "scope", Type: "string", Description: "Telemetry scope to apply redaction", Default: "all", Validation: &ParameterValidation{Enum: []string{"spans", "logs", "metrics", "all"}}},
		},
		Intent: TemplateIntent{
			Signal: "all",
			Receivers: []TemplateReceiverSpec{
				{Type: "otlp", Config: map[string]any{"protocols": map[string]any{"grpc": map[string]any{"endpoint": "0.0.0.0:4317"}}}},
			},
			Processors: []TemplateProcessorSpec{
				{Type: "transform", Config: map[string]any{
					"error_mode": "ignore",
					"log_statements": []map[string]any{
						{"context": "log", "statements": []string{
							`replace_pattern(body, "\\b[A-Za-z0-9._%+-]+@[A-Za-z0-9.-]+\\.[A-Z|a-z]{2,}\\b", "[REDACTED_EMAIL]")`,
							`replace_pattern(body, "\\b\\d{3}[-.]?\\d{3}[-.]?\\d{4}\\b", "[REDACTED_PHONE]")`,
							`replace_pattern(body, "\\b(?:\\d{4}[- ]?){3}\\d{4}\\b", "[REDACTED_CC]")`,
							`replace_pattern(body, "\\b\\d{3}-\\d{2}-\\d{4}\\b", "[REDACTED_SSN]")`,
						}},
					},
				}},
			},
			Exporters: []TemplateExporterSpec{
				{Type: "otlp", Config: map[string]any{"endpoint": "localhost:4317"}},
			},
		},
	}
}

func dropSensitiveAttrsTemplate() PipelineTemplate {
	return PipelineTemplate{
		Metadata: TemplateMetadata{
			Name:        "drop-sensitive-attrs",
			Version:     "1.0.0",
			Description: "Drop sensitive attributes (tokens, secrets, keys, passwords) from telemetry",
			Category:    "security",
			Labels:      map[string]string{"compliance": "security", "signal": "all"},
		},
		Parameters: []ParameterDef{
			{Name: "patterns", Type: "array", Description: "Attribute name patterns to drop", Default: []any{"*token*", "*secret*", "*key*", "*password*", "*api_key*", "*auth*"}},
			{Name: "scope", Type: "string", Description: "Scope: spans, logs, metrics, or all", Default: "all", Validation: &ParameterValidation{Enum: []string{"spans", "logs", "metrics", "all"}}},
			{Name: "case_sensitive", Type: "boolean", Description: "Case-sensitive pattern matching", Default: false},
		},
		Intent: TemplateIntent{
			Signal: "all",
			Receivers: []TemplateReceiverSpec{
				{Type: "otlp", Config: map[string]any{"protocols": map[string]any{"grpc": map[string]any{"endpoint": "0.0.0.0:4317"}}}},
			},
			Processors: []TemplateProcessorSpec{
				{Type: "attributes", Config: map[string]any{
					"actions": []map[string]any{
						{"key_regex": "(?i).*(token|secret|key|password|api_key|auth).*", "action": "delete"},
					},
				}},
			},
			Exporters: []TemplateExporterSpec{
				{Type: "otlp", Config: map[string]any{"endpoint": "localhost:4317"}},
			},
		},
	}
}

func traceSamplingTemplate() PipelineTemplate {
	return PipelineTemplate{
		Metadata: TemplateMetadata{
			Name:        "trace-sampling",
			Version:     "1.0.0",
			Description: "Intelligent trace sampling with probability, tail-based, and error-biased strategies",
			Category:    "performance",
			Labels:      map[string]string{"signal": "traces"},
		},
		Parameters: []ParameterDef{
			{Name: "sampling_type", Type: "string", Description: "Sampling strategy", Default: "probability", Validation: &ParameterValidation{Enum: []string{"probability", "tail-based", "error-biased"}}},
			{Name: "sample_rate", Type: "number", Description: "Sampling rate (0.0-1.0)", Default: 0.1, Validation: &ParameterValidation{Minimum: ptr(0.0), Maximum: ptr(1.0)}},
			{Name: "error_sample_rate", Type: "number", Description: "Sample rate for error traces", Default: 1.0, Validation: &ParameterValidation{Minimum: ptr(0.0), Maximum: ptr(1.0)}},
			{Name: "duration_threshold_ms", Type: "number", Description: "Duration threshold for tail-based sampling (ms)", Default: float64(1000), Validation: &ParameterValidation{Minimum: ptr(0.0)}},
		},
		Intent: TemplateIntent{
			Signal: "traces",
			Receivers: []TemplateReceiverSpec{
				{Type: "otlp", Config: map[string]any{"protocols": map[string]any{"grpc": map[string]any{"endpoint": "0.0.0.0:4317"}}}},
			},
			Processors: []TemplateProcessorSpec{
				{Type: "probabilistic_sampler", Config: map[string]any{"sampling_percentage": "{{.sample_rate}}"}},
			},
			Exporters: []TemplateExporterSpec{
				{Type: "otlp", Config: map[string]any{"endpoint": "localhost:4317"}},
			},
		},
	}
}

func logRoutingTemplate() PipelineTemplate {
	return PipelineTemplate{
		Metadata: TemplateMetadata{
			Name:        "log-routing",
			Version:     "1.0.0",
			Description: "Route logs to multiple destinations based on severity, source, and content",
			Category:    "routing",
			Labels:      map[string]string{"signal": "logs"},
		},
		Parameters: []ParameterDef{
			{Name: "routes", Type: "array", Description: "Routing rules: [{name, condition, exporter}]", Default: []any{
				map[string]any{"name": "errors", "condition": `severity_number >= 17`, "exporter": "error-backend"},
				map[string]any{"name": "default", "condition": "true", "exporter": "default-backend"},
			}},
			{Name: "default_exporter", Type: "string", Description: "Default exporter for unmatched logs", Default: "debug"},
		},
		Intent: TemplateIntent{
			Signal: "logs",
			Receivers: []TemplateReceiverSpec{
				{Type: "otlp", Config: map[string]any{"protocols": map[string]any{"grpc": map[string]any{"endpoint": "0.0.0.0:4317"}}}},
			},
			Processors: []TemplateProcessorSpec{
				{Type: "routing", Config: map[string]any{
					"default_exporters": []string{"{{.default_exporter}}"},
					"table":            "{{.routes}}",
				}},
			},
			Exporters: []TemplateExporterSpec{
				{Type: "otlp", Config: map[string]any{"endpoint": "localhost:4317"}},
				{Type: "debug", Config: map[string]any{}},
			},
		},
	}
}
