package rootcli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/akamensky/argparse"
	"github.com/vibepwners/hovel/internal/adapters/cli"
	"github.com/vibepwners/hovel/internal/adapters/commandmode"
	"github.com/vibepwners/hovel/internal/adapters/daemonlocal"
	"github.com/vibepwners/hovel/internal/adapters/daemonrpc"
	mcpadapter "github.com/vibepwners/hovel/internal/adapters/mcp"
	"github.com/vibepwners/hovel/internal/app/agentintegration"
	"github.com/vibepwners/hovel/internal/app/modulecatalog"
	"github.com/vibepwners/hovel/internal/app/services"
	"github.com/vibepwners/hovel/internal/domain/daemon"
	workspacepath "github.com/vibepwners/hovel/internal/domain/workspace"
	agentinfra "github.com/vibepwners/hovel/internal/infra/agentintegration"
	"github.com/vibepwners/hovel/internal/infra/daemonruntime"
	"github.com/vibepwners/hovel/internal/moduleruntime/pythonrpc"
	"github.com/vibepwners/hovel/internal/version"
)

func Run(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	var daemonOverrides daemonlocal.ClientOverrides
	var err error
	args, daemonOverrides, err = extractDaemonClientFlags(args)
	if err != nil {
		writeRootLine(stderr, err)
		return 2
	}
	args = normalizeLeadingConfig(args)
	if len(args) == 0 || helpRequested(args) && (args[0] == "-h" || args[0] == "--help") {
		parser := newRootParser()
		if helpRequested(args) {
			writeRootText(stdout, parser.Usage(nil))
			return 0
		}
		writeRootText(stderr, parser.Usage("role is required"))
		return 2
	}
	switch args[0] {
	case "command":
		requested, requestErr := daemonClientRequested(args[1:], daemonOverrides)
		if requestErr != nil {
			writeRootLine(stderr, requestErr)
			return 2
		}
		if requested {
			return runDaemonCommand(ctx, args[1:], daemonOverrides, stdout, stderr)
		}
		return commandmode.Run(ctx, args[1:], stdout, stderr)
	case "run":
		return runDaemonCommand(ctx, args[1:], daemonOverrides, stdout, stderr)
	case "cli", "shell":
		options, err := resolveDaemonClientOptions(args[1:], daemonOverrides)
		if err != nil {
			writeRootLine(stderr, err)
			return 2
		}
		return cli.RunWithDaemonOptions(ctx, args[1:], argumentValue(args[1:], "--config"), options, stdout, stderr)
	case "mcp":
		return runMCP(ctx, args[1:], daemonOverrides, stdout, stderr)
	case "agent":
		return runAgent(ctx, args[1:], stdout, stderr)
	case "daemon":
		return runDaemon(ctx, args[1:], daemonOverrides, stdout, stderr)
	case "version":
		return runVersion(args[1:], stdout, stderr)
	case "tui":
		if len(args) > 1 && helpRequested(args[1:]) {
			writeRootText(stdout, newTUIParser().Usage(nil))
			return 0
		}
		writeRootLine(stderr, "hovel tui is not implemented yet")
		return 1
	case "init":
		return commandmode.Run(ctx, append([]string{"control", "init"}, args[1:]...), stdout, stderr)
	case "status":
		statusArgs := append([]string{"control", "daemon", "status"}, args[1:]...)
		requested, requestErr := daemonClientRequested(statusArgs, daemonOverrides)
		if requestErr != nil {
			writeRootLine(stderr, requestErr)
			return 2
		}
		if requested {
			return runDaemonCommand(ctx, statusArgs, daemonOverrides, stdout, stderr)
		}
		return commandmode.Run(ctx, statusArgs, stdout, stderr)
	default:
		if args[0] == "throw" && throwFileArg(args[1:]) != "" {
			return runOneShotThrow(ctx, args, daemonOverrides, stdout, stderr)
		}
		if isDirectSessionConnectCommand(args) {
			return runDirectSessionConnect(ctx, args, daemonOverrides, stdout, stderr)
		}
		if commandmode.NewApp().Registry().HasRoot(args[0]) {
			requested, requestErr := daemonClientRequested(args, daemonOverrides)
			if requestErr != nil {
				writeRootLine(stderr, requestErr)
				return 2
			}
			if requested {
				return runDaemonCommand(ctx, args, daemonOverrides, stdout, stderr)
			}
			return commandmode.Run(ctx, args, stdout, stderr)
		}
		writeRootText(stderr, newRootParser().Usage(fmt.Sprintf("unknown command or role %q", args[0])))
		return 2
	}
}

