package mcpadapter

import (
	"context"
	"errors"
	"reflect"
	"sort"
	"testing"
	"time"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/vibepwners/hovel/internal/app/commands"
)

func TestOperatorCapabilityRoutesCoverEveryHumanCapability(t *testing.T) {
	capabilities, err := commands.HovelRegistry(commands.Runtime{}).OperatorCapabilities()
	if err != nil {
		t.Fatalf("OperatorCapabilities returned error: %v", err)
	}
	known := make(map[commands.CapabilityID]commands.OperatorCapability, len(capabilities))
	for _, capability := range capabilities {
		known[capability.ID] = capability
	}
	covered := make(map[commands.CapabilityID]bool, len(capabilities))
	advertised := make(map[string]bool)
	for _, tool := range defaultCapabilities() {
		advertised[tool] = true
	}
	for _, route := range OperatorCapabilityRoutes() {
		capability, ok := known[route.Capability]
		if !ok {
			t.Errorf("tool %s references unknown capability %s", route.Tool, route.Capability)
			continue
		}
		if capability.Risk != route.Risk {
			t.Errorf("tool %s risk = %s, capability %s risk = %s", route.Tool, route.Risk, route.Capability, capability.Risk)
		}
		if !advertised[route.Tool] {
			t.Errorf("tool %s implements %s but is not advertised", route.Tool, route.Capability)
		}
		covered[route.Capability] = true
	}
	for id := range known {
		if !covered[id] {
			t.Errorf("human capability %s has no typed MCP route", id)
		}
	}
}

func TestRegisteredToolsMatchAdvertisedCapabilities(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	serverTransport, clientTransport := mcpsdk.NewInMemoryTransports()
	done := make(chan error, 1)
	go func() { done <- (&Server{}).MCPServer().Run(ctx, serverTransport) }()

	client := mcpsdk.NewClient(&mcpsdk.Implementation{Name: "parity-contract", Version: "v1"}, nil)
	session, err := client.Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatalf("connect MCP client: %v", err)
	}
	tools, err := session.ListTools(ctx, &mcpsdk.ListToolsParams{})
	if err != nil {
		t.Fatalf("list MCP tools: %v", err)
	}
	registered := make([]string, 0, len(tools.Tools))
	for _, tool := range tools.Tools {
		registered = append(registered, tool.Name)
		if tool.InputSchema == nil || tool.Annotations == nil {
			t.Errorf("tool %s is missing its schema or annotations", tool.Name)
		}
	}
	advertised := defaultCapabilities()
	sort.Strings(registered)
	sort.Strings(advertised)
	if !reflect.DeepEqual(registered, advertised) {
		t.Fatalf("registered tools = %#v, advertised capabilities = %#v", registered, advertised)
	}

	if err := session.Close(); err != nil {
		t.Fatalf("close MCP client: %v", err)
	}
	cancel()
	if err := <-done; err != nil && !errors.Is(err, context.Canceled) {
		t.Fatalf("MCP server returned error: %v", err)
	}
}

func TestGeneratedCapabilityToolBuildsValidatedCommandArguments(t *testing.T) {
	definition, ok := commands.HovelRegistry(commands.Runtime{}).Find("target", "set", "add")
	if !ok {
		t.Fatal("target set add command is not registered")
	}
	args, err := commandCapabilityArgs(definition, map[string]any{
		"name":   "routers",
		"target": "router-1",
		"config": "/tmp/hovel.yaml",
		"debug":  true,
	})
	if err != nil {
		t.Fatalf("commandCapabilityArgs returned error: %v", err)
	}
	want := []string{"target", "set", "add", "routers", "router-1", "--config", "/tmp/hovel.yaml", "--debug"}
	if !reflect.DeepEqual(args, want) {
		t.Fatalf("args = %#v, want %#v", args, want)
	}
	if _, err := commandCapabilityArgs(definition, map[string]any{"name": 7}); err == nil {
		t.Fatal("commandCapabilityArgs accepted a non-string positional")
	}
}
