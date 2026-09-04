package daemonlocal

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/vibepwners/hovel/internal/adapters/daemonrpc"
	"github.com/vibepwners/hovel/internal/adapters/storage/filesystem"
	sqlitestore "github.com/vibepwners/hovel/internal/adapters/storage/sqlite"
	"github.com/vibepwners/hovel/internal/app/hovelconfig"
	apppki "github.com/vibepwners/hovel/internal/app/pki"
	"github.com/vibepwners/hovel/internal/app/services"
	"github.com/vibepwners/hovel/internal/domain/daemon"
	"github.com/vibepwners/hovel/internal/domain/workspace"
	"github.com/vibepwners/hovel/internal/infra/daemonmanager"
	"github.com/vibepwners/hovel/internal/infra/daemonruntime"
	"github.com/vibepwners/hovel/internal/moduleruntime/pythonrpc"
)

type ClientOptions struct {
	Endpoint                 string
	AllowInsecureFullControl bool
	ConnectTimeout           time.Duration
}

type ClientOverrides struct {
	Endpoint         string
	AllowInsecure    bool
	AllowInsecureSet bool
	ConnectTimeout   string
}

func ResolveClientOptions(workspacePath, configPath string, overrides ClientOverrides) (ClientOptions, error) {
	config, _, err := hovelconfig.Load(hovelconfig.Options{Workspace: workspace.ResolvePath(workspacePath), ExplicitPath: configPath})
	if err != nil {
		return ClientOptions{}, err
	}
	options := ClientOptions{
		Endpoint:                 strings.TrimSpace(config.Daemon.Client.Endpoint),
		AllowInsecureFullControl: config.Daemon.Client.AllowInsecureFullControl,
	}
	if value := strings.TrimSpace(config.Daemon.Client.ConnectTimeout); value != "" {
		options.ConnectTimeout, err = time.ParseDuration(value)
		if err != nil {
			return ClientOptions{}, errors.New("invalid daemon client connect timeout: " + err.Error())
		}
	}
	if value := strings.TrimSpace(os.Getenv("HOVEL_DAEMON_ENDPOINT")); value != "" {
		options.Endpoint = value
	}
	if value := strings.TrimSpace(os.Getenv("HOVEL_DAEMON_ALLOW_INSECURE_FULL_CONTROL")); value != "" {
		options.AllowInsecureFullControl, err = strconv.ParseBool(value)
		if err != nil {
			return ClientOptions{}, errors.New("invalid HOVEL_DAEMON_ALLOW_INSECURE_FULL_CONTROL")
		}
	}
	if value := strings.TrimSpace(os.Getenv("HOVEL_DAEMON_CONNECT_TIMEOUT")); value != "" {
		options.ConnectTimeout, err = time.ParseDuration(value)
		if err != nil {
			return ClientOptions{}, errors.New("invalid HOVEL_DAEMON_CONNECT_TIMEOUT: " + err.Error())
		}
	}
	if value := strings.TrimSpace(overrides.Endpoint); value != "" {
		options.Endpoint = value
	}
	if overrides.AllowInsecureSet {
		options.AllowInsecureFullControl = overrides.AllowInsecure
	}
	if value := strings.TrimSpace(overrides.ConnectTimeout); value != "" {
		options.ConnectTimeout, err = time.ParseDuration(value)
		if err != nil {
			return ClientOptions{}, errors.New("invalid --daemon-connect-timeout: " + err.Error())
		}
	}
	return options, nil
}

