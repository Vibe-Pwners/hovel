package daemonruntime

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/vibepwners/hovel/internal/app/hovelconfig"
	"github.com/vibepwners/hovel/internal/app/modulecatalog"
	"github.com/vibepwners/hovel/internal/app/operatorlog"
	"github.com/vibepwners/hovel/internal/app/operatorsession"
	apppki "github.com/vibepwners/hovel/internal/app/pki"
	"github.com/vibepwners/hovel/internal/app/services"
	"github.com/vibepwners/hovel/internal/domain/daemon"
	"github.com/vibepwners/hovel/internal/domain/event"
	operatordomain "github.com/vibepwners/hovel/internal/domain/operator"
	"github.com/vibepwners/hovel/internal/domain/workspace"
)

const daemonShutdownTimeout = 5 * time.Second

type Endpoint struct {
	Network   string
	Address   string
	Display   string
	Advertise string
	Access    string
}

func (e Endpoint) String() string {
	if e.Advertise != "" {
		return e.Advertise
	}
	if e.Display != "" {
		return e.Display
	}
	if e.Network == "tcp" {
		return "tcp://" + e.Address
	}
	return e.Address
}

func resolveListenerSpecs(args Args, config hovelconfig.Config, workspacePath string) ([]ListenerSpec, error) {
	if len(args.Listeners) != 0 {
		return applyInsecureTCP(args, append([]ListenerSpec(nil), args.Listeners...)), nil
	}
	if value := strings.TrimSpace(os.Getenv("HOVEL_DAEMON_LISTENERS")); value != "" {
		var listeners []ListenerSpec
		if err := json.Unmarshal([]byte(value), &listeners); err != nil {
			return nil, fmt.Errorf("invalid HOVEL_DAEMON_LISTENERS: %w", err)
		}
		return applyInsecureTCP(args, listeners), nil
	}
	if len(config.Daemon.Listeners) != 0 {
		listeners := make([]ListenerSpec, 0, len(config.Daemon.Listeners))
		for _, listener := range config.Daemon.Listeners {
			listeners = append(listeners, ListenerSpec{Bind: listener.Bind, Advertise: listener.Advertise, Access: listener.Access})
		}
		return applyInsecureTCP(args, listeners), nil
	}
	listenAddress := args.ListenAddress
	if listenAddress == "" {
		listenAddress = args.SocketPath
	}
	if listenAddress == "" {
		listenAddress = filepath.Join(workspacePath, "hoveld.sock")
	}
	return applyInsecureTCP(args, []ListenerSpec{{Bind: listenAddress}}), nil
}

func applyInsecureTCP(args Args, listeners []ListenerSpec) []ListenerSpec {
	if !args.AllowInsecureTCP {
		return listeners
	}
	for i := range listeners {
		endpoint, err := args.ParseEndpoint(listeners[i].Bind)
		if err == nil && endpoint.Network == "tcp" {
			listeners[i].Access = "insecure-full"
		}
	}
	return listeners
}

