package agentintegration

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/vibepwners/hovel/internal/version"
)

type Host string

const (
	HostClaude   Host = "claude"
	HostCodex    Host = "codex"
	HostOpenCode Host = "opencode"
)

type Scope string

const (
	ScopeUser    Scope = "user"
	ScopeProject Scope = "project"
)

type InstallRequest struct {
	Host       Host
	Scope      Scope
	Version    string
	Source     string
	ProjectDir string
	DryRun     bool
	Force      bool
}

type Installer interface {
	Install(context.Context, InstallRequest, io.Writer) error
}

type Service struct {
	Installer Installer
}

func (s Service) Install(ctx context.Context, request InstallRequest, output io.Writer) error {
	if s.Installer == nil {
		return errors.New("agent integration installer is required")
	}
	request.Scope = Scope(strings.ToLower(strings.TrimSpace(string(request.Scope))))
	if request.Scope == "" {
		request.Scope = ScopeUser
	}
	if request.Scope != ScopeUser && request.Scope != ScopeProject {
		return fmt.Errorf("unsupported agent installation scope %q; use user or project", request.Scope)
	}
	request.Host = Host(strings.ToLower(strings.TrimSpace(string(request.Host))))
	switch request.Host {
	case HostClaude, HostCodex, HostOpenCode:
	default:
		return fmt.Errorf("unsupported agent host %q; use claude, codex, or opencode", request.Host)
	}
	request.Version = strings.TrimSpace(request.Version)
	versionProvided := request.Version != ""
	if request.Version == "" {
		request.Version = version.Version
	}
	if !validVersion(request.Version) {
		return fmt.Errorf("invalid Hovel agent integration version %q", request.Version)
	}
	if strings.TrimSpace(request.Source) != "" && versionProvided {
		return errors.New("--source and --version cannot be used together")
	}
	if output == nil {
		output = io.Discard
	}
	return s.Installer.Install(ctx, request, output)
}

func validVersion(value string) bool {
	value = strings.TrimPrefix(value, "v")
	parts := strings.Split(value, ".")
	if len(parts) != 3 {
		return false
	}
	for _, part := range parts {
		if part == "" {
			return false
		}
		for _, char := range part {
			if char < '0' || char > '9' {
				return false
			}
		}
	}
	return true
}
