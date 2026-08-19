package commands

import (
	"fmt"
	"sort"
	"strings"
)

const OperatorParitySchemaVersion = "hovel.operator-parity/v1"

type CapabilityID string

type CapabilityRisk string

const (
	CapabilityRead        CapabilityRisk = "read"
	CapabilityWrite       CapabilityRisk = "write"
	CapabilityDestructive CapabilityRisk = "destructive"
)

type OperatorCapability struct {
	ID             CapabilityID   `json:"id"`
	Summary        string         `json:"summary"`
	HumanRoutes    []string       `json:"humanRoutes"`
	Risk           CapabilityRisk `json:"risk"`
	RequiresDaemon bool           `json:"requiresDaemon"`
}

type AgentRoute struct {
	Tool       string         `json:"tool"`
	Capability CapabilityID   `json:"capability"`
	Risk       CapabilityRisk `json:"risk"`
	Evidence   []string       `json:"evidence,omitempty"`
}

func (r Registry) OperatorCapabilities() ([]OperatorCapability, error) {
	byID := make(map[CapabilityID]OperatorCapability, len(r.definitions))
	for _, definition := range r.definitions {
		id := CapabilityIDForPath(definition.Path)
		if id == "" {
			return nil, fmt.Errorf("command %q has no operator capability id", definition.PathString())
		}
		if existing, ok := byID[id]; ok {
			return nil, fmt.Errorf("operator capability %q is shared by commands %q and %q", id, existing.HumanRoutes[0], definition.PathString())
		}
		byID[id] = OperatorCapability{
			ID:             id,
			Summary:        definition.Summary,
			HumanRoutes:    []string{definition.PathString()},
			Risk:           capabilityRisk(definition.Path),
			RequiresDaemon: definition.RequiresDaemon,
		}
	}
	result := make([]OperatorCapability, 0, len(byID))
	for _, capability := range byID {
		result = append(result, capability)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].ID < result[j].ID })
	return result, nil
}

func CapabilityIDForPath(path []string) CapabilityID {
	if len(path) == 0 {
		return ""
	}
	segments := append([]string(nil), path...)
	switch strings.Join(segments, " ") {
	case "control init":
		return "workspace.init"
	case "control daemon status":
		return "daemon.status"
	case "throw":
		return "throw.start"
	case "confirm":
		return "throw.confirm"
	case "review":
		return "throw.review"
	}
	switch segments[0] {
	case "op":
		segments[0] = "operation"
	case "payloads":
		segments[0] = "payload"
	}
	return CapabilityID(strings.Join(segments, "."))
}

func capabilityRisk(path []string) CapabilityRisk {
	joined := strings.Join(path, " ")
	for _, readSuffix := range []string{" list", " inspect", " status", " available", " installed", " commands", " logs", " tail", " read", " check"} {
		if strings.HasSuffix(" "+joined, readSuffix) {
			return CapabilityRead
		}
	}
	for _, destructive := range []string{" delete", " uninstall", " cleanup", " clear", " close", " revoke", " unbind", " mark-removed"} {
		if strings.Contains(" "+joined, destructive) {
			return CapabilityDestructive
		}
	}
	if joined == "throw" || joined == "session send" || joined == "session call" || joined == "payloads call" || joined == "payloads cmd" {
		return CapabilityDestructive
	}
	return CapabilityWrite
}
