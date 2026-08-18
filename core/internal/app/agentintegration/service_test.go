package agentintegration

import (
	"context"
	"io"
	"testing"
)

type recordingInstaller struct {
	request InstallRequest
}

func (r *recordingInstaller) Install(_ context.Context, request InstallRequest, _ io.Writer) error {
	r.request = request
	return nil
}

func TestServiceDefaultsAndValidatesInstall(t *testing.T) {
	recorder := &recordingInstaller{}
	service := Service{Installer: recorder}
	if err := service.Install(context.Background(), InstallRequest{Host: "CODEX"}, io.Discard); err != nil {
		t.Fatal(err)
	}
	if recorder.request.Host != HostCodex || recorder.request.Scope != ScopeUser || recorder.request.Version == "" {
		t.Fatalf("normalized request = %#v", recorder.request)
	}
	for _, request := range []InstallRequest{
		{Host: "unknown"},
		{Host: HostClaude, Scope: "machine"},
		{Host: HostClaude, Version: "latest"},
		{Host: HostClaude, Version: "0.3.1", Source: "local.tgz"},
	} {
		if err := service.Install(context.Background(), request, io.Discard); err == nil {
			t.Fatalf("Install(%#v) succeeded", request)
		}
	}
}