func Connect(ctx context.Context, workspacePath, moduleConfig, configPath string, options ClientOptions, requireFull bool) (*daemonmanager.Session, *daemonrpc.Client, error) {
	if strings.TrimSpace(options.Endpoint) == "" {
		session, err := NewManager().EnsureWithConfig(ctx, workspacePath, moduleConfig, configPath)
		if err != nil {
			return nil, nil, err
		}
		client, err := daemonrpc.DialWithOptions(session.Status().Identity.SocketPath, daemonrpc.DialOptions{
			ConnectTimeout: options.ConnectTimeout, AcknowledgeInsecureFullControl: options.AllowInsecureFullControl,
		})
		if err != nil {
			return nil, nil, errors.Join(err, session.Close())
		}
		return session, client, nil
	}
	client, err := daemonrpc.DialWithOptions(options.Endpoint, daemonrpc.DialOptions{
		ConnectTimeout: options.ConnectTimeout, AcknowledgeInsecureFullControl: options.AllowInsecureFullControl,
	})
	if err != nil {
		return nil, nil, err
	}
	info, err := client.GetDaemonInfo(ctx)
	if err != nil {
		return nil, nil, errors.Join(err, client.Close())
	}
	if requireFull && info.Access == string(daemonrpc.TransportAccessReadOnly) {
		return nil, nil, errors.Join(
			errors.New("daemon TCP endpoint is read-only; configure server access insecure-full and acknowledge it on the client"),
			client.Close(),
		)
	}
	if requireFull && info.Access == string(daemonrpc.TransportAccessInsecureFull) && !options.AllowInsecureFullControl {
		return nil, nil, errors.Join(errors.New("daemon TCP endpoint requires --allow-insecure-daemon"), client.Close())
	}
	startedAt, err := time.Parse(time.RFC3339Nano, info.StartedAt)
	if err != nil {
		return nil, nil, errors.Join(fmt.Errorf("invalid daemon start time: %w", err), client.Close())
	}
	listeners := make([]daemon.Listener, 0, len(info.Listeners))
	for _, listener := range info.Listeners {
		listeners = append(listeners, daemon.Listener{
			Network: listener.Network, Bind: listener.Bind,
			Advertise: listener.Advertise, Access: listener.Access,
		})
	}
	identity, err := daemon.NewIdentity(daemon.IdentityArgs{
		WorkspacePath: info.WorkspacePath, PID: info.PID, SocketPath: options.Endpoint,
		StartedAt: startedAt, Health: daemon.Health(info.Health), Listeners: listeners,
	})
	if err != nil {
		return nil, nil, errors.Join(err, client.Close())
	}
	return daemonmanager.NewAttachedSession(daemon.Running(identity)), client, nil
}

func NewManager() daemonmanager.Manager {
	store := filesystem.NewWorkspaceStore()
	return daemonmanager.New(store, SocketReachable, EndpointNetwork, Serve)
}

func Serve(ctx context.Context, args daemonruntime.Args) error {
	return daemonruntime.Serve(ctx, WithDefaults(args))
}

func WithDefaults(args daemonruntime.Args) daemonruntime.Args {
	if args.ParseEndpoint == nil {
		args.ParseEndpoint = ParseEndpoint
	}
	if args.Store == nil {
		args.Store = filesystem.NewWorkspaceStore()
	}
	if args.AcquireWorkspaceLock == nil {
		args.AcquireWorkspaceLock = func(workspacePath, owner string) (daemonruntime.WorkspaceLock, error) {
			return filesystem.AcquireWorkspaceLock(workspacePath, owner)
		}
	}
	if args.NewEventSink == nil {
		args.NewEventSink = func(workspacePath string) services.EventSink {
			return sqlitestore.NewStore(workspacePath)
		}
	}
	if args.NewLogPublisher == nil {
		args.NewLogPublisher = func() daemonruntime.LogPublisher {
			return daemonrpc.NewLogBroker()
		}
	}
	if args.NewRPCServer == nil {
		args.NewRPCServer = NewRPCServer
	}
	if args.WrapRPCHandler == nil {
		args.WrapRPCHandler = func(handler http.Handler, endpoint daemonruntime.Endpoint) http.Handler {
			return daemonrpc.WithTransportAccess(handler, daemonrpc.TransportAccess(endpoint.Access))
		}
	}
	if args.NewModuleRuntime == nil {
		args.NewModuleRuntime = NewModuleRuntime
	}
	if args.NewPKIControl == nil {
		if args.PKIBackends == nil && args.PKIValidators == nil {
			args.NewPKIControl = newWorkspacePKIControl
		} else {
			backends := args.PKIBackends
			validators := args.PKIValidators
			args.NewPKIControl = func(ctx context.Context, workspacePath string) (apppki.WorkspaceControl, error) {
				return newWorkspacePKIControlWithRegistries(
					ctx, workspacePath, backends, validators,
				)
			}
		}
	}
	return args
}