func daemonClientRequested(args []string, overrides daemonlocal.ClientOverrides) (bool, error) {
	options, err := resolveDaemonClientOptions(args, overrides)
	if err != nil {
		return false, err
	}
	return strings.TrimSpace(options.Endpoint) != "", nil
}

func extractDaemonClientFlags(args []string) ([]string, daemonlocal.ClientOverrides, error) {
	var overrides daemonlocal.ClientOverrides
	out := make([]string, 0, len(args))
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "--" {
			out = append(out, args[i:]...)
			break
		}
		switch {
		case arg == "--daemon-endpoint":
			if i+1 >= len(args) {
				return nil, overrides, errors.New("--daemon-endpoint requires a value")
			}
			i++
			overrides.Endpoint = args[i]
		case strings.HasPrefix(arg, "--daemon-endpoint="):
			overrides.Endpoint = strings.TrimPrefix(arg, "--daemon-endpoint=")
		case arg == "--daemon-connect-timeout":
			if i+1 >= len(args) {
				return nil, overrides, errors.New("--daemon-connect-timeout requires a value")
			}
			i++
			overrides.ConnectTimeout = args[i]
		case strings.HasPrefix(arg, "--daemon-connect-timeout="):
			overrides.ConnectTimeout = strings.TrimPrefix(arg, "--daemon-connect-timeout=")
		case arg == "--allow-insecure-daemon":
			overrides.AllowInsecure = true
			overrides.AllowInsecureSet = true
		default:
			out = append(out, arg)
		}
	}
	return out, overrides, nil
}

func resolveDaemonClientOptions(args []string, overrides daemonlocal.ClientOverrides) (daemonlocal.ClientOptions, error) {
	return daemonlocal.ResolveClientOptions(argumentValue(args, "--workspace", "-w"), argumentValue(args, "--config"), overrides)
}

func argumentValue(args []string, names ...string) string {
	for i, arg := range args {
		for _, name := range names {
			if arg == name && i+1 < len(args) {
				return args[i+1]
			}
			if strings.HasPrefix(arg, name+"=") {
				return strings.TrimPrefix(arg, name+"=")
			}
		}
	}
	return ""
}

func normalizeLeadingConfig(args []string) []string {
	if len(args) < 2 {
		return args
	}
	switch {
	case args[0] == "--config":
		if len(args) < 3 {
			return args
		}
		if args[2] == "run" {
			out := []string{"run", "--config", args[1]}
			return append(out, args[3:]...)
		}
		out := append([]string(nil), args[2:]...)
		return append(out, "--config", args[1])
	case strings.HasPrefix(args[0], "--config="):
		if len(args) > 1 && args[1] == "run" {
			out := []string{"run", "--config", strings.TrimPrefix(args[0], "--config=")}
			return append(out, args[2:]...)
		}
		out := append([]string(nil), args[1:]...)
		return append(out, "--config", strings.TrimPrefix(args[0], "--config="))
	default:
		return args
	}
}

func isDirectSessionConnectCommand(args []string) bool {
	if len(args) < 2 {
		return false
	}
	if args[0] != "session" && args[0] != "sessions" {
		return false
	}
	if args[1] != "connect" {
		return false
	}
	return !cli.SessionConnectHelpRequested(args[2:])
}

