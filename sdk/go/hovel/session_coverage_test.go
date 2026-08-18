package hovel

import (
	"errors"
	"io"
	"os"
	"testing"
	"time"
)

type coverageSession struct {
	openErr, writeErr, readErr, closeErr error
	closed                               bool
	data                                 []byte
}

type coverageWriter struct {
	writes []int
	err    error
}

func (w *coverageWriter) Write(data []byte) (int, error) {
	if len(w.writes) == 0 {
		return 0, w.err
	}
	n := w.writes[0]
	w.writes = w.writes[1:]
	if n > len(data) {
		n = len(data)
	}
	return n, w.err
}

func TestPTYSessionInternalLifecycleBranches(t *testing.T) {
	if err := (&PTYSession{}).Open(); err == nil {
		t.Fatal("nil PTY frontend accepted")
	}

	closed := &PTYSession{closed: true}
	closed.init()
	closed.init()
	closed.emit([]byte("ignored"))
	closed.markClosed()
	if err := closed.Write([]byte("ignored")); err != nil {
		t.Fatal(err)
	}
	if chunk, err := closed.Read(0); err != nil || chunk != nil {
		t.Fatalf("closed read = %q, %v", chunk, err)
	}

	idle := &PTYSession{}
	if err := idle.Write([]byte("ignored")); err != nil {
		t.Fatal(err)
	}
	if chunk, err := idle.Read(0); err != nil || chunk != nil {
		t.Fatalf("nonblocking PTY read = %q, %v", chunk, err)
	}
	if chunk, err := idle.Read(time.Millisecond); err != nil || chunk != nil {
		t.Fatalf("timed PTY read = %q, %v", chunk, err)
	}
	woke := make(chan struct{})
	go func() {
		_, _ = idle.Read(-1)
		close(woke)
	}()
	time.Sleep(time.Millisecond)
	idle.emit([]byte("ready"))
	select {
	case <-woke:
	case <-time.After(time.Second):
		t.Fatal("blocking PTY read did not wake")
	}
	if err := idle.Close("no handles"); err != nil {
		t.Fatal(err)
	}
	fresh := &PTYSession{}
	fresh.init()
	fresh.markClosed()

	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	withMaster := &PTYSession{master: writer}
	if err := withMaster.Write(nil); err != nil {
		t.Fatal(err)
	}
	if err := withMaster.Close("master only"); err != nil {
		t.Fatal(err)
	}

	for _, tc := range []struct {
		writer  io.Writer
		wantErr bool
	}{
		{&coverageWriter{writes: []int{1, 2}}, false},
		{&coverageWriter{writes: []int{1}, err: errors.New("write")}, true},
		{&coverageWriter{writes: []int{0}}, true},
	} {
		err := writeAll(tc.writer, []byte("abc"))
		if (err != nil) != tc.wantErr {
			t.Fatalf("writeAll error = %v, wantErr %v", err, tc.wantErr)
		}
	}
}

func TestPTYSessionOpenAndFrontendErrorBranches(t *testing.T) {
	original := openPTY
	t.Cleanup(func() { openPTY = original })
	openPTY = func() (*os.File, *os.File, *os.File, error) {
		return nil, nil, nil, errors.New("open failed")
	}
	if err := (&PTYSession{Frontend: func(io.Reader, io.Writer) error { return nil }}).Open(); err == nil {
		t.Fatal("PTY open failure was ignored")
	}

	for _, frontendErr := range []error{errors.New("frontend failed"), os.ErrClosed, io.EOF} {
		masterReader, masterWriter, err := os.Pipe()
		if err != nil {
			t.Fatal(err)
		}
		inputReader, inputWriter, err := os.Pipe()
		if err != nil {
			t.Fatal(err)
		}
		outputReader, outputWriter, err := os.Pipe()
		if err != nil {
			t.Fatal(err)
		}
		openPTY = func() (*os.File, *os.File, *os.File, error) {
			return masterReader, inputReader, outputWriter, nil
		}
		session := &PTYSession{Frontend: func(io.Reader, io.Writer) error { return frontendErr }}
		if err := session.Open(); err != nil {
			t.Fatal(err)
		}
		time.Sleep(time.Millisecond)
		_ = masterWriter.Close()
		_ = inputWriter.Close()
		_ = outputReader.Close()
		_ = session.Close("test")
	}
}

func (s *coverageSession) Open() error                        { return s.openErr }
func (s *coverageSession) Write([]byte) error                 { return s.writeErr }
func (s *coverageSession) Read(time.Duration) ([]byte, error) { return s.data, s.readErr }
func (s *coverageSession) Close(string) error {
	if s.closeErr == nil {
		s.closed = true
	}
	return s.closeErr
}
func (s *coverageSession) Closed() bool { return s.closed }

type coverageCommandSession struct{ coverageSession }

func (*coverageCommandSession) ListPayloadCommands(PayloadCommandListRequest) ([]PayloadCommand, error) {
	return []PayloadCommand{{Name: "command"}}, nil
}
func (*coverageCommandSession) RunPayloadCommand(PayloadCommandRequest) (PayloadCommandResult, error) {
	return PayloadCommandResult{Command: "command"}, nil
}