func parseListenerEndpoints(specs []ListenerSpec, parse EndpointParser) ([]Endpoint, error) {
	if len(specs) == 0 {
		return nil, errors.New("daemon requires at least one listener")
	}
	endpoints := make([]Endpoint, 0, len(specs))
	seen := map[string]struct{}{}
	for _, spec := range specs {
		endpoint, err := parse(spec.Bind)
		if err != nil {
			return nil, fmt.Errorf("parse daemon listener %q: %w", spec.Bind, err)
		}
		key := endpoint.Network + "\x00" + endpoint.Address
		if _, ok := seen[key]; ok {
			return nil, fmt.Errorf("duplicate daemon listener %q", spec.Bind)
		}
		seen[key] = struct{}{}
		endpoint.Access = strings.TrimSpace(spec.Access)
		if endpoint.Access == "" {
			if endpoint.Network == "unix" {
				endpoint.Access = "owner"
			} else {
				endpoint.Access = "read-only"
			}
		}
		if endpoint.Network == "unix" && endpoint.Access != "owner" {
			return nil, fmt.Errorf("daemon unix listener access must be owner, got %q", endpoint.Access)
		}
		if endpoint.Network == "tcp" && endpoint.Access != "read-only" && endpoint.Access != "insecure-full" {
			return nil, fmt.Errorf("daemon tcp listener access must be read-only or insecure-full, got %q", endpoint.Access)
		}
		advertise := strings.TrimSpace(spec.Advertise)
		if advertise != "" {
			advertised, err := parse(advertise)
			if err != nil {
				return nil, fmt.Errorf("parse advertised daemon endpoint %q: %w", advertise, err)
			}
			if advertised.Network != endpoint.Network {
				return nil, errors.New("daemon listener bind and advertise networks must match")
			}
			if advertised.Network == "tcp" {
				host, _, splitErr := net.SplitHostPort(advertised.Address)
				if splitErr != nil {
					return nil, splitErr
				}
				if host == "" || net.ParseIP(host) != nil && net.ParseIP(host).IsUnspecified() {
					return nil, errors.New("advertised daemon tcp endpoint requires a dialable host")
				}
			}
			endpoint.Advertise = advertised.String()
		}
		if endpoint.Network == "tcp" {
			host, port, splitErr := net.SplitHostPort(endpoint.Address)
			if splitErr != nil {
				return nil, splitErr
			}
			unspecified := host == "" || net.ParseIP(host) != nil && net.ParseIP(host).IsUnspecified()
			if (unspecified || port == "0") && endpoint.Advertise == "" {
				return nil, fmt.Errorf("daemon listener %q requires an advertised endpoint", spec.Bind)
			}
		}
		endpoints = append(endpoints, endpoint)
	}
	return endpoints, nil
}

func bindListeners(endpoints []Endpoint) ([]net.Listener, []Endpoint, error) {
	listeners := make([]net.Listener, 0, len(endpoints))
	bound := append([]Endpoint(nil), endpoints...)
	for i := range bound {
		endpoint := &bound[i]
		if endpoint.Network == "unix" {
			if err := os.Remove(endpoint.Address); err != nil && !errors.Is(err, os.ErrNotExist) {
				closeListeners(listeners, bound[:i])
				return nil, nil, err
			}
		}
		listener, err := net.Listen(endpoint.Network, endpoint.Address)
		if err != nil {
			closeListeners(listeners, bound[:i])
			return nil, nil, err
		}
		listeners = append(listeners, listener)
		if endpoint.Network == "unix" {
			const privateSocketMode = 0o600
			if err := os.Chmod(endpoint.Address, privateSocketMode); err != nil {
				closeListeners(listeners, bound[:i+1])
				return nil, nil, fmt.Errorf("restrict daemon socket permissions: %w", err)
			}
			if endpoint.Advertise == "" {
				endpoint.Advertise = endpoint.Display
			}
			continue
		}
		actual := listener.Addr().String()
		_, actualPort, splitErr := net.SplitHostPort(actual)
		if splitErr != nil {
			closeListeners(listeners, bound[:i+1])
			return nil, nil, splitErr
		}
		endpoint.Address = actual
		if endpoint.Advertise == "" {
			endpoint.Advertise = "tcp://" + actual
		} else {
			advertisedHost, advertisedPort, splitErr := net.SplitHostPort(strings.TrimPrefix(endpoint.Advertise, "tcp://"))
			if splitErr != nil {
				closeListeners(listeners, bound[:i+1])
				return nil, nil, splitErr
			}
			if advertisedPort == "0" {
				endpoint.Advertise = "tcp://" + net.JoinHostPort(advertisedHost, actualPort)
			}
		}
	}
	return listeners, bound, nil
}

func closeListeners(listeners []net.Listener, endpoints []Endpoint) {
	for _, listener := range listeners {
		logDaemonRuntimeError("close daemon listener", listener.Close())
	}
	for _, endpoint := range endpoints {
		if endpoint.Network == "unix" {
			logDaemonRuntimeError("remove daemon socket", os.Remove(endpoint.Address))
		}
	}
}