func runDirectSessionConnect(ctx context.Context, args []string, overrides daemonlocal.ClientOverrides, stdout, stderr io.Writer) int {
	if ok, code := commandmode.NewApp().Validate(args, stderr); !ok {
		return code
	}
	parsed, err := cli.ParseSessionConnectCommand(args)
	if err != nil {
		writeRootLine(stderr, err)
		return 2
	}
	workspacePath := workspacepath.ResolvePath(parsed.Workspace)
	options, err := daemonlocal.ResolveClientOptions(workspacePath, "", overrides)
	if err != nil {
		writeRootLine(stderr, err)
		return 1
	}
	if strings.TrimSpace(options.Endpoint) == "" {
		status, statusErr := daemonlocal.NewManager().Daemons.Status(ctx, services.DaemonStatusRequest{WorkspacePath: workspacePath})
		if statusErr != nil {
			writeRootLine(stderr, statusErr)
			return 1
		}
		if status.State != daemon.StateRunning {
			writeRootLine(stderr, "daemon is not running for workspace "+status.WorkspacePath)
			return 1
		}
		client, dialErr := daemonrpc.Dial(status.Identity.SocketPath)
		if dialErr != nil {
			writeRootLine(stderr, dialErr)
			return 1
		}
		defer func() { logRootError("close daemon rpc client", client.Close()) }()
		return cli.RunSessionConnect(ctx, client, parsed.SessionID, parsed.Options, stdout, stderr)
	}
	session, client, err := daemonlocal.Connect(ctx, workspacePath, "", "", options, true)
	if err != nil {
		writeRootLine(stderr, err)
		return 1
	}
	defer func() { logRootError("close daemon manager session", session.Close()) }()
	defer func() { logRootError("close daemon rpc client", client.Close()) }()
	return cli.RunSessionConnect(ctx, client, parsed.SessionID, parsed.Options, stdout, stderr)
}

func runOneShotThrow(ctx context.Context, args []string, overrides daemonlocal.ClientOverrides, stdout, stderr io.Writer) int {
	if ok, code := commandmode.NewApp().Validate(args, stderr); !ok {
		return code
	}
	requested, err := daemonClientRequested(args, overrides)
	if err != nil {
		writeRootLine(stderr, err)
		return 2
	}
	if requested {
		return runDaemonCommand(ctx, args, overrides, stdout, stderr)
	}
	session, err := daemonlocal.NewManager().Ensure(ctx, throwWorkspaceArg(args[1:]))
	if err != nil {
		writeRootLine(stderr, err)
		return 1
	}
	defer func() { logRootError("close daemon manager session", session.Close()) }()
	return commandmode.Run(ctx, args, stdout, stderr)
}

func runDaemon(ctx context.Context, args []string, overrides daemonlocal.ClientOverrides, stdout, stderr io.Writer) int {
	if len(args) == 0 || helpRequested(args) && (args[0] == "-h" || args[0] == "--help") {
		parser := newDaemonParser()
		if helpRequested(args) {
			writeRootText(stdout, parser.Usage(nil))
			return 0
		}
		writeRootText(stderr, parser.Usage("daemon command is required"))
		return 2
	}
	switch args[0] {
	case "serve":
		return runDaemonServe(ctx, args[1:], stdout, stderr)
	case "status":
		statusArgs := append([]string{"control", "daemon", "status"}, args[1:]...)
		requested, requestErr := daemonClientRequested(statusArgs, overrides)
		if requestErr != nil {
			writeRootLine(stderr, requestErr)
			return 2
		}
		if requested {
			return runDaemonCommand(ctx, statusArgs, overrides, stdout, stderr)
		}
		return commandmode.Run(ctx, statusArgs, stdout, stderr)
	default:
		writeRootText(stderr, newDaemonParser().Usage(fmt.Sprintf("unknown daemon command %q", args[0])))
		return 2
	}
}

func runDaemonServe(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	parser := argparse.NewParser("hovel daemon serve", "Run the daemon role in the mono-binary.")
	workspacePath := parser.String("w", "workspace", &argparse.Options{Help: "Workspace path"})
	socketPath := parser.String("s", "socket", &argparse.Options{Help: "Local RPC socket path"})
	listenAddresses := parser.StringList("", "listen", &argparse.Options{Help: "RPC listen endpoint; repeat for multiple listeners"})
	advertiseAddresses := parser.StringList("", "advertise", &argparse.Options{Help: "Advertised endpoint mapping as <bind>=<endpoint>; repeat as needed"})
	allowInsecureTCP := parser.Flag("", "allow-insecure-tcp", &argparse.Options{Help: "Enable unauthenticated, unencrypted full control on configured TCP listeners"})
	configPath := parser.String("", "config", &argparse.Options{Help: "Hovel config file path"})
	moduleConfig := parser.String("", "module-config", &argparse.Options{Help: "Module launch catalog path"})
	if ok, code := parseArgs(parser, args, stdout, stderr); !ok {
		return code
	}
	if *socketPath != "" && (len(*listenAddresses) != 0 || len(*advertiseAddresses) != 0) {
		writeRootLine(stderr, "--socket cannot be combined with --listen or --advertise")
		return 2
	}
	var listeners []daemonruntime.ListenerSpec
	var err error
	if *socketPath != "" {
		listeners = []daemonruntime.ListenerSpec{{Bind: *socketPath}}
	} else {
		listeners, err = daemonListenerSpecs(*listenAddresses, *advertiseAddresses, *allowInsecureTCP)
	}
	if err != nil {
		writeRootLine(stderr, err)
		return 2
	}

	writeRootFormat(stdout, "serving hoveld role for workspace %s\n", displayWorkspace(*workspacePath))
	if err := daemonlocal.Serve(ctx, daemonruntime.Args{
		WorkspacePath:    *workspacePath,
		Listeners:        listeners,
		AllowInsecureTCP: *allowInsecureTCP,
		ModuleConfig:     *moduleConfig,
		HovelConfig:      *configPath,
	}); err != nil {
		if errors.Is(err, context.Canceled) {
			return 0
		}
		writeRootLine(stderr, err)
		return 1
	}
	return 0
}

