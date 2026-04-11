package mcp

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
)

// Tool represents an MCP tool definition.
type Tool struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	InputSchema map[string]any `json:"inputSchema"`
}

// Request represents an incoming MCP JSON-RPC request.
type Request struct {
	JSONRPC string         `json:"jsonrpc"`
	ID      any            `json:"id"`
	Method  string         `json:"method"`
	Params  map[string]any `json:"params,omitempty"`
}

// Response represents an outgoing MCP JSON-RPC response.
type Response struct {
	JSONRPC string `json:"jsonrpc"`
	ID      any    `json:"id"`
	Result  any    `json:"result,omitempty"`
	Error   any    `json:"error,omitempty"`
}

// ToolHandler handles a tool invocation.
type ToolHandler func(args map[string]any) (any, error)

// Server implements an MCP server over stdio.
type Server struct {
	tools    []Tool
	handlers map[string]ToolHandler
	info     map[string]any
}

// NewServer creates a new MCP server.
func NewServer(name, version string) *Server {
	return &Server{
		handlers: make(map[string]ToolHandler),
		info: map[string]any{
			"name":    name,
			"version": version,
		},
	}
}

// RegisterTool registers a tool with its handler.
func (s *Server) RegisterTool(name, description string, schema map[string]any, handler ToolHandler) {
	s.tools = append(s.tools, Tool{
		Name:        name,
		Description: description,
		InputSchema: schema,
	})
	s.handlers[name] = handler
}

// Serve runs the MCP server on stdin/stdout.
func (s *Server) Serve() error {
	reader := bufio.NewReader(os.Stdin)
	encoder := json.NewEncoder(os.Stdout)

	for {
		line, err := reader.ReadBytes('\n')
		if err != nil {
			if err == io.EOF {
				return nil
			}
			return err
		}

		var req Request
		if err := json.Unmarshal(line, &req); err != nil {
			continue
		}

		resp := s.handleRequest(req)
		encoder.Encode(resp)
	}
}

func (s *Server) handleRequest(req Request) Response {
	switch req.Method {
	case "initialize":
		return Response{
			JSONRPC: "2.0",
			ID:      req.ID,
			Result: map[string]any{
				"protocolVersion": "2024-11-05",
				"capabilities":   map[string]any{"tools": map[string]any{}},
				"serverInfo":     s.info,
			},
		}

	case "tools/list":
		return Response{
			JSONRPC: "2.0",
			ID:      req.ID,
			Result:  map[string]any{"tools": s.tools},
		}

	case "tools/call":
		toolName, _ := req.Params["name"].(string)
		args, _ := req.Params["arguments"].(map[string]any)

		handler, ok := s.handlers[toolName]
		if !ok {
			return Response{
				JSONRPC: "2.0",
				ID:      req.ID,
				Error:   map[string]any{"code": -32601, "message": fmt.Sprintf("tool not found: %s", toolName)},
			}
		}

		result, err := handler(args)
		if err != nil {
			return Response{
				JSONRPC: "2.0",
				ID:      req.ID,
				Result: map[string]any{
					"content": []map[string]any{{"type": "text", "text": fmt.Sprintf("Error: %s", err.Error())}},
					"isError": true,
				},
			}
		}

		text, _ := json.MarshalIndent(result, "", "  ")
		return Response{
			JSONRPC: "2.0",
			ID:      req.ID,
			Result: map[string]any{
				"content": []map[string]any{{"type": "text", "text": string(text)}},
			},
		}

	default:
		return Response{
			JSONRPC: "2.0",
			ID:      req.ID,
			Error:   map[string]any{"code": -32601, "message": "method not found"},
		}
	}
}

// RegisterConduitTools registers all standard Conduit MCP tools.
func (s *Server) RegisterConduitTools(baseURL, token string) {
	objectSchema := map[string]any{"type": "object", "properties": map[string]any{}}

	s.RegisterTool("list_agents", "List all agents in the fleet", objectSchema, func(args map[string]any) (any, error) {
		return apiCall(baseURL, token, "GET", "/api/v1/agents", nil)
	})

	s.RegisterTool("get_agent", "Get agent details by ID", map[string]any{
		"type": "object",
		"properties": map[string]any{"id": map[string]any{"type": "string", "description": "Agent ID"}},
		"required": []string{"id"},
	}, func(args map[string]any) (any, error) {
		return map[string]any{"note": "Use list_agents and filter by ID"}, nil
	})

	s.RegisterTool("list_fleets", "List all fleets", objectSchema, func(args map[string]any) (any, error) {
		return apiCall(baseURL, token, "GET", "/api/v1/fleets", nil)
	})

	s.RegisterTool("list_templates", "List available pipeline templates", objectSchema, func(args map[string]any) (any, error) {
		return apiCall(baseURL, token, "GET", "/api/v1/templates", nil)
	})

	s.RegisterTool("get_rollout_status", "Get rollout status", map[string]any{
		"type": "object",
		"properties": map[string]any{"id": map[string]any{"type": "string", "description": "Rollout ID"}},
		"required": []string{"id"},
	}, func(args map[string]any) (any, error) {
		id, _ := args["id"].(string)
		return apiCall(baseURL, token, "GET", "/api/v1/rollouts/"+id, nil)
	})

	s.RegisterTool("compile_config", "Compile an intent document to OTel YAML", map[string]any{
		"type": "object",
		"properties": map[string]any{"intent": map[string]any{"type": "object", "description": "Intent document"}},
		"required": []string{"intent"},
	}, func(args map[string]any) (any, error) {
		return apiCall(baseURL, token, "POST", "/api/v1/config/compile", args["intent"])
	})

	s.RegisterTool("create_rollout", "Create a new rollout", map[string]any{
		"type": "object",
		"properties": map[string]any{
			"fleet_id":  map[string]any{"type": "string"},
			"intent_id": map[string]any{"type": "string"},
			"strategy":  map[string]any{"type": "string", "enum": []string{"all-at-once", "canary"}},
		},
		"required": []string{"fleet_id", "intent_id"},
	}, func(args map[string]any) (any, error) {
		return apiCall(baseURL, token, "POST", "/api/v1/rollouts", args)
	})
}

func apiCall(baseURL, token, method, path string, body any) (any, error) {
	// This is a simplified client — in production, use the SDK
	return map[string]any{
		"endpoint": method + " " + baseURL + path,
		"note":     "MCP tool executed (connect to running Conduit for real data)",
	}, nil
}
