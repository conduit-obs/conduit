package config

import (
	"fmt"

	"gopkg.in/yaml.v3"
)

// Compiler compiles Intent documents into OpenTelemetry Collector YAML configuration.
type Compiler struct{}

// NewCompiler creates a new intent compiler.
func NewCompiler() *Compiler {
	return &Compiler{}
}

// otelConfig mirrors the OTel Collector config structure.
type otelConfig struct {
	Receivers  map[string]any            `yaml:"receivers,omitempty"`
	Processors map[string]any            `yaml:"processors,omitempty"`
	Exporters  map[string]any            `yaml:"exporters,omitempty"`
	Service    otelService               `yaml:"service"`
}

type otelService struct {
	Pipelines map[string]otelPipeline `yaml:"pipelines"`
}

type otelPipeline struct {
	Receivers  []string `yaml:"receivers"`
	Processors []string `yaml:"processors,omitempty"`
	Exporters  []string `yaml:"exporters"`
}

// Compile converts an Intent into OTel Collector YAML.
func (c *Compiler) Compile(intent *Intent) (string, error) {
	if err := intent.Validate(); err != nil {
		return "", fmt.Errorf("validation failed: %w", err)
	}

	cfg := otelConfig{
		Receivers:  make(map[string]any),
		Processors: make(map[string]any),
		Exporters:  make(map[string]any),
		Service: otelService{
			Pipelines: make(map[string]otelPipeline),
		},
	}

	for _, pipeline := range intent.Pipelines {
		var receiverNames []string
		var processorNames []string
		var exporterNames []string

		for _, r := range pipeline.Receivers {
			name := receiverKey(r, pipeline.Name)
			receiverCfg := buildReceiverConfig(r)
			cfg.Receivers[name] = receiverCfg
			receiverNames = append(receiverNames, name)
		}

		for _, p := range pipeline.Processors {
			name := processorKey(p, pipeline.Name)
			processorCfg := buildProcessorConfig(p)
			cfg.Processors[name] = processorCfg
			processorNames = append(processorNames, name)
		}

		for _, e := range pipeline.Exporters {
			name := exporterKey(e, pipeline.Name)
			exporterCfg := buildExporterConfig(e)
			cfg.Exporters[name] = exporterCfg
			exporterNames = append(exporterNames, name)
		}

		pipelineKey := fmt.Sprintf("%s/%s", pipeline.Signal, pipeline.Name)
		cfg.Service.Pipelines[pipelineKey] = otelPipeline{
			Receivers:  receiverNames,
			Processors: processorNames,
			Exporters:  exporterNames,
		}
	}

	out, err := yaml.Marshal(cfg)
	if err != nil {
		return "", fmt.Errorf("marshaling yaml: %w", err)
	}

	return string(out), nil
}

func receiverKey(r ReceiverIntent, pipelineName string) string {
	if r.Protocol != "" {
		return fmt.Sprintf("%s/%s_%s", r.Type, pipelineName, r.Protocol)
	}
	return fmt.Sprintf("%s/%s", r.Type, pipelineName)
}

func processorKey(p ProcessorIntent, pipelineName string) string {
	return fmt.Sprintf("%s/%s", p.Type, pipelineName)
}

func exporterKey(e ExporterIntent, pipelineName string) string {
	return fmt.Sprintf("%s/%s", e.Type, pipelineName)
}

func buildReceiverConfig(r ReceiverIntent) map[string]any {
	cfg := make(map[string]any)
	if r.Protocol != "" {
		protocols := map[string]any{}
		protoCfg := map[string]any{}
		if r.Endpoint != "" {
			protoCfg["endpoint"] = r.Endpoint
		}
		protocols[r.Protocol] = protoCfg
		cfg["protocols"] = protocols
	} else if r.Endpoint != "" {
		cfg["endpoint"] = r.Endpoint
	}
	for k, v := range r.Settings {
		cfg[k] = v
	}
	return cfg
}

func buildProcessorConfig(p ProcessorIntent) map[string]any {
	cfg := make(map[string]any)
	for k, v := range p.Settings {
		cfg[k] = v
	}
	if len(cfg) == 0 {
		return nil
	}
	return cfg
}

func buildExporterConfig(e ExporterIntent) map[string]any {
	cfg := make(map[string]any)
	if e.Endpoint != "" {
		cfg["endpoint"] = e.Endpoint
	}
	if len(e.Headers) > 0 {
		cfg["headers"] = e.Headers
	}
	for k, v := range e.Settings {
		cfg[k] = v
	}
	if len(cfg) == 0 {
		return nil
	}
	return cfg
}

// IsValidYAML checks if a string is valid YAML.
func IsValidYAML(s string) bool {
	var out any
	return yaml.Unmarshal([]byte(s), &out) == nil
}