func daemonListenerSpecs(listen, advertise []string, allowInsecure bool) ([]daemonruntime.ListenerSpec, error) {
	if len(listen) == 0 {
		if len(advertise) != 0 {
			return nil, errors.New("--advertise requires at least one --listen endpoint")
		}
		value := strings.TrimSpace(os.Getenv("HOVEL_DAEMON_LISTENERS"))
		if value == "" {
			return nil, nil
		}
		var configured []struct {
			Bind      string `json:"bind"`
			Advertise string `json:"advertise"`
			Access    string `json:"access"`
		}
		if err := json.Unmarshal([]byte(value), &configured); err != nil {
			return nil, fmt.Errorf("invalid HOVEL_DAEMON_LISTENERS: %w", err)
		}
		out := make([]daemonruntime.ListenerSpec, 0, len(configured))
		for _, listener := range configured {
			out = append(out, daemonruntime.ListenerSpec{Bind: listener.Bind, Advertise: listener.Advertise, Access: listener.Access})
		}
		return out, nil
	}
	out := make([]daemonruntime.ListenerSpec, 0, len(listen))
	byBind := make(map[string]string, len(advertise))
	for _, mapping := range advertise {
		bind, endpoint, ok := strings.Cut(mapping, "=")
		if !ok || strings.TrimSpace(bind) == "" || strings.TrimSpace(endpoint) == "" {
			return nil, fmt.Errorf("invalid --advertise %q; use <bind>=<endpoint>", mapping)
		}
		byBind[strings.TrimSpace(bind)] = strings.TrimSpace(endpoint)
	}
	for _, bind := range listen {
		bind = strings.TrimSpace(bind)
		access := ""
		if allowInsecure {
			endpoint, err := daemonrpc.ParseEndpoint(bind)
			if err != nil {
				return nil, err
			}
			if endpoint.Network == "tcp" {
				access = "insecure-full"
			}
		}
		out = append(out, daemonruntime.ListenerSpec{Bind: bind, Advertise: byBind[bind], Access: access})
		delete(byBind, bind)
	}
	if len(byBind) != 0 {
		return nil, errors.New("--advertise mapping does not match a --listen endpoint")
	}
	return out, nil
}

func newRootParser() *argparse.Parser {
	parser := argparse.NewParser("hovel", "Hovel operator console.")
	parser.String("", "daemon-endpoint", &argparse.Options{Help: "Explicit daemon endpoint (Unix socket or tcp://host:port)"})
	parser.String("", "daemon-connect-timeout", &argparse.Options{Help: "Daemon connection timeout, such as 2s"})
	parser.Flag("", "allow-insecure-daemon", &argparse.Options{Help: "Acknowledge unencrypted, unauthenticated full-control TCP"})
	for _, definition := range commandmode.NewApp().Registry().FirstSegments() {
		parser.NewCommand(definition.Path[0], definition.Summary)
	}
	parser.NewCommand("init", "Initialize a workspace.")
	parser.NewCommand("status", "Inspect workspace and daemon status.")
	agent := parser.NewCommand("agent", "Install agent-host integrations.")
	agent.NewCommand("install", "Install Hovel skills and MCP configuration for an agent host.")
	for _, role := range []struct {
		name    string
		summary string
	}{
		{"shell", "Launch the interactive prompt shell."},
		{"command", "Run one command from the shell. Compatibility alias for direct commands."},
		{"run", "Run one command against a daemon-backed operator session."},
		{"cli", "Launch the interactive prompt shell. Alias for shell."},
		{"mcp", "Launch the MCP agent interface."},
		{"version", "Print the Hovel version."},
	} {
		parser.NewCommand(role.name, role.summary)
	}
	daemon := parser.NewCommand("daemon", "Run or inspect the daemon role.")
	daemon.NewCommand("serve", "Run the daemon role.")
	daemon.NewCommand("status", "Inspect daemon status.")
	parser.NewCommand("tui", "Launch the terminal UI.")
	return parser
}

