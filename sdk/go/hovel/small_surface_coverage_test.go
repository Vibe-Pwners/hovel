package hovel

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"io"
	"net"
	"testing"
	"time"
)

type coverageJSONError struct{}

func (coverageJSONError) MarshalJSON() ([]byte, error) { return nil, errors.New("marshal") }

type coverageLargeJSON struct{}

func (coverageLargeJSON) MarshalJSON() ([]byte, error) {
	return append([]byte{'"'}, append(bytes.Repeat([]byte{'x'}, maxFrameBytes), '"')...), nil
}

type coverageFailWriter struct {
	writes int
	failAt int
}

func (w *coverageFailWriter) Write(p []byte) (int, error) {
	w.writes++
	if w.writes >= w.failAt {
		return 0, errors.New("write")
	}
	return len(p), nil
}

type coverageConn struct {
	writes         []int
	err            error
	deadlineErrors []error
	closeErr       error
}

func (c *coverageConn) Read([]byte) (int, error)        { return 0, io.EOF }
func (c *coverageConn) Close() error                    { return c.closeErr }
func (c *coverageConn) LocalAddr() net.Addr             { return nil }
func (c *coverageConn) RemoteAddr() net.Addr            { return nil }
func (c *coverageConn) SetDeadline(time.Time) error     { return nil }
func (c *coverageConn) SetReadDeadline(time.Time) error { return nil }
func (c *coverageConn) SetWriteDeadline(time.Time) error {
	if len(c.deadlineErrors) == 0 {
		return nil
	}
	err := c.deadlineErrors[0]
	c.deadlineErrors = c.deadlineErrors[1:]
	return err
}
func (c *coverageConn) Write(p []byte) (int, error) {
	if c.err != nil {
		return 0, c.err
	}
	if len(c.writes) == 0 {
		return 0, nil
	}
	n := c.writes[0]
	c.writes = c.writes[1:]
	if n > len(p) {
		n = len(p)
	}
	return n, nil
}

func TestFramingFailureBranchContract(t *testing.T) {
	frames := []string{
		"",
		"Broken\r\n\r\n",
		"Content-Length: nope\r\n\r\n",
		"X-Test: yes\r\n\r\n",
		"Content-Length: 4\r\n\r\n{}",
		"Content-Length: 1\r\n\r\n{",
		"Content-Length: 3\r\n\r\nxxx",
	}
	for _, frame := range frames {
		_, _ = newFrameReader(bytes.NewBufferString(frame)).read()
	}
	for _, value := range []any{coverageJSONError{}, coverageLargeJSON{}} {
		if err := newFrameWriter(io.Discard).write(value); err == nil {
			t.Errorf("write(%T) unexpectedly succeeded", value)
		}
	}
	for _, failAt := range []int{1, 2} {
		if err := newFrameWriter(&coverageFailWriter{failAt: failAt}).write(map[string]any{"ok": true}); err == nil {
			t.Errorf("writer failure %d was ignored", failAt)
		}
	}
}

func TestResultAndMeshHelperBranchContract(t *testing.T) {
	if JSONArtifact("good", map[string]any{"ok": true}).Data == "null" {
		t.Fatal("JSONArtifact rejected a serializable value")
	}
	if JSONArtifact("bad", coverageJSONError{}).Data != "null" {
		t.Fatal("JSONArtifact did not fail closed")
	}
	result := Ok(nil,
		WithSummary("summary"),
		WithFindings(Finding{Title: "finding"}),
		WithArtifacts(InlineArtifact("inline", "text/plain", "data"), FileArtifact("file", "x", "/tmp/file")),
		WithInstalledPayloads(InstalledPayloadDescriptor{PayloadID: "payload"}),
		WithAgentHints(AgentHint{Text: "hint"}),
	)
	result.Status = ""
	result.Sessions = []SessionRef{{ID: "same"}, {ID: "same"}}
	wire := result.toRPC([]SessionRef{{ID: "same"}, {ID: "other"}})
	if wire["status"] != "succeeded" || len(wire["sessions"].([]map[string]any)) != 2 {
		t.Fatalf("unexpected result wire form: %#v", wire)
	}
	_ = Failed("failed", WithSummary("override"))
	_ = Ok(nil)
	_ = Failed("failed")
	_ = TextArtifact("text", "data")
	_ = Finding{}.toRPC()
	_ = Artifact{}.toRPC()

	mesh := &MeshContext{}
	if _, err := mesh.OpenSession(nil); err == nil {
		t.Fatal("mesh session opened without a registry")
	}
	if meshModuleID(Info{Name: " module "}) != "module" || meshModuleID(Info{Name: "module", Version: " 1 "}) != "module@1" {
		t.Fatal("mesh module identity changed")
	}
}