func ParseEndpoint(value string) (daemonruntime.Endpoint, error) {
	endpoint, err := daemonrpc.ParseEndpoint(value)
	if err != nil {
		return daemonruntime.Endpoint{}, err
	}
	return daemonruntime.Endpoint{
		Network: endpoint.Network,
		Address: endpoint.Address,
		Display: endpoint.String(),
	}, nil
}

func EndpointNetwork(value string) (string, bool) {
	endpoint, err := daemonrpc.ParseEndpoint(value)
	if err != nil {
		return "", false
	}
	return endpoint.Network, true
}

func SocketReachable(ctx context.Context, socketPath string) bool {
	client, err := daemonrpc.Dial(socketPath)
	if err != nil {
		return false
	}
	defer func() { logDaemonLocalError("close daemon health-check client", client.Close()) }()
	_, err = client.PollLogs(ctx, 0)
	return err == nil
}

func NewRPCServer(config daemonruntime.RPCServerConfig) (http.Handler, error) {
	logs, ok := config.Logs.(*daemonrpc.LogBroker)
	if !ok {
		return nil, errors.New("daemon local rpc server requires daemonrpc log broker")
	}
	listeners := make([]daemonrpc.DaemonListenerInfo, 0, len(config.Identity.Listeners))
	for _, listener := range config.Identity.Listeners {
		listeners = append(listeners, daemonrpc.DaemonListenerInfo{
			Network: listener.Network, Bind: listener.Bind,
			Advertise: listener.Advertise, Access: listener.Access,
		})
	}
	options := []daemonrpc.ServerOption{
		daemonrpc.WithDaemonInfo(daemonrpc.DaemonInfo{
			WorkspacePath: config.Identity.WorkspacePath,
			PID:           config.Identity.PID,
			StartedAt:     config.Identity.StartedAt.UTC().Format(time.RFC3339Nano),
			Health:        string(config.Identity.Health),
			Listeners:     listeners,
		}),
		daemonrpc.WithModuleCatalog(config.Modules),
		daemonrpc.WithSession(config.Session),
		daemonrpc.WithLogBroker(logs),
		daemonrpc.WithSessionPersistence(config.PersistSession),
		daemonrpc.WithModuleSessions(config.ModuleSessions),
		daemonrpc.WithLaunchKeyPolicy(config.LaunchKeyPolicy),
		daemonrpc.WithPKIControl(config.PKI),
		daemonrpc.WithPKISecretResponses(config.Confidential),
		daemonrpc.WithPrivilegedControl(config.Confidential),
	}
	if config.ModuleProvider != nil {
		options = append(options, daemonrpc.WithModuleCatalogProvider(config.ModuleProvider.Catalog))
	}
	return daemonrpc.NewHandler(
		config.Runs,
		options...,
	)
}

func NewModuleRuntime(config daemonruntime.ModuleRuntimeConfig) (services.ModuleRunner, services.SessionBroker) {
	sessions := pythonrpc.NewSessionBroker()
	return pythonrpc.Runner{
		ConfigPath:           config.ModuleConfig,
		HovelConfig:          config.HovelConfig,
		WorkspacePath:        config.WorkspacePath,
		Events:               config.Events,
		IDs:                  config.IDs,
		Clock:                config.Clock,
		Sessions:             sessions,
		CredentialExecutions: config.CredentialExecutions,
	}, sessions
}