func runVersion(args []string, stdout, stderr io.Writer) int {
	switch {
	case len(args) == 0:
		writeRootLine(stdout, "version "+version.Version)
		return 0
	case len(args) == 1 && (args[0] == "-h" || args[0] == "--help"):
		writeRootText(stdout, "Usage: hovel version\n\nPrint the Hovel version.\n")
		return 0
	default:
		writeRootText(stderr, "Usage: hovel version\n\nversion does not take arguments\n")
		return 2
	}
}

type runCommandArgs struct {
	Workspace string
	Config    string
	Operation string
	Chain     string
	Command   []string
}

func runDaemonCommand(ctx context.Context, args []string, overrides daemonlocal.ClientOverrides, stdout, stderr io.Writer) int {
	parsed, ok, code := parseRunCommandArgs(args, stdout, stderr)
	if !ok {
		return code
	}
	commandArgs := injectWorkspaceForDaemonCommand(normalizeRunCommand(parsed.Command), parsed.Workspace)
	commandArgs = injectConfigForDaemonCommand(commandArgs, parsed.Config)
	if valid, validationCode := commandmode.NewApp().Validate(commandArgs, stderr); !valid {
		return validationCode
	}
	options, err := daemonlocal.ResolveClientOptions(parsed.Workspace, parsed.Config, overrides)
	if err != nil {
		writeRootLine(stderr, err)
		return 1
	}
	requireFull := !isDaemonStatusCommand(commandArgs)
	session, client, err := daemonlocal.Connect(ctx, parsed.Workspace, "", parsed.Config, options, requireFull)
	if err != nil {
		writeRootLine(stderr, err)
		return 1
	}
	defer func() { logRootError("close daemon manager session", session.Close()) }()

	defer func() { logRootError("close daemon rpc client", client.Close()) }()

	operatorSession := daemonrpc.NewSessionClient(ctx, client)
	if parsed.Operation != "" {
		if err := operatorSession.UseOperation(parsed.Operation); err != nil {
			writeRootLine(stderr, err)
			return 1
		}
	}
	if parsed.Chain != "" {
		if err := operatorSession.UseChain(parsed.Chain); err != nil {
			writeRootLine(stderr, err)
			return 1
		}
	}
	attached := strings.TrimSpace(options.Endpoint) != ""
	var catalog modulecatalog.Catalog
	if attached {
		catalog, err = client.GetModuleCatalog(ctx)
		if err != nil {
			writeRootFormat(stderr, "hovel: failed to load daemon module catalog: %v\n", err)
			catalog = modulecatalog.New()
		}
	} else {
		catalog, err = (pythonrpc.Runner{WorkspacePath: parsed.Workspace, HovelConfig: parsed.Config}).Catalog(ctx)
		if err != nil {
			writeRootFormat(stderr, "hovel: failed to load module catalog: %v\n", err)
			catalog = modulecatalog.New()
		}
	}
	app := commandmode.NewAppWithSessionModulesAndWorkspace(operatorSession, catalog, parsed.Workspace)
	if attached {
		app = commandmode.NewAppWithAttachedDaemon(operatorSession, catalog, parsed.Workspace, session.Status(), client)
	}
	return app.Run(ctx, commandArgs, stdout, stderr)
}

func isDaemonStatusCommand(args []string) bool {
	return len(args) >= 3 && args[0] == "control" && args[1] == "daemon" && args[2] == "status"
}

