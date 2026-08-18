package hovel

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"testing"
)

type coverageCommandModule struct{ fakeModule }

func (coverageCommandModule) ListPayloadCommands(PayloadCommandListRequest) ([]PayloadCommand, error) {
	return []PayloadCommand{{Name: "command"}}, nil
}
func (coverageCommandModule) RunPayloadCommand(PayloadCommandRequest) (PayloadCommandResult, error) {
	return PayloadCommandResult{Command: "command"}, nil
}

type coverageCommandErrorModule struct{ coverageCommandModule }

func (coverageCommandErrorModule) ListPayloadCommands(PayloadCommandListRequest) ([]PayloadCommand, error) {
	return nil, errors.New("commands failed")
}

type coverageErrorModule struct{ fakeModule }

func (coverageErrorModule) Run(*Context) (Result, error) { return Result{}, errors.New("run failed") }

type coverageSchemaModule struct{ fakeModule }

func (coverageSchemaModule) Schema() Schema { return Schema{Outputs: map[string]any{"x": true}} }

type coverageBeaconErrorModule struct{ fakeMeshModule }

func (coverageBeaconErrorModule) ListMeshBeacons(MeshBeaconRequest) ([]MeshBeacon, error) {
	return nil, errors.New("beacons failed")
}

type coverageListenerErrorModule struct{ fakeMeshModule }

func (coverageListenerErrorModule) ListMeshListeners(MeshListenerListRequest) ([]MeshListener, error) {
	return nil, errors.New("listeners failed")
}
func (coverageListenerErrorModule) StartMeshListener(MeshListenerStartRequest) (MeshListener, error) {
	return MeshListener{}, errors.New("start failed")
}
func (coverageListenerErrorModule) StopMeshListener(MeshListenerStopRequest) (MeshListener, error) {
	return MeshListener{}, errors.New("stop failed")
}

type coverageRuntimeErrorModule struct{ fakeMeshModule }

func (coverageRuntimeErrorModule) LoadRuntimeCredential(CredentialRuntimeRequest) (CredentialDeliveryReceipt, error) {
	return CredentialDeliveryReceipt{}, errors.New("runtime failed")
}
func (coverageRuntimeErrorModule) LoadCredentialFiles(CredentialFilesRequest) (CredentialDeliveryReceipt, error) {
	return CredentialDeliveryReceipt{}, errors.New("files failed")
}
func (coverageRuntimeErrorModule) EncodeCredentialMaterial(CredentialEncodingRequest) (CredentialEncodingResult, error) {
	return CredentialEncodingResult{}, errors.New("encoding failed")
}
func (coverageRuntimeErrorModule) StampCredential(CredentialStampExecutionRequest) (CredentialStampExecutionResult, error) {
	return CredentialStampExecutionResult{}, errors.New("stamp failed")
}

type coverageTaskErrorModule struct{ fakeMeshModule }

func (coverageTaskErrorModule) RunMeshTask(*MeshContext, MeshTaskRequest) (MeshTaskResult, error) {
	return MeshTaskResult{}, errors.New("task failed")
}

func coverageServer(module Module) *server {
	s := &server{module: module, writer: newFrameWriter(io.Discard)}
	s.sessions = newSessionManager(func(sessionEvent) {})
	return s
}

func TestServerDispatchRejectsUnavailableSurfaces(t *testing.T) {
	s := coverageServer(fakeModule{})
	methods := []string{
		"list_payloads", "resolve_payload", "prepare_listener", "generate_payload", "connect_session", "cleanup_payload", "read_payload_chunk",
		"payload.command.list", "payload.command.run", meshRPCDescribeMethod, meshRPCTopologyMethod, meshRPCBeaconsMethod, meshRPCListenersMethod,
		meshRPCListenerStartMethod, meshRPCListenerStopMethod, meshRPCTaskMethod, meshRPCOpenStreamMethod,
		credentialRPCRuntimeMethod, credentialRPCDescribeMethod, credentialRPCFilesMethod, credentialRPCEncodeMethod, credentialRPCStampMethod,
		"step.describe", "step.prepare", "step.execute", "step.cleanup", "unknown",
	}
	for _, method := range methods {
		if _, err := s.dispatch(method, nil); err == nil {
			t.Errorf("%s unexpectedly available", method)
		}
	}
	if result, err := s.dispatch("shutdown", nil); err != nil || result.(map[string]any)["status"] != "ok" {
		t.Fatalf("shutdown = %#v, %v", result, err)
	}
}

