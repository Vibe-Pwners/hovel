package mcpadapter

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"sort"
	"testing"
	"time"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/vibepwners/hovel/internal/adapters/daemonrpc"
	"github.com/vibepwners/hovel/internal/app/commands"
)

type semanticContractDaemon struct {
	Daemon
	entity daemonrpc.OperatorEntity
}

func (d *semanticContractDaemon) HeartbeatEntity(_ context.Context, req daemonrpc.HeartbeatEntityRequest) (daemonrpc.EntityResponse, error) {
	if req.Operation != nil {
		d.entity.Operation = *req.Operation
	}
	if req.ActiveChain != nil {
		d.entity.ActiveChain = *req.ActiveChain
	}
	return daemonrpc.EntityResponse{Entity: d.entity}, nil
}

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

func TestEveryOperatorCapabilityHasEquivalentHumanAndAgentInvocation(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	invocations := make(chan commandRunInput, 1)
	entity := daemonrpc.OperatorEntity{ID: "semantic-parity-contract", Kind: "agent", DisplayName: "Semantic parity contract"}
	server := &Server{
		daemon: &semanticContractDaemon{entity: entity},
		entity: entity,
		commandRunner: func(_ context.Context, input commandRunInput) (commandRunOutput, error) {
			invocations <- input
			return commandRunOutput{Args: append([]string(nil), input.Args...), ExitCode: 0, OK: true}, nil
		},
	}
	serverTransport, clientTransport := mcpsdk.NewInMemoryTransports()
	done := make(chan error, 1)
	go func() { done <- server.MCPServer().Run(ctx, serverTransport) }()

	client := mcpsdk.NewClient(&mcpsdk.Implementation{Name: "semantic-parity-contract", Version: "v1"}, nil)
	session, err := client.Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatalf("connect MCP client: %v", err)
	}

	registry := commands.HovelRegistry(commands.Runtime{})
	dedicated := map[commands.CapabilityID]bool{}
	for _, route := range dedicatedOperatorCapabilityRoutes() {
		dedicated[route.Capability] = true
	}
	for _, definition := range registry.Definitions() {
		definition := definition
		id := commands.CapabilityIDForPath(definition.Path)
		t.Run(string(id), func(t *testing.T) {
			input, humanArgs := semanticContractFixture(definition)
			tool := semanticCapabilityToolName(id, dedicated[id])
			result, err := session.CallTool(ctx, &mcpsdk.CallToolParams{Name: tool, Arguments: input})
			if err != nil {
				t.Fatalf("CallTool(%s): %v", tool, err)
			}
			if result.IsError {
				t.Fatalf("CallTool(%s) returned tool error: %#v", tool, result.Content)
			}
			select {
			case agentInvocation := <-invocations:
				if !reflect.DeepEqual(agentInvocation.Args, humanArgs) {
					t.Fatalf("agent invocation = %#v, human invocation = %#v", agentInvocation.Args, humanArgs)
				}
			case <-ctx.Done():
				t.Fatalf("waiting for %s invocation: %v", tool, ctx.Err())
			}
		})
	}

	if err := session.Close(); err != nil {
		t.Fatalf("close MCP client: %v", err)
	}
	cancel()
	if err := <-done; err != nil && !errors.Is(err, context.Canceled) {
		t.Fatalf("MCP server returned error: %v", err)
	}
}

// semanticContractFixture constructs the smallest valid human command line and
// the equivalent typed MCP input independently. Exact argv equality is an
// effects contract because both routes then enter the same command-mode
// registry, application handlers, daemon session, policy, and audit path.
func semanticContractFixture(definition commands.Definition) (map[string]any, []string) {
	input := map[string]any{}
	humanArgs := append([]string(nil), definition.Path...)
	for index, positional := range definition.Positionals {
		if !positional.Required {
			continue
		}
		value := fmt.Sprintf("contract-%s-%d", positional.Name, index)
		input[positional.Name] = value
		humanArgs = append(humanArgs, value)
	}
	for index, option := range definition.Options {
		if !option.Required {
			continue
		}
		flag := "--" + option.Name
		switch option.Kind {
		case commands.OptionBool:
			input[option.Name] = true
			humanArgs = append(humanArgs, flag)
		case commands.OptionStringList:
			value := fmt.Sprintf("contract-%s-%d", option.Name, index)
			input[option.Name] = []any{value}
			humanArgs = append(humanArgs, flag, value)
		default:
			value := fmt.Sprintf("contract-%s-%d", option.Name, index)
			input[option.Name] = value
			humanArgs = append(humanArgs, flag, value)
		}
	}
	if definition.Passthrough.Required {
		value := "contract-" + definition.Passthrough.Name
		input[definition.Passthrough.Name] = []any{value}
		humanArgs = append(humanArgs, "--", value)
	}
	return input, humanArgs
}