func shutdownServers(servers []*http.Server) error {
	ctx, cancel := context.WithTimeout(context.Background(), daemonShutdownTimeout)
	defer cancel()
	var errs []error
	for _, server := range servers {
		if err := server.Shutdown(ctx); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

func collectServeErrors(errs <-chan error, count int) error {
	var collected []error
	for range count {
		if err := <-errs; err != nil {
			collected = append(collected, err)
		}
	}
	return errors.Join(collected...)
}

type EndpointParser func(string) (Endpoint, error)

type ListenerSpec struct {
	Bind      string `json:"bind"`
	Advertise string `json:"advertise,omitempty"`
	Access    string `json:"access,omitempty"`
}

type RPCHandlerWrapper func(http.Handler, Endpoint) http.Handler

type WorkspaceLock interface {
	Release() error
}

type WorkspaceLockFactory func(workspacePath, owner string) (WorkspaceLock, error)

type WorkspaceStore interface {
	EnsureWorkspaceDatabase(context.Context, string) error
	SaveOperatorSession(context.Context, string, operatorsession.PersistedState) error
	LoadOperatorSession(context.Context, string) (operatorsession.PersistedState, bool, error)
	WriteDaemonStatus(context.Context, daemon.Identity) error
	ClearDaemonStatus(context.Context, string) error
}

type EventSinkFactory func(workspacePath string) services.EventSink

type LogPublisher interface {
	Publish(operation, chain string, entries ...operatorlog.Entry)
}

type LogPublisherFactory func() LogPublisher

type RPCServerConfig struct {
	Runs            services.RunService
	Session         *operatorsession.Session
	Logs            LogPublisher
	PersistSession  func(operatorsession.PersistedState) error
	ModuleSessions  services.SessionBroker
	LaunchKeyPolicy operatordomain.LaunchKeyPolicy
	PKI             apppki.WorkspaceControl
	Confidential    bool
	Identity        daemon.Identity
	Modules         modulecatalog.Catalog
	ModuleProvider  ModuleCatalogProvider
}

type ModuleCatalogProvider interface {
	Catalog(context.Context) (modulecatalog.Catalog, error)
}

type RPCServerFactory func(RPCServerConfig) (http.Handler, error)

type ModuleRuntimeConfig struct {
	ModuleConfig         string
	HovelConfig          string
	WorkspacePath        string
	Events               services.EventSink
	IDs                  services.IDGenerator
	Clock                services.Clock
	CredentialExecutions apppki.CredentialExecutionRecorder
}

type ModuleRuntimeFactory func(ModuleRuntimeConfig) (services.ModuleRunner, services.SessionBroker)

type PKIControlFactory func(context.Context, string) (apppki.WorkspaceControl, error)

type Args struct {
	WorkspacePath        string
	SocketPath           string
	ListenAddress        string
	Listeners            []ListenerSpec
	AllowInsecureTCP     bool
	ModuleConfig         string
	HovelConfig          string
	PID                  int
	StartedAt            time.Time
	IDs                  services.IDGenerator
	Clock                services.Clock
	Events               services.EventSink
	ModuleRunner         services.ModuleRunner
	ModuleSessions       services.SessionBroker
	ParseEndpoint        EndpointParser
	Store                WorkspaceStore
	AcquireWorkspaceLock WorkspaceLockFactory
	NewEventSink         EventSinkFactory
	NewLogPublisher      LogPublisherFactory
	NewRPCServer         RPCServerFactory
	WrapRPCHandler       RPCHandlerWrapper
	NewModuleRuntime     ModuleRuntimeFactory
	NewPKIControl        PKIControlFactory
	PKIBackends          apppki.BackendRegistry
	PKIValidators        apppki.ValidatorRegistry
}

func Serve(ctx context.Context, args Args) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	workspacePath := workspace.ResolvePath(args.WorkspacePath)
	if args.ParseEndpoint == nil {
		return errors.New("daemon runtime endpoint parser is not configured")
	}
	config, _, err := hovelconfig.Load(hovelconfig.Options{
		Workspace:    workspacePath,
		ExplicitPath: args.HovelConfig,
	})
	if err != nil {
		return err
	}
	listenerSpecs, err := resolveListenerSpecs(args, config, workspacePath)
	if err != nil {
		return err
	}
	endpoints, err := parseListenerEndpoints(listenerSpecs, args.ParseEndpoint)
	if err != nil {
		return err
	}
	pid := args.PID
	if pid == 0 {
		pid = os.Getpid()
	}
	startedAt := args.StartedAt
	if startedAt.IsZero() {
		startedAt = time.Now().UTC()
	}
	ids := args.IDs
	if ids == nil {
		ids = runtimeIDs{}
	}
	clock := args.Clock
	if clock == nil {
		clock = systemClock{}
	}
	baseEvents := args.Events
	ownsBaseEvents := false
	if baseEvents == nil {
		if args.NewEventSink == nil {
			return errors.New("daemon runtime event sink factory is not configured")
		}
		baseEvents = args.NewEventSink(workspacePath)
		ownsBaseEvents = true
	}
	if ownsBaseEvents {
		if closer, ok := baseEvents.(interface{ Close() error }); ok {
			defer func() { logDaemonRuntimeError("close daemon event sink", closer.Close()) }()
		}
	}
	store := args.Store
	if store == nil {
		return errors.New("daemon runtime workspace store is not configured")
	}
	if args.AcquireWorkspaceLock == nil {
		return errors.New("daemon runtime workspace lock factory is not configured")
	}
	if args.NewLogPublisher == nil {
		return errors.New("daemon runtime log publisher factory is not configured")
	}
	if args.NewRPCServer == nil {
		return errors.New("daemon runtime rpc server factory is not configured")
	}

	lock, err := args.AcquireWorkspaceLock(workspacePath, fmt.Sprintf("pid:%d", os.Getpid()))
	if err != nil {
		return err
	}
	defer func() { logDaemonRuntimeError("release workspace lock", lock.Release()) }()

	if err := store.EnsureWorkspaceDatabase(ctx, workspacePath); err != nil {
		return err
	}
	var pkiControl apppki.WorkspaceControl
	if args.NewPKIControl != nil {
		pkiControl, err = args.NewPKIControl(ctx, workspacePath)
		if err != nil {
			return err
		}
		defer func() { logDaemonRuntimeError("close workspace PKI", pkiControl.Close()) }()
	}
	session := operatorsession.New()
	if state, ok, err := store.LoadOperatorSession(ctx, workspacePath); err != nil {
		return err
	} else if ok {
		session.Import(state)
	}
	persistSession := func(state operatorsession.PersistedState) error {
		return store.SaveOperatorSession(context.Background(), workspacePath, state)
	}
	logs := args.NewLogPublisher()
	events := newPublishingEventSink(baseEvents, session, logs, func() error {
		return persistSession(session.Export())
	})
	runner := args.ModuleRunner
	sessionBroker := args.ModuleSessions
	if runner == nil {
		if args.NewModuleRuntime == nil {
			return errors.New("daemon runtime module runtime factory is not configured")
		}
		credentialExecutions, _ := pkiControl.(apppki.CredentialExecutionRecorder)
		runner, sessionBroker = args.NewModuleRuntime(ModuleRuntimeConfig{
			ModuleConfig:         args.ModuleConfig,
			HovelConfig:          args.HovelConfig,
			WorkspacePath:        workspacePath,
			Events:               events,
			IDs:                  ids,
			Clock:                clock,
			CredentialExecutions: credentialExecutions,
		})
		if runner == nil {
			return errors.New("daemon runtime module runner factory returned nil")
		}
	}
	modules := modulecatalog.New()
	moduleProvider, _ := runner.(ModuleCatalogProvider)

	listeners, endpoints, err := bindListeners(endpoints)
	if err != nil {
		return err
	}
	defer closeListeners(listeners, endpoints)

	identityListeners := make([]daemon.Listener, 0, len(endpoints))
	for _, endpoint := range endpoints {
		identityListeners = append(identityListeners, daemon.Listener{
			Network: endpoint.Network, Bind: endpoint.Address,
			Advertise: endpoint.String(), Access: endpoint.Access,
		})
	}
	identity, err := daemon.NewIdentity(daemon.IdentityArgs{
		WorkspacePath: workspacePath,
		PID:           pid,
		SocketPath:    endpoints[0].String(),
		HovelConfig:   args.HovelConfig,
		StartedAt:     startedAt,
		Health:        daemon.HealthHealthy,
		Listeners:     identityListeners,
	})
	if err != nil {
		return err
	}

	runOptions := make([]services.RunServiceOption, 0, 1)
	if credentialResolver, ok := pkiControl.(services.CredentialOperationResolver); ok {
		runOptions = append(
			runOptions,
			services.WithCredentialOperationResolver(credentialResolver),
		)
	}
	runs := services.NewRunService(runner, events, ids, clock, runOptions...)
	handler, err := args.NewRPCServer(RPCServerConfig{
		Runs:            runs,
		Session:         session,
		Logs:            logs,
		PersistSession:  persistSession,
		ModuleSessions:  sessionBroker,
		LaunchKeyPolicy: launchKeyPolicyFromConfig(config.Policy.LaunchKey),
		PKI:             pkiControl,
		Confidential:    args.WrapRPCHandler != nil || allUnixEndpoints(endpoints),
		Identity:        identity,
		Modules:         modules,
		ModuleProvider:  moduleProvider,
	})
	if err != nil {
		return err
	}
	acceptErrs := make(chan error, len(listeners))
	httpServers := make([]*http.Server, 0, len(listeners))
	for i, listener := range listeners {
		listenerHandler := handler
		if args.WrapRPCHandler != nil {
			listenerHandler = args.WrapRPCHandler(handler, endpoints[i])
		}
		httpServer := &http.Server{Handler: listenerHandler}
		httpServers = append(httpServers, httpServer)
		go serveRPC(listener, httpServer, acceptErrs)
	}

	if err := store.WriteDaemonStatus(ctx, identity); err != nil {
		return err
	}

	select {
	case <-ctx.Done():
		shutdownErr := shutdownServers(httpServers)
		serveErr := collectServeErrors(acceptErrs, len(httpServers))
		if shutdownErr != nil || serveErr != nil {
			clearErr := store.ClearDaemonStatus(context.Background(), workspacePath)
			return errors.Join(shutdownErr, serveErr, clearErr)
		}
	case err := <-acceptErrs:
		shutdownErr := shutdownServers(httpServers)
		remainingErr := collectServeErrors(acceptErrs, len(httpServers)-1)
		clearErr := store.ClearDaemonStatus(context.Background(), workspacePath)
		if clearErr != nil || shutdownErr != nil || remainingErr != nil {
			return errors.Join(err, shutdownErr, remainingErr, clearErr)
		}
		return err
	}

	clearErr := store.ClearDaemonStatus(context.Background(), workspacePath)
	if clearErr != nil {
		return errors.Join(ctx.Err(), clearErr)
	}
	return nil
}

func allUnixEndpoints(endpoints []Endpoint) bool {
	for _, endpoint := range endpoints {
		if endpoint.Network != "unix" {
			return false
		}
	}
	return true
}

func launchKeyPolicyFromConfig(config hovelconfig.LaunchKeyPolicy) operatordomain.LaunchKeyPolicy {
	policy := operatordomain.LaunchKeyPolicy{
		Mode:   operatordomain.LaunchKeyMode(config.Mode),
		Quorum: config.Quorum,
	}
	if timeout := config.HeartbeatTimeout; timeout != "" {
		if parsed, err := time.ParseDuration(timeout); err == nil {
			policy.HeartbeatTimeout = parsed
		}
	}
	return operatordomain.NormalizeLaunchKeyPolicy(policy)
}

func serveRPC(listener net.Listener, server *http.Server, errs chan<- error) {
	err := server.Serve(listener)
	if err != nil && !errors.Is(err, http.ErrServerClosed) {
		errs <- err
		return
	}
	errs <- nil
}

type publishingEventSink struct {
	mu          sync.Mutex
	next        services.EventSink
	session     *operatorsession.Session
	logs        LogPublisher
	persist     func() error
	runStarts   map[string]time.Time
	throwStarts map[string]time.Time
}

func newPublishingEventSink(next services.EventSink, session *operatorsession.Session, logs LogPublisher, persist func() error) *publishingEventSink {
	return &publishingEventSink{
		next:        next,
		session:     session,
		logs:        logs,
		persist:     persist,
		runStarts:   map[string]time.Time{},
		throwStarts: map[string]time.Time{},
	}
}

func (s *publishingEventSink) Append(ctx context.Context, evt event.Event) error {
	if err := s.next.Append(ctx, evt); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	operation := evt.Refs.Operation
	chain := evt.Refs.Chain
	if operation == "" || chain == "" {
		state := s.session.Snapshot()
		if operation == "" {
			operation = state.ActiveOperation
		}
		if chain == "" {
			chain = state.ActiveChain
		}
	}
	if chain == "" {
		return nil
	}
	switch evt.Type.String() {
	case "hovel.run.started", "run.started":
		s.runStarts[evt.Refs.RunID] = evt.Timestamp
		if throwStarted, ok := evt.Fields["throwStarted"]; ok {
			if at, err := time.Parse(time.RFC3339Nano, throwStarted); err == nil && !at.IsZero() {
				s.throwStarts[chain] = at
			}
		}
		if s.throwStarts[chain].IsZero() {
			s.throwStarts[chain] = evt.Timestamp
		}
	case "hovel.module.log", "module.log":
		entry := s.moduleLogEntry(operation, chain, evt)
		if err := s.session.AppendLogToChain(chain, entry); err != nil {
			return err
		}
		s.publishLog(operation, chain, entry)
		return s.persistIfConfigured()
	case "hovel.session.created", "session.created":
		entry := s.runEventEntry(operation, chain, evt, operatorlog.Info("session", "session opened",
			operatorlog.Field{Name: "session", Value: evt.Fields["sessionId"]},
			operatorlog.Field{Name: "kind", Value: evt.Fields["kind"]},
			operatorlog.Field{Name: "state", Value: evt.Fields["state"]},
		))
		if err := s.session.AppendLogToChain(chain, entry); err != nil {
			return err
		}
		s.publishLog(operation, chain, entry)
		return s.persistIfConfigured()
	case "hovel.run.completed", "run.succeeded":
		entry := s.runEventEntry(operation, chain, evt, operatorlog.Success("throw", "run completed"))
		if err := s.session.AppendLogToChain(chain, entry); err != nil {
			return err
		}
		s.publishLog(operation, chain, entry)
		return s.persistIfConfigured()
	case "hovel.run.failed", "run.failed":
		entry := s.runEventEntry(operation, chain, evt, operatorlog.Finding("throw", "run failed"))
		if err := s.session.AppendLogToChain(chain, entry); err != nil {
			return err
		}
		s.publishLog(operation, chain, entry)
		return s.persistIfConfigured()
	}
	return nil
}

func (s *publishingEventSink) publishLog(operation, chain string, entry operatorlog.Entry) {
	if s.logs != nil {
		s.logs.Publish(operation, chain, entry)
	}
}

func (s *publishingEventSink) persistIfConfigured() error {
	if s.persist == nil {
		return nil
	}
	return s.persist()
}

func (s *publishingEventSink) moduleLogEntry(operation, chain string, evt event.Event) operatorlog.Entry {
	fields := make([]operatorlog.Field, 0, len(evt.Fields))
	for key, value := range evt.Fields {
		if key == "message" {
			continue
		}
		fields = append(fields, operatorlog.Field{Name: key, Value: value})
	}
	entry := operatorlog.Info("module", evt.Fields["message"], fields...).WithLevel(operatorlog.Level(evt.Level))
	return s.runEventEntry(operation, chain, evt, entry)
}

func (s *publishingEventSink) runEventEntry(operation, chain string, evt event.Event, entry operatorlog.Entry) operatorlog.Entry {
	started := s.throwStarts[chain]
	if started.IsZero() {
		started = s.runStarts[evt.Refs.RunID]
	}
	if started.IsZero() {
		started = evt.Timestamp
	}
	return entry.
		WithElapsed(evt.Timestamp.Sub(started).Seconds()).
		WithChain(chain).
		WithRun(evt.Refs.RunID).
		WithTarget(evt.Refs.TargetID).
		WithModule(evt.Refs.ModuleID).
		WithTopic("operation/" + operation + "/chain/" + chain + "/logs")
}

type systemClock struct{}

func (systemClock) Now() time.Time {
	return time.Now().UTC()
}

func logDaemonRuntimeError(action string, err error) {
	if err != nil && !errors.Is(err, os.ErrNotExist) && !errors.Is(err, net.ErrClosed) {
		log.Printf("hovel daemon runtime: %s: %v", action, err)
	}
}

type runtimeIDs struct{}

var runtimeIDCounter atomic.Uint64

func (runtimeIDs) NewID() string {
	return fmt.Sprintf("id-%d-%d", os.Getpid(), runtimeIDCounter.Add(1))
}