func TestServerMalformedParamsAcrossProviderSurfaces(t *testing.T) {
	bad := json.RawMessage(`{`)
	for _, tc := range []struct {
		module  Module
		methods []string
	}{
		{fakePayloadProvider{}, []string{"list_payloads", "resolve_payload", "prepare_listener", "generate_payload", "connect_session", "cleanup_payload", "read_payload_chunk"}},
		{fakeStepModule{}, []string{"step.prepare", "step.execute", "step.cleanup"}},
		{fakeMeshModule{}, []string{meshRPCDescribeMethod, meshRPCTopologyMethod, meshRPCBeaconsMethod, meshRPCListenersMethod, meshRPCListenerStartMethod, meshRPCListenerStopMethod, meshRPCTaskMethod, meshRPCOpenStreamMethod, credentialRPCRuntimeMethod, credentialRPCFilesMethod, credentialRPCEncodeMethod, credentialRPCStampMethod}},
	} {
		s := coverageServer(tc.module)
		for _, method := range tc.methods {
			if _, err := s.dispatch(method, bad); err == nil {
				t.Errorf("%T %s accepted malformed params", tc.module, method)
			}
		}
	}
	var decoded map[string]any
	if value, err := decodeParams[map[string]any](nil); err != nil || value != nil {
		t.Fatalf("empty decode = %#v, %v", value, err)
	}
	_ = decoded
}

func TestServerPropagatesCredentialProviderFailures(t *testing.T) {
	s := coverageServer(coverageRuntimeErrorModule{})
	for _, method := range []string{
		credentialRPCRuntimeMethod,
		credentialRPCFilesMethod,
		credentialRPCEncodeMethod,
		credentialRPCStampMethod,
	} {
		params, err := json.Marshal(validCredentialProviderParams(method))
		if err != nil {
			t.Fatal(err)
		}
		if _, err := s.dispatch(method, params); err == nil {
			t.Errorf("%s did not propagate its provider failure", method)
		}
	}
}

func TestServerHandshakeSchemaAndHelpers(t *testing.T) {
	for _, info := range []Info{{}, {Name: "name"}, {Name: "name", Version: "v1", Type: "bad"}} {
		if err := validateInfo(info); err == nil {
			t.Fatalf("accepted %#v", info)
		}
	}
	for _, kind := range []ModuleType{TypeSurvey, TypeExploit, TypePayloadProvider} {
		if err := validateInfo(Info{Name: "name", Version: "v1", Type: kind}); err != nil {
			t.Fatal(err)
		}
	}
	s := coverageServer(fakeModule{})
	if _, err := s.handshake(); err != nil {
		t.Fatal(err)
	}
	_ = s.schema()
	if contextPresent(ModuleContext{}) {
		t.Fatal("empty context present")
	}
	if !contextPresent(ModuleContext{Summary: "summary"}) || !contextPresent(ModuleContext{Keywords: []string{"key"}}) || !contextPresent(ModuleContext{Cleanup: "cleanup"}) {
		t.Fatal("populated context absent")
	}
	if got := requirementsToRPC(nil); got == nil || len(got) != 0 {
		t.Fatalf("requirements = %#v", got)
	}
	got := requirementsToRPC([]Requirement{{Key: "key"}})
	if got[0]["type"] != "string" || got[0]["allowed"] == nil {
		t.Fatalf("requirement = %#v", got)
	}
	if stringField(map[string]json.RawMessage{}, "method") != "" || stringField(map[string]json.RawMessage{"method": json.RawMessage(`1`)}, "method") != "" || stringField(map[string]json.RawMessage{"method": json.RawMessage(`"ok"`)}, "method") != "ok" {
		t.Fatal("stringField")
	}
	if len(orEmpty(nil)) != 0 {
		t.Fatal("nil map not normalized")
	}
	original := map[string]any{"x": 1}
	if orEmpty(original)["x"] != 1 {
		t.Fatal("map changed")
	}
	refs := mergeSessionRefs([]SessionRef{{ID: "a"}, {ID: ""}}, []SessionRef{{ID: "a"}, {ID: "b"}, {ID: ""}})
	if len(refs) != 3 || refs[2].ID != "b" {
		t.Fatalf("refs = %#v", refs)
	}
	if _, err := normalizeMeshListenerResult("id", MeshListener{}); err == nil {
		t.Fatal("empty listener accepted")
	}
	if _, err := normalizeMeshListenerResult("id", MeshListener{ID: "other"}); err == nil {
		t.Fatal("mismatch listener accepted")
	}
	if value, err := normalizeMeshListenerResult("id", MeshListener{ID: " id "}); err != nil || value.ID != "id" {
		t.Fatalf("listener = %#v, %v", value, err)
	}
	if _, err := normalizeCredentialReceipt("id", CredentialDeliveryReceipt{}); err == nil {
		t.Fatal("bad receipt accepted")
	}
	if _, err := normalizeCredentialReceipt("id", CredentialDeliveryReceipt{RequestID: "other"}); err == nil {
		t.Fatal("mismatch receipt accepted")
	}
	if _, err := normalizeCredentialReceipt("id", CredentialDeliveryReceipt{RequestID: "id"}); err != nil {
		t.Fatal(err)
	}
}