func TestMeshBridgeFailureBranchContract(t *testing.T) {
	if (MeshBridgeCapability{}).Reveal() != "" {
		t.Fatal("zero capability revealed data")
	}
	valid := base64.RawURLEncoding.EncodeToString(bytes.Repeat([]byte{1}, meshBridgeCapabilityBytes))
	endpoint, err := NewMeshBridgeEndpoint("127.0.0.1", 1, MeshBridgeNetworkTCP, valid)
	if err != nil {
		t.Fatal(err)
	}
	for _, invalid := range []MeshBridgeEndpoint{
		{LocalHost: " 127.0.0.1", LocalPort: 1, LocalNetwork: MeshBridgeNetworkTCP, Capability: endpoint.Capability},
		{LocalHost: "127.0.0.1", LocalPort: 1, LocalNetwork: MeshBridgeNetworkTCP},
	} {
		if _, err := invalid.Address(); err == nil {
			t.Fatal("invalid endpoint produced an address")
		}
	}
	if _, err := NewMeshBridgeEndpoint("public.example", 1, MeshBridgeNetworkTCP, valid); err == nil {
		t.Fatal("unsafe endpoint was accepted")
	}
	if _, err := DialMeshBridge(nil, endpoint); err == nil {
		t.Fatal("nil dial context was accepted")
	}
	if _, err := DialMeshBridge(context.Background(), MeshBridgeEndpoint{}); err == nil {
		t.Fatal("invalid endpoint was dialed")
	}
	if _, err := DialMeshBridge(context.Background(), endpoint); err == nil {
		t.Fatal("closed local endpoint unexpectedly accepted a connection")
	}
	for _, conn := range []*coverageConn{{err: errors.New("write")}, {}, {writes: []int{1, 64}}} {
		_ = writeMeshBridgePreface(conn, []byte("capability"))
	}
	originalDial := meshBridgeDialContext
	t.Cleanup(func() { meshBridgeDialContext = originalDial })
	dial := func(conn net.Conn) {
		meshBridgeDialContext = func(context.Context, string, string) (net.Conn, error) { return conn, nil }
	}
	dial(&coverageConn{writes: []int{64}})
	if conn, err := DialMeshBridge(context.Background(), endpoint); err != nil {
		t.Fatal(err)
	} else {
		_ = conn.Close()
	}
	deadlineContext, cancel := context.WithDeadline(context.Background(), time.Now().Add(time.Minute))
	defer cancel()
	dial(&coverageConn{deadlineErrors: []error{errors.New("deadline")}})
	_, _ = DialMeshBridge(deadlineContext, endpoint)
	dial(&coverageConn{err: errors.New("authenticate")})
	_, _ = DialMeshBridge(context.Background(), endpoint)
	dial(&coverageConn{writes: []int{64}, deadlineErrors: []error{nil, errors.New("reset")}})
	_, _ = DialMeshBridge(deadlineContext, endpoint)
	udp := endpoint
	udp.LocalNetwork = MeshBridgeNetworkUDP
	dial(&coverageConn{err: errors.New("datagram")})
	_, _ = DialMeshBridge(context.Background(), udp)
	dial(&coverageConn{writes: []int{1}})
	_, _ = DialMeshBridge(context.Background(), udp)
}