func parseRunCommandArgs(args []string, stdout, stderr io.Writer) (runCommandArgs, bool, int) {
	if len(args) == 0 || helpRequested(args) && (args[0] == "-h" || args[0] == "--help") {
		parser := newRunParser()
		if helpRequested(args) {
			writeRootText(stdout, parser.Usage(nil))
			return runCommandArgs{}, false, 0
		}
		writeRootText(stderr, parser.Usage("command is required"))
		return runCommandArgs{}, false, 2
	}
	parsed := runCommandArgs{Workspace: workspacepath.ResolvePath("")}
	for len(args) > 0 {
		arg := args[0]
		switch {
		case arg == "--":
			args = args[1:]
			parsed.Command = append([]string(nil), args...)
			return validateRunCommandArgs(parsed, stderr)
		case arg == "--workspace" || arg == "-w":
			if len(args) < 2 {
				writeRootText(stderr, newRunParser().Usage(arg+" requires a value"))
				return runCommandArgs{}, false, 2
			}
			parsed.Workspace = args[1]
			args = args[2:]
		case strings.HasPrefix(arg, "--workspace="):
			parsed.Workspace = strings.TrimPrefix(arg, "--workspace=")
			args = args[1:]
		case arg == "--config":
			if len(args) < 2 {
				writeRootText(stderr, newRunParser().Usage(arg+" requires a value"))
				return runCommandArgs{}, false, 2
			}
			parsed.Config = args[1]
			args = args[2:]
		case strings.HasPrefix(arg, "--config="):
			parsed.Config = strings.TrimPrefix(arg, "--config=")
			args = args[1:]
		case arg == "--op" || arg == "--operation":
			if len(args) < 2 {
				writeRootText(stderr, newRunParser().Usage(arg+" requires a value"))
				return runCommandArgs{}, false, 2
			}
			parsed.Operation = args[1]
			args = args[2:]
		case strings.HasPrefix(arg, "--op="):
			parsed.Operation = strings.TrimPrefix(arg, "--op=")
			args = args[1:]
		case strings.HasPrefix(arg, "--operation="):
			parsed.Operation = strings.TrimPrefix(arg, "--operation=")
			args = args[1:]
		case arg == "--chain" || arg == "-c":
			if len(args) < 2 {
				writeRootText(stderr, newRunParser().Usage(arg+" requires a value"))
				return runCommandArgs{}, false, 2
			}
			parsed.Chain = args[1]
			args = args[2:]
		case strings.HasPrefix(arg, "--chain="):
			parsed.Chain = strings.TrimPrefix(arg, "--chain=")
			args = args[1:]
		case strings.HasPrefix(arg, "-"):
			writeRootText(stderr, newRunParser().Usage(fmt.Sprintf("unknown run option %q", arg)))
			return runCommandArgs{}, false, 2
		default:
			parsed.Command = append([]string(nil), args...)
			return validateRunCommandArgs(parsed, stderr)
		}
	}
	return validateRunCommandArgs(parsed, stderr)
}

func validateRunCommandArgs(parsed runCommandArgs, stderr io.Writer) (runCommandArgs, bool, int) {
	if len(parsed.Command) == 0 {
		writeRootText(stderr, newRunParser().Usage("command is required"))
		return runCommandArgs{}, false, 2
	}
	return parsed, true, 0
}

func newRunParser() *argparse.Parser {
	parser := argparse.NewParser("hovel run", "Run one command against a daemon-backed operator session.")
	parser.String("w", "workspace", &argparse.Options{Help: "Workspace path"})
	parser.String("", "config", &argparse.Options{Help: "Hovel config file path"})
	parser.String("", "op", &argparse.Options{Help: "Operation context to select before running the command"})
	parser.String("", "operation", &argparse.Options{Help: "Operation context to select before running the command"})
	parser.String("c", "chain", &argparse.Options{Help: "Chain context to select before running the command"})
	parser.NewCommand("<command>", "Command and arguments to run in the selected context.")
	return parser
}

func injectWorkspaceForDaemonCommand(args []string, workspace string) []string {
	if workspace == "" || hasWorkspaceArg(args) || !commandUsesWorkspace(args) {
		return append([]string(nil), args...)
	}
	return insertRunInjectedOption(args, "--workspace", workspace)
}

func injectConfigForDaemonCommand(args []string, configPath string) []string {
	configPath = strings.TrimSpace(configPath)
	if configPath == "" || hasConfigArg(args) {
		return append([]string(nil), args...)
	}
	return insertRunInjectedOption(args, "--config", configPath)
}

