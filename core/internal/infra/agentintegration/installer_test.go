package agentintegration

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	app "github.com/vibepwners/hovel/internal/app/agentintegration"
)

func TestDryRunDoesNotRequireNetworkOrHostCLI(t *testing.T) {
	var output bytes.Buffer
	err := (Installer{HomeDir: t.TempDir(), WorkDir: t.TempDir(), CacheDir: t.TempDir()}).Install(
		context.Background(),
		app.InstallRequest{Host: app.HostClaude, Scope: app.ScopeProject, Version: "0.3.2", DryRun: true},
		&output,
	)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), "vibepwners/hovel@v0.3.2") || !strings.Contains(output.String(), "--scope project") {
		t.Fatalf("dry-run output:\n%s", output.String())
	}
}

func TestClaudeUsesNativeCommands(t *testing.T) {
	var calls [][]string
	installer := Installer{
		HomeDir:  t.TempDir(),
		WorkDir:  t.TempDir(),
		CacheDir: t.TempDir(),
		RunCommand: func(_ context.Context, name string, args []string) error {
			calls = append(calls, append([]string{name}, args...))
			return nil
		},
	}
	err := installer.Install(context.Background(), app.InstallRequest{
		Host: app.HostClaude, Scope: app.ScopeUser, Version: "0.3.2",
	}, io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	want := [][]string{
		{"claude", "plugin", "marketplace", "add", "vibepwners/hovel@v0.3.2", "--scope", "user"},
		{"claude", "plugin", "install", "hovel@vibepwners-hovel", "--scope", "user"},
	}
	if !reflect.DeepEqual(calls, want) {
		t.Fatalf("calls = %#v, want %#v", calls, want)
	}
}

func TestOpenCodeProjectInstallAndConflict(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "source")
	writeTestSkill(t, filepath.Join(source, ".opencode", "skills", "hovel"), "official")
	if err := os.WriteFile(filepath.Join(source, "hovel-agent.json"), []byte(`{"name":"hovel","version":"0.3.2","host":"opencode"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	project := filepath.Join(root, "project")
	installer := Installer{HomeDir: filepath.Join(root, "home"), WorkDir: project, CacheDir: filepath.Join(root, "cache")}
	request := app.InstallRequest{Host: app.HostOpenCode, Scope: app.ScopeProject, Version: "0.3.2", Source: source}
	if err := installer.Install(context.Background(), request, io.Discard); err != nil {
		t.Fatal(err)
	}
	config, err := os.ReadFile(filepath.Join(project, "opencode.json"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(config), `"command": [`) || !strings.Contains(string(config), `"hovel"`) {
		t.Fatalf("OpenCode config:\n%s", config)
	}
	if err := installer.Install(context.Background(), request, io.Discard); err != nil {
		t.Fatalf("idempotent install failed: %v", err)
	}
	if err := os.WriteFile(filepath.Join(source, ".opencode", "skills", "hovel", "SKILL.md"), []byte("changed"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := installer.Install(context.Background(), request, io.Discard); err == nil || !strings.Contains(err.Error(), "--force") {
		t.Fatalf("conflicting install error = %v", err)
	}
}

func TestMergeCodexConfigPreservesOtherSections(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".codex", "config.toml")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	original := "# keep me\nmodel = \"example\"\n\n[features]\nplugins = true\n"
	if err := os.WriteFile(path, []byte(original), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := mergeCodexConfig(path, false); err != nil {
		t.Fatal(err)
	}
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(body), original) || !strings.Contains(string(body), "[mcp_servers.hovel]") {
		t.Fatalf("merged config:\n%s", body)
	}
	if _, err := os.Stat(path + ".hovel-backup"); err != nil {
		t.Fatalf("backup missing: %v", err)
	}
}

func TestMergeOpenCodeConfigPreservesJSONCComments(t *testing.T) {
	path := filepath.Join(t.TempDir(), "opencode.jsonc")
	original := "{\n  // keep this comment\n  \"model\": \"https://example.test/model\",\n  \"mcp\": {\n    /* and this one */\n    \"other\": {\"type\": \"local\", \"command\": [\"other\"]}\n  }\n}\n"
	if err := os.WriteFile(path, []byte(original), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := mergeOpenCodeConfig(path, false); err != nil {
		t.Fatal(err)
	}
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"// keep this comment", "/* and this one */", "https://example.test/model", `"hovel":`} {
		if !strings.Contains(string(body), want) {
			t.Fatalf("merged config missing %q:\n%s", want, body)
		}
	}
}

func TestMergeOpenCodeConfigAddsMissingMCPObject(t *testing.T) {
	path := filepath.Join(t.TempDir(), "opencode.json")
	original := "{\n  \"model\": \"example\"\n}\n"
	if err := os.WriteFile(path, []byte(original), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := mergeOpenCodeConfig(path, false); err != nil {
		t.Fatal(err)
	}
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`"model": "example"`, `"mcp":`, `"hovel":`} {
		if !strings.Contains(string(body), want) {
			t.Fatalf("merged config missing %q:\n%s", want, body)
		}
	}
}

func TestChecksumAndArchiveTraversal(t *testing.T) {
	body := []byte("archive")
	digest := sha256.Sum256(body)
	if err := verifyChecksum("bundle.tgz", body, []byte(fmt.Sprintf("%x  bundle.tgz\n", digest))); err != nil {
		t.Fatal(err)
	}
	if err := verifyChecksum("bundle.tgz", []byte("other"), []byte(fmt.Sprintf("%x  bundle.tgz\n", digest))); err == nil {
		t.Fatal("checksum mismatch succeeded")
	}
	archive := filepath.Join(t.TempDir(), "unsafe.tar.gz")
	writeArchive(t, archive, "../escape", []byte("bad"))
	if err := extractArchive(archive, filepath.Join(t.TempDir(), "out")); err == nil {
		t.Fatal("unsafe archive extraction succeeded")
	}
}

func writeTestSkill(t *testing.T, root, body string) {
	t.Helper()
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "SKILL.md"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func writeArchive(t *testing.T, path, name string, body []byte) {
	t.Helper()
	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	gz := gzip.NewWriter(file)
	archive := tar.NewWriter(gz)
	if err := archive.WriteHeader(&tar.Header{Name: name, Mode: 0o644, Size: int64(len(body)), Typeflag: tar.TypeReg}); err != nil {
		t.Fatal(err)
	}
	if _, err := archive.Write(body); err != nil {
		t.Fatal(err)
	}
	for _, closer := range []func() error{archive.Close, gz.Close, file.Close} {
		if err := closer(); err != nil {
			t.Fatal(err)
		}
	}
}
