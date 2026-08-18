package hoveltest

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"testing"

	"github.com/vibepwners/hovel/sdk/go/hovel"
)

type failingContractProvider struct{ fakeProvider }

func (failingContractProvider) Info() hovel.Info {
	info := (fakeProvider{}).Info()
	if os.Getenv("HOVELTEST_FAILURE_MODE") == "module-type" {
		info.Type = hovel.TypeSurvey
	}
	return info
}

func (failingContractProvider) ListPayloads(query hovel.PayloadQuery) ([]hovel.PayloadInfo, error) {
	if os.Getenv("HOVELTEST_FAILURE_MODE") == "payload-list" {
		return nil, nil
	}
	return (fakeProvider{}).ListPayloads(query)
}

func (failingContractProvider) ResolvePayload(query hovel.PayloadQuery) (hovel.PayloadInfo, error) {
	info, err := (fakeProvider{}).ResolvePayload(query)
	switch os.Getenv("HOVELTEST_FAILURE_MODE") {
	case "resolved-id":
		info.ID = ""
	case "kind":
		info.Kind = "wrong"
	case "format":
		info.Formats = nil
	case "transport":
		info.Transport.Kind = "wrong"
	case "tag":
		info.Tags = nil
	case "capability":
		info.Capabilities = nil
	}
	return info, err
}

func (failingContractProvider) GeneratePayload(request hovel.GeneratePayloadRequest) (hovel.PayloadArtifactSet, error) {
	result, err := (fakeProvider{}).GeneratePayload(request)
	switch os.Getenv("HOVELTEST_FAILURE_MODE") {
	case "primary-role":
		result.Primary.Role = "wrong"
	case "primary-format":
		result.Primary.Format = ""
	case "generated-kind":
		result.Primary.Kind = "wrong"
	case "encoding":
		result.Primary.Encoding = "wrong"
	case "artifacts":
		result.Artifacts = nil
	}
	return result, err
}

func (failingContractProvider) ConnectSession(request hovel.ConnectSessionRequest) (hovel.SessionRef, error) {
	result, err := (fakeProvider{}).ConnectSession(request)
	switch os.Getenv("HOVELTEST_FAILURE_MODE") {
	case "session-id":
		result.ID = ""
	case "installed-id":
		result.InstalledPayloadID = "wrong"
	}
	return result, err
}

func (failingContractProvider) CleanupPayload(request hovel.CleanupPayloadRequest) (hovel.CleanupResult, error) {
	result, err := (fakeProvider{}).CleanupPayload(request)
	if os.Getenv("HOVELTEST_FAILURE_MODE") == "cleanup" {
		result.Status = "wrong"
	}
	return result, err
}

func contractForFailureTest() PayloadProviderContract {
	return PayloadProviderContract{
		Query: hovel.PayloadQuery{Transport: "reverse-tcp", Format: "pe-exe"},
		Target: "target-1", RunID: "run-1",
		WantKind: string(hovel.PayloadKindPE), WantFormat: "pe-exe",
		WantTransport: "reverse-tcp", WantTags: []string{"native"},
		WantCapabilities: []string{"file.get"}, WantInstalledPayloadID: "installed",
	}
}

