package daemonlocal

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestResolveClientOptionsPrecedence(t *testing.T) {
	workspace := t.TempDir()
	configPath := filepath.Join(workspace, "config.yaml")
	if err := os.WriteFile(configPath, []byte(`apiVersion: hovel.dev/v1alpha1
kind: HovelConfig
daemon:
  client:
    endpoint: tcp://yaml:9090
    allowInsecureFullControl: false
    connectTimeout: 1s
`), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOVEL_DAEMON_ENDPOINT", "tcp://environment:9090")
	t.Setenv("HOVEL_DAEMON_ALLOW_INSECURE_FULL_CONTROL", "false")
	t.Setenv("HOVEL_DAEMON_CONNECT_TIMEOUT", "3s")

	options, err := ResolveClientOptions(workspace, "", ClientOverrides{
		Endpoint: "tcp://flag:9090", AllowInsecure: true, AllowInsecureSet: true, ConnectTimeout: "5s",
	})
	if err != nil {
		t.Fatal(err)
	}
	if options.Endpoint != "tcp://flag:9090" || !options.AllowInsecureFullControl || options.ConnectTimeout != 5*time.Second {
		t.Fatalf("resolved client options = %#v", options)
	}
}