func insertRunInjectedOption(args []string, name, value string) []string {
	delimiter := passthroughDelimiterIndex(args)
	if delimiter < 0 {
		out := append([]string(nil), args...)
		return append(out, name, value)
	}
	out := make([]string, 0, len(args)+2)
	out = append(out, args[:delimiter]...)
	out = append(out, name, value)
	out = append(out, args[delimiter:]...)
	return out
}

func normalizeRunCommand(args []string) []string {
	if len(args) == 0 {
		return nil
	}
	switch args[0] {
	case "add", "config", "inspect", "logs", "validate":
		out := make([]string, 0, len(args)+1)
		out = append(out, "chain")
		out = append(out, args...)
		return out
	default:
		return append([]string(nil), args...)
	}
}

func hasWorkspaceArg(args []string) bool {
	args = argsBeforePassthrough(args)
	for i, arg := range args {
		if arg == "--workspace" || arg == "-w" {
			return i+1 < len(args)
		}
		if strings.HasPrefix(arg, "--workspace=") {
			return true
		}
	}
	return false
}

func hasConfigArg(args []string) bool {
	args = argsBeforePassthrough(args)
	for i, arg := range args {
		if arg == "--config" {
			return i+1 < len(args)
		}
		if strings.HasPrefix(arg, "--config=") {
			return true
		}
	}
	return false
}

func argsBeforePassthrough(args []string) []string {
	delimiter := passthroughDelimiterIndex(args)
	if delimiter < 0 {
		return args
	}
	return args[:delimiter]
}

func passthroughDelimiterIndex(args []string) int {
	for i, arg := range args {
		if arg == "--" {
			return i
		}
	}
	return -1
}

func commandUsesWorkspace(args []string) bool {
	if len(args) == 0 {
		return false
	}
	switch args[0] {
	case "throw", "throws", "confirm", "review", "artifact", "artifacts", "module", "modules", "session", "sessions":
		return true
	default:
		return false
	}
}

func newDaemonParser() *argparse.Parser {
	parser := argparse.NewParser("hovel daemon", "Run or inspect the daemon role.")
	parser.NewCommand("serve", "Run the daemon role.")
	parser.NewCommand("status", "Inspect daemon status.")
	return parser
}

func runMCP(ctx context.Context, args []string, overrides daemonlocal.ClientOverrides, stdout, stderr io.Writer) int {
	parser := newMCPParser()
	workspacePath := parser.String("w", "workspace", &argparse.Options{Help: "Workspace path"})
	operation := parser.String("", "op", &argparse.Options{Help: "Operation context for this MCP operator"})
	operationAlias := parser.String("", "operation", &argparse.Options{Help: "Alias for --op"})
	chain := parser.String("c", "chain", &argparse.Options{Help: "Chain context for this MCP operator"})
	entityID := parser.String("", "entity-id", &argparse.Options{Help: "Stable operator entity ID for launch-key approvals"})
	displayName := parser.String("", "display-name", &argparse.Options{Help: "Human-readable operator entity name"})
	configPath := parser.String("", "config", &argparse.Options{Help: "Hovel config file path"})
	moduleConfig := parser.String("", "module-config", &argparse.Options{Help: "Module launch catalog path for MCP tools"})
	transport := parser.String("", "transport", &argparse.Options{Help: "MCP transport: stdio or http (default stdio)"})
	httpAddr := parser.String("", "http-addr", &argparse.Options{Help: "HTTP MCP listen address when --transport=http (default 127.0.0.1:0)"})
	if ok, code := parseArgs(parser, args, stdout, stderr); !ok {
		return code
	}
	selectedTransport := strings.ToLower(strings.TrimSpace(*transport))
	if selectedTransport == "" {
		selectedTransport = mcpadapter.DefaultTransportMode
	}
	if selectedTransport != "stdio" && selectedTransport != "http" {
		writeRootFormat(stderr, "unsupported MCP transport %q; use stdio or http\n", *transport)
		return 2
	}
	selectedOperation := *operation
	if selectedOperation == "" {
		selectedOperation = *operationAlias
	}
	clientOptions, err := daemonlocal.ResolveClientOptions(*workspacePath, *configPath, overrides)
	if err != nil {
		writeRootLine(stderr, err)
		return 2
	}
	if err := mcpadapter.Run(ctx, mcpadapter.Config{
		Workspace:     *workspacePath,
		Operation:     selectedOperation,
		Chain:         *chain,
		EntityID:      *entityID,
		DisplayName:   *displayName,
		CatalogPath:   *moduleConfig,
		Output:        stdout,
		Status:        stderr,
		TransportMode: selectedTransport,
		HTTPAddr:      *httpAddr,
		HovelConfig:   *configPath,
		DaemonOptions: clientOptions,
	}); err != nil {
		if errors.Is(err, context.Canceled) {
			return 0
		}
		writeRootLine(stderr, err)
		return 1
	}
	return 0
}