func framedServerInput(t *testing.T, messages ...map[string]any) []byte {
	t.Helper()
	var result bytes.Buffer
	for _, message := range messages {
		body, err := json.Marshal(message)
		if err != nil {
			t.Fatal(err)
		}
		fmt.Fprintf(&result, "Content-Length: %d\r\n\r\n", len(body))
		result.Write(body)
	}
	return result.Bytes()
}

func TestServeIOControlFlow(t *testing.T) {
	if err := ServeIO(fakeModule{}, bytes.NewReader(nil), &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	if err := ServeIO(fakeModule{}, bytes.NewBufferString("Bad-Header\r\n\r\n"), &bytes.Buffer{}); err == nil {
		t.Fatal("bad frame accepted")
	}
	input := framedServerInput(t,
		map[string]any{"jsonrpc": "2.0"},
		map[string]any{"jsonrpc": "2.0", "method": "cancel"},
		map[string]any{"jsonrpc": "2.0", "id": 1, "method": "shutdown", "params": map[string]any{}},
	)
	var output bytes.Buffer
	if err := ServeIO(fakeModule{}, bytes.NewReader(input), &output); err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(output.Bytes(), []byte(`"status":"ok"`)) {
		t.Fatalf("shutdown output = %s", output.Bytes())
	}
}

func TestServeEntryPointBranches(t *testing.T) {
	originalIO, originalExit := serveModuleIO, serveExit
	t.Cleanup(func() { serveModuleIO, serveExit = originalIO, originalExit })
	exitCode := 0
	serveExit = func(code int) { exitCode = code }
	serveModuleIO = func(Module, io.Reader, io.Writer) error { return nil }
	Serve(fakeModule{})
	if exitCode != 0 {
		t.Fatalf("successful Serve exited with %d", exitCode)
	}
	serveModuleIO = func(Module, io.Reader, io.Writer) error { return errors.New("injected") }
	Serve(fakeModule{})
	if exitCode != 2 {
		t.Fatalf("failed Serve exited with %d", exitCode)
	}
}

func TestServerExecuteAndSessionRPCFailures(t *testing.T) {
	s := coverageServer(fakeModule{})
	if _, err := s.execute(json.RawMessage(`{`)); err == nil {
		t.Fatal("bad execute params accepted")
	}
	if _, err := s.execute(nil); err != nil {
		t.Fatal(err)
	}
	for _, method := range []func(json.RawMessage) (any, error){s.sessionWrite, s.sessionRead, s.sessionClose, s.sessionCommandList, s.sessionCommandRun} {
		if _, err := method(json.RawMessage(`{`)); err == nil {
			t.Fatal("bad session params accepted")
		}
	}
	if _, err := s.sessionWrite(json.RawMessage(`{"sessionId":"missing","data":"bad"}`)); err == nil {
		t.Fatal("bad base64 accepted")
	}
	if _, err := s.sessionWrite(json.RawMessage(`{"sessionId":"missing","data":""}`)); err == nil {
		t.Fatal("missing session write accepted")
	}
	if _, err := s.sessionRead(json.RawMessage(`{"sessionId":"missing","timeoutMs":0}`)); err == nil {
		t.Fatal("missing session read accepted")
	}
	if _, err := s.sessionRead(json.RawMessage(`{"sessionId":"missing","timeoutMs":-1}`)); err == nil {
		t.Fatal("missing blocking session read accepted")
	}
	if _, err := s.sessionClose(json.RawMessage(`{"sessionId":"missing"}`)); err == nil {
		t.Fatal("missing session close accepted")
	}
	if _, err := s.sessionCommandList(json.RawMessage(`{"sessionId":"missing"}`)); err == nil {
		t.Fatal("missing session command list accepted")
	}
	if _, err := s.sessionCommandRun(json.RawMessage(`{"sessionId":"missing"}`)); err == nil {
		t.Fatal("missing session command run accepted")
	}
}

func TestServerLiveSessionRPCs(t *testing.T) {
	s := coverageServer(fakeModule{withSession: true})
	if _, err := s.execute(json.RawMessage(`{"runId":"run","moduleId":"module","target":"target"}`)); err != nil {
		t.Fatal(err)
	}
	refs := s.sessions.refsForRun("run")
	if len(refs) != 1 {
		t.Fatalf("refs = %#v", refs)
	}
	id := refs[0].ID
	write := json.RawMessage(fmt.Sprintf(`{"sessionId":%q,"data":"d2hvYW1pCg=="}`, id))
	if _, err := s.sessionWrite(write); err != nil {
		t.Fatal(err)
	}
	read := json.RawMessage(fmt.Sprintf(`{"sessionId":%q,"timeoutMs":100}`, id))
	if _, err := s.sessionRead(read); err != nil {
		t.Fatal(err)
	}
	list := json.RawMessage(fmt.Sprintf(`{"sessionId":%q,"request":{}}`, id))
	if _, err := s.sessionCommandList(list); err != nil {
		t.Fatal(err)
	}
	run := json.RawMessage(fmt.Sprintf(`{"sessionId":%q,"request":{"command":"session.info"}}`, id))
	if _, err := s.sessionCommandRun(run); err != nil {
		t.Fatal(err)
	}
	closeRequest := json.RawMessage(fmt.Sprintf(`{"sessionId":%q}`, id))
	if _, err := s.sessionClose(closeRequest); err != nil {
		t.Fatal(err)
	}
}

func TestServerRemainingDispatchBranches(t *testing.T) {
	commandServer := coverageServer(coverageCommandModule{})
	if _, err := commandServer.listPayloadCommands(nil); err != nil {
		t.Fatal(err)
	}
	if _, err := commandServer.runPayloadCommand(nil); err != nil {
		t.Fatal(err)
	}
	if _, err := commandServer.listPayloadCommands(json.RawMessage(`{`)); err == nil {
		t.Fatal("bad command list accepted")
	}
	if _, err := commandServer.runPayloadCommand(json.RawMessage(`{`)); err == nil {
		t.Fatal("bad command accepted")
	}
	if _, err := coverageServer(coverageCommandErrorModule{}).listPayloadCommands(nil); err == nil {
		t.Fatal("command provider error lost")
	}
	payloadServer := coverageServer(fakePayloadProvider{})
	if _, err := payloadServer.readPayloadChunk(nil); err != nil {
		t.Fatal(err)
	}
	meshServer := coverageServer(fakeMeshModule{})
	if _, err := meshServer.startMeshListener(json.RawMessage(`{}`)); err == nil {
		t.Fatal("empty start id accepted")
	}
	if _, err := meshServer.stopMeshListener(json.RawMessage(`{}`)); err == nil {
		t.Fatal("empty stop id accepted")
	}
	for _, tc := range []struct {
		s      *server
		method string
		params json.RawMessage
	}{
		{coverageServer(coverageBeaconErrorModule{}), meshRPCBeaconsMethod, json.RawMessage(`{}`)},
		{coverageServer(coverageListenerErrorModule{}), meshRPCListenersMethod, json.RawMessage(`{}`)},
		{coverageServer(coverageListenerErrorModule{}), meshRPCListenerStartMethod, json.RawMessage(`{"listenerId":"id"}`)},
		{coverageServer(coverageListenerErrorModule{}), meshRPCListenerStopMethod, json.RawMessage(`{"listenerId":"id"}`)},
		{coverageServer(coverageTaskErrorModule{}), meshRPCTaskMethod, json.RawMessage(`{}`)},
	} {
		if _, err := tc.s.dispatch(tc.method, tc.params); err == nil {
			t.Errorf("%s provider error lost", tc.method)
		}
	}
	if _, err := coverageServer(coverageErrorModule{}).execute(nil); err == nil {
		t.Fatal("run error lost")
	}
	_ = coverageServer(coverageSchemaModule{}).schema()
	got := requirementsToRPC([]Requirement{{Key: "key", Type: "integer", Allowed: []string{"1"}}})
	if got[0]["type"] != "integer" {
		t.Fatalf("requirements = %#v", got)
	}
}
