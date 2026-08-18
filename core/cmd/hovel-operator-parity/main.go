package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	mcpadapter "github.com/vibepwners/hovel/internal/adapters/mcp"
	"github.com/vibepwners/hovel/internal/app/commands"
)

type parityRoute struct {
	Tool     string   `json:"tool"`
	Evidence []string `json:"evidence,omitempty"`
}

type parityCapability struct {
	ID             commands.CapabilityID   `json:"id"`
	Summary        string                  `json:"summary"`
	HumanRoutes    []string                `json:"humanRoutes"`
	AgentRoutes    []parityRoute           `json:"agentRoutes,omitempty"`
	FallbackTool   string                  `json:"fallbackTool,omitempty"`
	Risk           commands.CapabilityRisk `json:"risk"`
	RequiresDaemon bool                    `json:"requiresDaemon"`
	Level          int                     `json:"level"`
	Status         string                  `json:"status"`
}

type parityTotals struct {
	Capabilities int     `json:"capabilities"`
	Reachable    int     `json:"reachable"`
	Typed        int     `json:"typed"`
	Contracted   int     `json:"contracted"`
	Reachability float64 `json:"reachabilityPercentage"`
	TypedPercent float64 `json:"typedPercentage"`
	ContractRate float64 `json:"contractPercentage"`
}

type parityReport struct {
	SchemaVersion string             `json:"schemaVersion"`
	Totals        parityTotals       `json:"totals"`
	Capabilities  []parityCapability `json:"capabilities"`
}

func main() {
	output := flag.String("output", "", "write JSON to this path instead of stdout")
	flag.Parse()
	report, err := buildReport()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	data, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	data = append(data, '\n')
	if *output == "" {
		_, err = os.Stdout.Write(data)
	} else {
		err = os.MkdirAll(filepath.Dir(*output), 0o755)
		if err == nil {
			err = os.WriteFile(*output, data, 0o644)
		}
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func buildReport() (parityReport, error) {
	capabilities, err := commands.HovelRegistry(commands.Runtime{}).OperatorCapabilities()
	if err != nil {
		return parityReport{}, err
	}
	known := make(map[commands.CapabilityID]commands.OperatorCapability, len(capabilities))
	for _, capability := range capabilities {
		known[capability.ID] = capability
	}
	routes := make(map[commands.CapabilityID][]commands.AgentRoute)
	for _, route := range mcpadapter.OperatorCapabilityRoutes() {
		capability, ok := known[route.Capability]
		if !ok {
			return parityReport{}, fmt.Errorf("MCP tool %s references unknown operator capability %s", route.Tool, route.Capability)
		}
		if capability.Risk != route.Risk {
			return parityReport{}, fmt.Errorf("MCP tool %s risk %s disagrees with capability %s risk %s", route.Tool, route.Risk, route.Capability, capability.Risk)
		}
		routes[route.Capability] = append(routes[route.Capability], route)
	}

	report := parityReport{SchemaVersion: commands.OperatorParitySchemaVersion}
	for _, capability := range capabilities {
		item := parityCapability{
			ID: capability.ID, Summary: capability.Summary, HumanRoutes: capability.HumanRoutes,
			Risk: capability.Risk, RequiresDaemon: capability.RequiresDaemon,
			FallbackTool: mcpadapter.ToolCommandRun, Level: 1, Status: "command_run",
		}
		for _, route := range routes[capability.ID] {
			item.AgentRoutes = append(item.AgentRoutes, parityRoute{Tool: route.Tool, Evidence: route.Evidence})
			item.FallbackTool = ""
			if len(route.Evidence) > 0 {
				item.Level, item.Status = 3, "contracted"
			} else if item.Level < 2 {
				item.Level, item.Status = 2, "typed"
			}
		}
		sort.Slice(item.AgentRoutes, func(i, j int) bool { return item.AgentRoutes[i].Tool < item.AgentRoutes[j].Tool })
		report.Capabilities = append(report.Capabilities, item)
		report.Totals.Capabilities++
		report.Totals.Reachable++
		if item.Level >= 2 {
			report.Totals.Typed++
		}
		if item.Level >= 3 {
			report.Totals.Contracted++
		}
	}
	total := float64(report.Totals.Capabilities)
	if total > 0 {
		report.Totals.Reachability = 100 * float64(report.Totals.Reachable) / total
		report.Totals.TypedPercent = 100 * float64(report.Totals.Typed) / total
		report.Totals.ContractRate = 100 * float64(report.Totals.Contracted) / total
	}
	if report.Totals.Contracted != report.Totals.Capabilities {
		return parityReport{}, fmt.Errorf("operator semantic parity is incomplete: %d of %d capabilities are contracted", report.Totals.Contracted, report.Totals.Capabilities)
	}
	return report, nil
}