func newMCPParser() *argparse.Parser {
	return argparse.NewParser("hovel mcp", "Launch the MCP agent interface.")
}

func runAgent(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 || args[0] != "install" {
		parser := newAgentParser()
		if helpRequested(args) {
			writeRootText(stdout, parser.Usage(nil))
			return 0
		}
		message := "agent command is required"
		if len(args) > 0 {
			message = fmt.Sprintf("unknown agent command %q", args[0])
		}
		writeRootText(stderr, parser.Usage(message))
		return 2
	}
	parser := argparse.NewParser("hovel agent install", "Install Hovel skills and MCP configuration for an agent host.")
	host := parser.StringPositional(&argparse.Options{Required: true, Help: "Agent host: claude, codex, or opencode"})
	scope := parser.Selector("", "scope", []string{"user", "project"}, &argparse.Options{Default: "user", Help: "Installation scope"})
	integrationVersion := parser.String("", "version", &argparse.Options{Help: "Hovel integration version (defaults to this binary's version)"})
	source := parser.String("", "source", &argparse.Options{Help: "Local package archive or extracted package directory"})
	dryRun := parser.Flag("", "dry-run", &argparse.Options{Help: "Print intended actions without changing files or invoking host CLIs"})
	force := parser.Flag("", "force", &argparse.Options{Help: "Replace conflicting Hovel-owned integration entries after preserving a backup"})
	if ok, code := parseArgs(parser, args[1:], stdout, stderr); !ok {
		return code
	}
	service := agentintegration.Service{Installer: agentinfra.Installer{}}
	err := service.Install(ctx, agentintegration.InstallRequest{
		Host:    agentintegration.Host(*host),
		Scope:   agentintegration.Scope(*scope),
		Version: *integrationVersion,
		Source:  *source,
		DryRun:  *dryRun,
		Force:   *force,
	}, stdout)
	if err != nil {
		writeRootLine(stderr, err)
		return 1
	}
	return 0
}

func newAgentParser() *argparse.Parser {
	parser := argparse.NewParser("hovel agent", "Install agent-host integrations.")
	parser.NewCommand("install", "Install Hovel skills and MCP configuration for an agent host.")
	return parser
}

func newTUIParser() *argparse.Parser {
	return argparse.NewParser("hovel tui", "Launch the terminal UI. This role is not implemented yet.")
}

func parseArgs(parser *argparse.Parser, args []string, stdout, stderr io.Writer) (bool, int) {
	parser.ExitOnHelp(false)
	if helpRequested(args) {
		writeRootText(stdout, parser.Usage(nil))
		return false, 0
	}
	if err := parser.Parse(append([]string{"hovel"}, args...)); err != nil {
		writeRootText(stderr, parser.Usage(err))
		return false, 2
	}
	return true, 0
}

func helpRequested(args []string) bool {
	for _, arg := range args {
		if arg == "-h" || arg == "--help" {
			return true
		}
	}
	return false
}

func throwFileArg(args []string) string {
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "list" || arg == "inspect":
			return ""
		case arg == "--workspace" || arg == "-w" || arg == "--chain" || arg == "-c" || arg == "--target" || arg == "-t":
			i++
		case strings.HasPrefix(arg, "--workspace=") || strings.HasPrefix(arg, "--chain=") || strings.HasPrefix(arg, "--target="):
		case strings.HasPrefix(arg, "-"):
		default:
			return arg
		}
	}
	return ""
}

func throwWorkspaceArg(args []string) string {
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "--workspace" || arg == "-w":
			if i+1 < len(args) {
				return args[i+1]
			}
		case strings.HasPrefix(arg, "--workspace="):
			return strings.TrimPrefix(arg, "--workspace=")
		}
	}
	return workspacepath.ResolvePath("")
}

func displayWorkspace(path string) string {
	return workspacepath.ResolvePath(path)
}