func TestPayloadProviderContractFailureChild(t *testing.T) {
	mode := os.Getenv("HOVELTEST_FAILURE_MODE")
	if mode == "" {
		t.Skip("child-process failure probe")
	}
	switch mode {
	case "marshal":
		conn := NewRPCConn(t, fakeProvider{})
		conn.Call("handshake", func() {}, nil)
		return
	case "rpc-error":
		conn := NewRPCConn(t, fakeProvider{})
		conn.Call("unknown", nil, nil)
		return
	case "decode-result":
		conn := NewRPCConn(t, fakeProvider{})
		var result chan int
		conn.Call("handshake", nil, &result)
		return
	case "read-error":
		(&RPCConn{t: t, out: bufio.NewReader(strings.NewReader(""))}).readFrame()
		return
	case "bad-length":
		(&RPCConn{t: t, out: bufio.NewReader(strings.NewReader("Content-Length: bad\r\n\r\n"))}).readFrame()
		return
	case "short-body":
		(&RPCConn{t: t, out: bufio.NewReader(strings.NewReader("Content-Length: 2\r\n\r\nx"))}).readFrame()
		return
	case "bad-json":
		(&RPCConn{t: t, out: bufio.NewReader(strings.NewReader("Content-Length: 1\r\n\r\n{"))}).readFrame()
		return
	case "write-header":
		_, writer := io.Pipe()
		_ = writer.Close()
		(&RPCConn{t: t, in: writer}).Call("handshake", nil, nil)
		return
	case "write-body":
		reader, writer := io.Pipe()
		go func() {
			buffered := bufio.NewReader(reader)
			_, _ = buffered.ReadString('\n')
			_, _ = buffered.ReadString('\n')
			_ = reader.Close()
		}()
		(&RPCConn{t: t, in: writer}).Call("handshake", nil, nil)
		return
	case "serve-error":
		reader, writer := io.Pipe()
		go func() {
			buffered := bufio.NewReader(reader)
			_, _ = buffered.ReadString('\n')
			_, _ = buffered.ReadString('\n')
			body := make([]byte, 60)
			_, _ = io.ReadFull(buffered, body)
		}()
		var response strings.Builder
		response.WriteString("Content-Length: 36\r\n\r\n{\"jsonrpc\":\"2.0\",\"id\":1,\"result\":{}}")
		(&RPCConn{t: t, in: writer, out: bufio.NewReader(strings.NewReader(response.String())), done: func() chan error { ch := make(chan error, 1); ch <- fmt.Errorf("injected"); return ch }()}).Close()
		return
	}
	AssertPayloadProviderContract(t, failingContractProvider{}, contractForFailureTest())
}

func TestPayloadProviderContractFailureBranches(t *testing.T) {
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	for _, mode := range []string{
		"module-type", "payload-list", "resolved-id", "kind", "format", "transport", "tag", "capability",
		"primary-role", "primary-format", "generated-kind", "encoding", "artifacts", "session-id", "installed-id", "cleanup",
		"marshal", "rpc-error", "decode-result", "read-error", "bad-length", "short-body", "bad-json", "write-header", "write-body", "serve-error",
	} {
		command := exec.Command(executable, "-test.run=^TestPayloadProviderContractFailureChild$")
		command.Env = append(os.Environ(), "HOVELTEST_FAILURE_MODE="+mode)
		if err := command.Run(); err == nil {
			t.Errorf("failure mode %q unexpectedly passed", mode)
		}
	}
}

func TestPayloadProviderOptionalExpectationsAndRPCFrames(t *testing.T) {
	AssertPayloadProviderContract(t, fakeProvider{}, PayloadProviderContract{
		Query: hovel.PayloadQuery{}, Target: "target", RunID: "run",
	})
	conn := NewRPCConn(t, fakeProvider{})
	conn.Call("handshake", nil, nil)
	conn.Close()
	frame := fmt.Sprintf("Other\r\nOther: value\r\nContent-Length: %d\r\n\r\n{}", 2)
	message := (&RPCConn{t: t, out: bufio.NewReader(strings.NewReader(frame))}).readFrame()
	if message == nil {
		t.Fatal("frame was not decoded")
	}
}

func TestRPCConnSkipsNotificationsAndReportsCleanupErrors(t *testing.T) {
	reader, writer := io.Pipe()
	defer reader.Close()
	go func() { _, _ = io.Copy(io.Discard, reader) }()
	var frames strings.Builder
	frames.WriteString("Content-Length: 34\r\n\r\n{\"jsonrpc\":\"2.0\",\"method\":\"event\"}")
	frames.WriteString("Content-Length: 36\r\n\r\n{\"jsonrpc\":\"2.0\",\"id\":1,\"result\":{}}")
	c := &RPCConn{t: t, in: writer, out: bufio.NewReader(strings.NewReader(frames.String()))}
	c.Call("handshake", nil, nil)
	logRPCOutputClose(fmt.Errorf("injected"))
	logRPCInputClose(t, fmt.Errorf("injected"))
}

func TestContainsEmptyAndMissing(t *testing.T) {
	if contains(nil, "missing") || contains([]string{"other"}, "missing") {
		t.Fatal("contains reported a missing value")
	}
}
