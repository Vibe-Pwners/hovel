package daemon

import (
	"errors"
	"strings"
	"time"
)

type State string

const (
	StateNotRunning State = "not_running"
	StateRunning    State = "running"
)

type Health string

const (
	HealthUnknown Health = "unknown"
	HealthHealthy Health = "healthy"
)

type IdentityArgs struct {
	WorkspacePath string
	PID           int
	SocketPath    string
	HovelConfig   string
	StartedAt     time.Time
	Health        Health
	Listeners     []Listener
}

type Listener struct {
	Network   string `json:"network"`
	Bind      string `json:"bind"`
	Advertise string `json:"advertise"`
	Access    string `json:"access"`
}

type Identity struct {
	WorkspacePath string
	PID           int
	SocketPath    string
	HovelConfig   string
	StartedAt     time.Time
	Health        Health
	Listeners     []Listener
}

func NewIdentity(args IdentityArgs) (Identity, error) {
	args.WorkspacePath = strings.TrimSpace(args.WorkspacePath)
	args.SocketPath = strings.TrimSpace(args.SocketPath)
	if args.WorkspacePath == "" {
		return Identity{}, errors.New("daemon workspace path is required")
	}
	if args.PID <= 0 {
		return Identity{}, errors.New("daemon pid is required")
	}
	if args.SocketPath == "" {
		return Identity{}, errors.New("daemon socket path is required")
	}
	if args.StartedAt.IsZero() {
		return Identity{}, errors.New("daemon start time is required")
	}
	if args.Health == "" {
		args.Health = HealthUnknown
	}
	return Identity{
		WorkspacePath: args.WorkspacePath,
		PID:           args.PID,
		SocketPath:    args.SocketPath,
		HovelConfig:   strings.TrimSpace(args.HovelConfig),
		StartedAt:     args.StartedAt,
		Health:        args.Health,
		Listeners:     append([]Listener(nil), args.Listeners...),
	}, nil
}

type Status struct {
	WorkspacePath string
	State         State
	Identity      Identity
}

func NotRunning(workspacePath string) Status {
	return Status{
		WorkspacePath: strings.TrimSpace(workspacePath),
		State:         StateNotRunning,
	}
}

func Running(identity Identity) Status {
	return Status{
		WorkspacePath: identity.WorkspacePath,
		State:         StateRunning,
		Identity:      identity,
	}
}