func TestLineShellSessionBranchContract(t *testing.T) {
	shell := &LineShellSession{Echo: true, Handle: func(command string) (string, error) {
		if command == "bad" {
			return "", errors.New("bad command")
		}
		return command, nil
	}}
	if err := shell.Open(); err != nil {
		t.Fatal(err)
	}
	shell.init()
	shell.signal()
	shell.signal()
	for _, input := range []string{"partial", "\n", "\n", "bad\n", "plain\n", "exit\n"} {
		if err := shell.Write([]byte(input)); err != nil {
			t.Fatal(err)
		}
	}
	if err := shell.Write([]byte("ignored\n")); err != nil {
		t.Fatal(err)
	}
	shell.emit([]byte("ignored"))
	for {
		chunk, err := shell.Read(0)
		if err != nil {
			t.Fatal(err)
		}
		if len(chunk) == 0 {
			break
		}
	}
	if err := shell.Close("again"); err != nil {
		t.Fatal(err)
	}
	(&LineShellSession{}).handleLine("no-handler")
	(&LineShellSession{Handle: func(string) (string, error) { return "line\n", nil }}).handleLine("newline")
	closing := &LineShellSession{}
	closing.Handle = func(string) (string, error) {
		_ = closing.Close("handler")
		return "", nil
	}
	closing.handleLine("close")

	waiting := &LineShellSession{}
	waiting.init()
	if chunk, err := waiting.Read(0); err != nil || chunk != nil {
		t.Fatalf("nonblocking read = %q, %v", chunk, err)
	}
	if chunk, err := waiting.Read(time.Millisecond); err != nil || chunk != nil {
		t.Fatalf("timed read = %q, %v", chunk, err)
	}
	done := make(chan struct{})
	go func() {
		_, _ = waiting.Read(-1)
		close(done)
	}()
	time.Sleep(10 * time.Millisecond)
	waiting.emit([]byte("ready"))
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("indefinite read did not wake")
	}
}

func TestSessionManagerFailureAndLifecycleBranchContract(t *testing.T) {
	var events []sessionEvent
	manager := newSessionManager(func(event sessionEvent) { events = append(events, event) })
	_ = appendSessionCapability(nil, "new")
	_ = appendSessionCapability([]string{"same"}, "same")
	scope := sessionScope{runID: "run", moduleID: "module", target: "target"}
	registry := manager.forRun(scope)
	if _, err := registry.open(&coverageSession{openErr: errors.New("open")}); err == nil {
		t.Fatal("open failure was ignored")
	}
	regular := &coverageSession{data: []byte("data")}
	ref, err := registry.open(regular, WithName("name"), WithKind("kind"), WithTransport("transport"), WithCapabilities("custom"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := manager.lookup("missing"); err == nil {
		t.Fatal("unknown lookup succeeded")
	}
	if err := manager.write("missing", nil); err == nil {
		t.Fatal("unknown write succeeded")
	}
	if _, _, err := manager.read("missing", 0); err == nil {
		t.Fatal("unknown read succeeded")
	}
	if err := manager.close("missing", "reason"); err == nil {
		t.Fatal("unknown close succeeded")
	}
	if _, err := manager.listCommands(ref.ID, PayloadCommandListRequest{}); err == nil {
		t.Fatal("non-provider listed commands")
	}
	if _, err := manager.runCommand(ref.ID, PayloadCommandRequest{}); err == nil {
		t.Fatal("non-provider ran a command")
	}
	_, _ = manager.listCommands("missing", PayloadCommandListRequest{})
	_, _ = manager.runCommand("missing", PayloadCommandRequest{})
	if err := manager.write(ref.ID, nil); err != nil {
		t.Fatal(err)
	}
	if chunk, closed, err := manager.read(ref.ID, 0); err != nil || closed || string(chunk) != "data" {
		t.Fatalf("read = %q, %v, %v", chunk, closed, err)
	}
	regular.closed = true
	_, closed, err := manager.read(ref.ID, 0)
	if err != nil || !closed {
		t.Fatalf("closed read = %v, %v", closed, err)
	}
	manager.markClosed("missing", "reason")
	manager.markClosed(ref.ID, "again")
	if manager.state("missing") != "" || len(manager.refsForRun("other")) != 0 {
		t.Fatal("unknown session state leaked")
	}

	failing := &coverageSession{writeErr: errors.New("write"), readErr: errors.New("read"), closeErr: errors.New("close")}
	failingRef, err := registry.open(failing)
	if err != nil {
		t.Fatal(err)
	}
	_ = manager.write(failingRef.ID, nil)
	_, _, _ = manager.read(failingRef.ID, 0)
	_ = manager.close(failingRef.ID, "reason")

	commands := &coverageCommandSession{}
	commandRef, err := registry.open(commands)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := manager.listCommands(commandRef.ID, PayloadCommandListRequest{}); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.runCommand(commandRef.ID, PayloadCommandRequest{}); err != nil {
		t.Fatal(err)
	}
	manager.closeAll("shutdown")
	newSessionManager(nil).fire("ignored", SessionRef{}, nil)
	if len(events) == 0 {
		t.Fatal("session events were not emitted")
	}
}
