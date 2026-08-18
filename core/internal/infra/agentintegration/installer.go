package agentintegration

import (
	"archive/tar"
	"bufio"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"

	app "github.com/vibepwners/hovel/internal/app/agentintegration"
)

const releaseBaseURL = "https://github.com/vibepwners/hovel/releases/download"

type CommandRunner func(context.Context, string, []string) error
type Downloader func(context.Context, string) ([]byte, error)

type Installer struct {
	HomeDir    string
	WorkDir    string
	CacheDir   string
	RunCommand CommandRunner
	Download   Downloader
}

func (i Installer) Install(ctx context.Context, request app.InstallRequest, output io.Writer) error {
	resolved, err := i.defaults()
	if err != nil {
		return err
	}
	version := strings.TrimPrefix(request.Version, "v")
	if request.DryRun {
		return resolved.describe(request, version, output)
	}
	switch request.Host {
	case app.HostClaude:
		return resolved.installClaude(ctx, request, version, output)
	case app.HostCodex:
		return resolved.installCodex(ctx, request, version, output)
	case app.HostOpenCode:
		return resolved.installOpenCode(ctx, request, version, output)
	default:
		return fmt.Errorf("unsupported agent host %q", request.Host)
	}
}

func (i Installer) defaults() (Installer, error) {
	var err error
	if i.HomeDir == "" {
		i.HomeDir, err = os.UserHomeDir()
		if err != nil {
			return Installer{}, fmt.Errorf("resolve home directory: %w", err)
		}
	}
	if i.WorkDir == "" {
		i.WorkDir, err = os.Getwd()
		if err != nil {
			return Installer{}, fmt.Errorf("resolve working directory: %w", err)
		}
	}
	if i.CacheDir == "" {
		cache, cacheErr := os.UserCacheDir()
		if cacheErr != nil {
			return Installer{}, fmt.Errorf("resolve cache directory: %w", cacheErr)
		}
		i.CacheDir = filepath.Join(cache, "hovel", "agent")
	}
	if i.RunCommand == nil {
		i.RunCommand = runCommand
	}
	if i.Download == nil {
		i.Download = download
	}
	return i, nil
}

func (i Installer) describe(request app.InstallRequest, version string, output io.Writer) error {
	if err := writeFormat(output, "Dry run: install Hovel agent integration %s for %s at %s scope\n", version, request.Host, request.Scope); err != nil {
		return err
	}
	if request.Source != "" {
		if err := writeFormat(output, "Source: %s\n", request.Source); err != nil {
			return err
		}
	} else if request.Host != app.HostClaude {
		if err := writeFormat(output, "Source: %s/v%s/hovel-agent-%s-v%s.tar.gz\n", releaseBaseURL, version, request.Host, version); err != nil {
			return err
		}
	}
	switch request.Host {
	case app.HostClaude:
		source := "vibepwners/hovel@v" + version
		if request.Source != "" {
			source = request.Source
		}
		if err := writeFormat(output, "Run: claude plugin marketplace add %s --scope %s\n", source, request.Scope); err != nil {
			return err
		}
		return writeFormat(output, "Run: claude plugin install hovel@vibepwners-hovel --scope %s\n", request.Scope)
	case app.HostCodex:
		if request.Scope == app.ScopeUser {
			return writeFormat(output, "Add the packaged Codex marketplace and install hovel@vibepwners-hovel\n")
		} else {
			return writeFormat(output, "Write skills under %s and merge .codex/config.toml\n", filepath.Join(i.WorkDir, ".agents", "skills"))
		}
	case app.HostOpenCode:
		skills, config := i.openCodePaths(request.Scope)
		return writeFormat(output, "Write skills under %s and merge %s\n", skills, config)
	}
	return nil
}

func (i Installer) installClaude(ctx context.Context, request app.InstallRequest, version string, output io.Writer) error {
	source := "vibepwners/hovel@v" + version
	if request.Source != "" {
		root, err := i.prepareSource(ctx, request, version, app.HostClaude)
		if err != nil {
			return err
		}
		source = root
	}
	if err := i.RunCommand(ctx, "claude", []string{"plugin", "marketplace", "add", source, "--scope", string(request.Scope)}); err != nil {
		return err
	}
	if err := i.RunCommand(ctx, "claude", []string{"plugin", "install", "hovel@vibepwners-hovel", "--scope", string(request.Scope)}); err != nil {
		return err
	}
	return writeFormat(output, "Installed Hovel agent integration %s for Claude Code (%s scope).\n", version, request.Scope)
}

func (i Installer) installCodex(ctx context.Context, request app.InstallRequest, version string, output io.Writer) error {
	root, err := i.prepareSource(ctx, request, version, app.HostCodex)
	if err != nil {
		return err
	}
	if request.Scope == app.ScopeUser {
		if err := i.RunCommand(ctx, "codex", []string{"plugin", "marketplace", "add", root}); err != nil {
			if !request.Force {
				return fmt.Errorf("add Codex marketplace (use --force to replace a conflicting Hovel marketplace): %w", err)
			}
			if removeErr := i.RunCommand(ctx, "codex", []string{"plugin", "marketplace", "remove", "vibepwners-hovel"}); removeErr != nil {
				return fmt.Errorf("remove conflicting Codex marketplace: %w", removeErr)
			}
			if retryErr := i.RunCommand(ctx, "codex", []string{"plugin", "marketplace", "add", root}); retryErr != nil {
				return retryErr
			}
		}
		if err := i.RunCommand(ctx, "codex", []string{"plugin", "add", "hovel@vibepwners-hovel"}); err != nil {
			return err
		}
	} else {
		sourceSkills := filepath.Join(root, ".agents", "plugins", "plugins", "hovel", "skills")
		destination := filepath.Join(i.WorkDir, ".agents", "skills")
		if err := installSkillTree(sourceSkills, destination, request.Force); err != nil {
			return err
		}
		config := filepath.Join(i.WorkDir, ".codex", "config.toml")
		if err := mergeCodexConfig(config, request.Force); err != nil {
			return err
		}
	}
	return writeFormat(output, "Installed Hovel agent integration %s for Codex (%s scope).\n", version, request.Scope)
}

func (i Installer) installOpenCode(ctx context.Context, request app.InstallRequest, version string, output io.Writer) error {
	root, err := i.prepareSource(ctx, request, version, app.HostOpenCode)
	if err != nil {
		return err
	}
	sourceSkills := filepath.Join(root, ".opencode", "skills")
	destination, config := i.openCodePaths(request.Scope)
	if err := installSkillTree(sourceSkills, destination, request.Force); err != nil {
		return err
	}
	if err := mergeOpenCodeConfig(config, request.Force); err != nil {
		return err
	}
	return writeFormat(output, "Installed Hovel agent integration %s for OpenCode (%s scope).\n", version, request.Scope)
}

func (i Installer) openCodePaths(scope app.Scope) (string, string) {
	if scope == app.ScopeProject {
		return filepath.Join(i.WorkDir, ".opencode", "skills"), filepath.Join(i.WorkDir, "opencode.json")
	}
	return filepath.Join(i.HomeDir, ".config", "opencode", "skills"), filepath.Join(i.HomeDir, ".config", "opencode", "opencode.json")
}

func (i Installer) prepareSource(ctx context.Context, request app.InstallRequest, version string, host app.Host) (string, error) {
	if request.Source != "" {
		path, err := filepath.Abs(request.Source)
		if err != nil {
			return "", err
		}
		info, err := os.Stat(path)
		if err != nil {
			return "", fmt.Errorf("inspect agent integration source: %w", err)
		}
		if info.IsDir() {
			if err := validatePackage(path, host, version); err != nil {
				return "", err
			}
			return path, nil
		}
		root := filepath.Join(i.CacheDir, "local", string(host))
		if err := extractArchive(path, root); err != nil {
			return "", err
		}
		if err := validatePackage(root, host, version); err != nil {
			return "", err
		}
		return root, nil
	}
	root := filepath.Join(i.CacheDir, version, string(host))
	marker := filepath.Join(root, ".complete")
	if _, err := os.Stat(marker); err == nil {
		if err := validatePackage(root, host, version); err != nil {
			return "", err
		}
		return root, nil
	}
	base := fmt.Sprintf("%s/v%s", releaseBaseURL, version)
	name := fmt.Sprintf("hovel-agent-%s-v%s.tar.gz", host, version)
	archive, err := i.Download(ctx, base+"/"+name)
	if err != nil {
		return "", fmt.Errorf("download %s: %w", name, err)
	}
	checksums, err := i.Download(ctx, base+"/SHA256SUMS")
	if err != nil {
		return "", fmt.Errorf("download SHA256SUMS: %w", err)
	}
	if err := verifyChecksum(name, archive, checksums); err != nil {
		return "", err
	}
	if err := os.MkdirAll(filepath.Dir(root), 0o755); err != nil {
		return "", err
	}
	temporary, err := os.MkdirTemp(filepath.Dir(root), ".agent-integration-")
	if err != nil {
		return "", err
	}
	defer func() { ignoreError(os.RemoveAll(temporary)) }()
	archivePath := filepath.Join(temporary, name)
	if err := os.WriteFile(archivePath, archive, 0o600); err != nil {
		return "", err
	}
	staged := filepath.Join(temporary, "root")
	if err := extractArchive(archivePath, staged); err != nil {
		return "", err
	}
	if err := os.WriteFile(filepath.Join(staged, ".complete"), []byte(version+"\n"), 0o644); err != nil {
		return "", err
	}
	if err := os.RemoveAll(root); err != nil {
		return "", err
	}
	if err := os.Rename(staged, root); err != nil {
		return "", err
	}
	if err := validatePackage(root, host, version); err != nil {
		return "", err
	}
	return root, nil
}

func validatePackage(root string, host app.Host, version string) error {
	body, err := os.ReadFile(filepath.Join(root, "hovel-agent.json"))
	if err != nil {
		return fmt.Errorf("read Hovel agent package manifest: %w", err)
	}
	var manifest struct {
		Name    string `json:"name"`
		Version string `json:"version"`
		Host    string `json:"host"`
	}
	if err := json.Unmarshal(body, &manifest); err != nil {
		return fmt.Errorf("parse Hovel agent package manifest: %w", err)
	}
	if manifest.Name != "hovel" || manifest.Version != version || manifest.Host != string(host) {
		return fmt.Errorf("agent package identifies %s %s for %s, want Hovel %s for %s", manifest.Name, manifest.Version, manifest.Host, version, host)
	}
	return nil
}

func runCommand(ctx context.Context, name string, args []string) error {
	path, err := exec.LookPath(name)
	if err != nil {
		return fmt.Errorf("%s is not installed or not on PATH", name)
	}
	command := exec.CommandContext(ctx, path, args...)
	output, err := command.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s %s failed: %w: %s", name, strings.Join(args, " "), err, strings.TrimSpace(string(output)))
	}
	return nil
}

func download(ctx context.Context, url string) ([]byte, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		return nil, err
	}
	defer func() { ignoreError(response.Body.Close()) }()
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GET %s returned %s", url, response.Status)
	}
	return io.ReadAll(io.LimitReader(response.Body, 64<<20))
}

func verifyChecksum(name string, body, checksums []byte) error {
	want := ""
	scanner := bufio.NewScanner(strings.NewReader(string(checksums)))
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) == 2 && strings.TrimPrefix(fields[1], "*") == name {
			want = strings.ToLower(fields[0])
			break
		}
	}
	if want == "" {
		return fmt.Errorf("SHA256SUMS does not contain %s", name)
	}
	digest := sha256.Sum256(body)
	got := hex.EncodeToString(digest[:])
	if got != want {
		return fmt.Errorf("checksum mismatch for %s: got %s, want %s", name, got, want)
	}
	return nil
}

func extractArchive(archivePath, destination string) error {
	file, err := os.Open(archivePath)
	if err != nil {
		return err
	}
	defer func() { ignoreError(file.Close()) }()
	gz, err := gzip.NewReader(file)
	if err != nil {
		return fmt.Errorf("open agent integration archive: %w", err)
	}
	defer func() { ignoreError(gz.Close()) }()
	if err := os.RemoveAll(destination); err != nil {
		return err
	}
	if err := os.MkdirAll(destination, 0o755); err != nil {
		return err
	}
	reader := tar.NewReader(gz)
	for {
		header, nextErr := reader.Next()
		if errors.Is(nextErr, io.EOF) {
			break
		}
		if nextErr != nil {
			return nextErr
		}
		clean := filepath.Clean(filepath.FromSlash(header.Name))
		if clean == "." || filepath.IsAbs(clean) || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
			return fmt.Errorf("unsafe path %q in agent integration archive", header.Name)
		}
		target := filepath.Join(destination, clean)
		switch header.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, 0o755); err != nil {
				return err
			}
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				return err
			}
			stream, err := os.OpenFile(target, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
			if err != nil {
				return err
			}
			_, copyErr := io.Copy(stream, io.LimitReader(reader, 16<<20))
			closeErr := stream.Close()
			if copyErr != nil {
				return copyErr
			}
			if closeErr != nil {
				return closeErr
			}
		default:
			return fmt.Errorf("unsupported archive entry %q", header.Name)
		}
	}
	return nil
}

func installSkillTree(source, destination string, force bool) error {
	entries, err := os.ReadDir(source)
	if err != nil {
		return fmt.Errorf("read packaged skills: %w", err)
	}
	if err := os.MkdirAll(destination, 0o755); err != nil {
		return err
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		src := filepath.Join(source, entry.Name())
		dst := filepath.Join(destination, entry.Name())
		if same, compareErr := sameTree(src, dst); compareErr == nil && same {
			continue
		}
		if _, statErr := os.Stat(dst); statErr == nil {
			if !force {
				return fmt.Errorf("skill %s already exists with different content; use --force to replace it", dst)
			}
			if err := backupPath(dst); err != nil {
				return err
			}
		}
		temporary, err := os.MkdirTemp(destination, ".hovel-skill-")
		if err != nil {
			return err
		}
		if err := os.Remove(temporary); err != nil {
			return err
		}
		if err := copyTree(src, temporary); err != nil {
			return err
		}
		if err := os.Rename(temporary, dst); err != nil {
			return err
		}
	}
	return nil
}

func copyTree(source, destination string) error {
	return filepath.WalkDir(source, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		relative, err := filepath.Rel(source, path)
		if err != nil {
			return err
		}
		target := filepath.Join(destination, relative)
		if entry.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("skill source contains unsupported symlink %s", path)
		}
		body, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(target, body, 0o644)
	})
}

func sameTree(left, right string) (bool, error) {
	leftFiles, err := treeDigests(left)
	if err != nil {
		return false, err
	}
	rightFiles, err := treeDigests(right)
	if err != nil {
		return false, err
	}
	if len(leftFiles) != len(rightFiles) {
		return false, nil
	}
	for name, digest := range leftFiles {
		if rightFiles[name] != digest {
			return false, nil
		}
	}
	return true, nil
}

func treeDigests(root string) (map[string]string, error) {
	result := map[string]string{}
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			return nil
		}
		body, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		digest := sha256.Sum256(body)
		result[filepath.ToSlash(relative)] = hex.EncodeToString(digest[:])
		return nil
	})
	return result, err
}

func backupPath(path string) error {
	return os.Rename(path, nextBackupPath(path))
}

func mergeOpenCodeConfig(path string, force bool) error {
	desired := map[string]any{"type": "local", "command": []any{"hovel", "mcp"}, "enabled": true}
	body, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		value := map[string]any{"$schema": "https://opencode.ai/config.json", "mcp": map[string]any{"hovel": desired}}
		encoded, marshalErr := json.MarshalIndent(value, "", "  ")
		if marshalErr != nil {
			return marshalErr
		}
		return atomicWrite(path, append(encoded, '\n'), false)
	}
	if err != nil {
		return err
	}
	standard, err := stripJSONComments(body)
	if err != nil {
		return fmt.Errorf("parse OpenCode config %s: %w", path, err)
	}
	normalized := stripTrailingJSONCommas(standard)
	var value map[string]any
	if err := json.Unmarshal(normalized, &value); err != nil {
		return fmt.Errorf("parse OpenCode config %s: %w", path, err)
	}
	mcp, ok := value["mcp"].(map[string]any)
	if !ok && value["mcp"] != nil {
		return fmt.Errorf("OpenCode config %s has a non-object mcp value", path)
	}
	if mcp == nil {
		mcp = map[string]any{}
		value["mcp"] = mcp
	}
	existing, hovelExists := mcp["hovel"]
	if hovelExists {
		if equalJSON(existing, desired) {
			return nil
		}
		if !force {
			return fmt.Errorf("OpenCode config %s already defines a different hovel MCP server; use --force to replace it", path)
		}
	}
	updated, err := patchOpenCodeJSONC(body, standard, value["mcp"] != nil, hovelExists)
	if err != nil {
		return err
	}
	return atomicWrite(path, updated, true)
}

func stripTrailingJSONCommas(body []byte) []byte {
	result := append([]byte(nil), body...)
	inString := false
	escaped := false
	for index := 0; index < len(result); index++ {
		if inString {
			switch {
			case escaped:
				escaped = false
			case result[index] == '\\':
				escaped = true
			case result[index] == '"':
				inString = false
			}
			continue
		}
		if result[index] == '"' {
			inString = true
			continue
		}
		if result[index] != ',' {
			continue
		}
		next := skipJSONSpace(result, index+1)
		if next < len(result) && (result[next] == '}' || result[next] == ']') {
			result[index] = ' '
		}
	}
	return result
}

func patchOpenCodeJSONC(original, clean []byte, hasMCP, hasHovel bool) ([]byte, error) {
	rootStart := bytesIndexByte(clean, '{', 0)
	if rootStart < 0 {
		return nil, errors.New("OpenCode config root must be an object")
	}
	rootEnd, err := matchingJSONDelimiter(clean, rootStart)
	if err != nil {
		return nil, err
	}
	const hovelValue = `{"type":"local","command":["hovel","mcp"],"enabled":true}`
	if !hasMCP {
		return insertJSONObjectProperty(original, clean, rootStart, rootEnd, `"mcp": {"hovel": `+hovelValue+`}`, "  "), nil
	}
	mcpStart, mcpEnd, found, err := findJSONObjectProperty(clean, rootStart, rootEnd, "mcp")
	if err != nil || !found {
		return nil, errors.New("cannot locate OpenCode mcp object")
	}
	if clean[mcpStart] != '{' {
		return nil, errors.New("OpenCode mcp value must be an object")
	}
	if !hasHovel {
		return insertJSONObjectProperty(original, clean, mcpStart, mcpEnd, `"hovel": `+hovelValue, "    "), nil
	}
	hovelStart, hovelEnd, found, err := findJSONObjectProperty(clean, mcpStart, mcpEnd, "hovel")
	if err != nil || !found {
		return nil, errors.New("cannot locate OpenCode hovel MCP entry")
	}
	updated := append([]byte(nil), original[:hovelStart]...)
	updated = append(updated, hovelValue...)
	updated = append(updated, original[hovelEnd:]...)
	return updated, nil
}

func insertJSONObjectProperty(original, clean []byte, start, end int, property, indent string) []byte {
	content := strings.TrimSpace(string(clean[start+1 : end]))
	prefix := "\n" + indent
	if content != "" {
		trimmed := strings.TrimSpace(string(clean[:end]))
		if !strings.HasSuffix(trimmed, ",") {
			prefix = "," + prefix
		}
	}
	insert := prefix + property + "\n" + strings.TrimSuffix(indent, "  ")
	updated := append([]byte(nil), original[:end]...)
	updated = append(updated, insert...)
	updated = append(updated, original[end:]...)
	return updated
}

func findJSONObjectProperty(clean []byte, objectStart, objectEnd int, key string) (int, int, bool, error) {
	depth := 1
	for index := objectStart + 1; index < objectEnd; {
		switch clean[index] {
		case '"':
			stringEnd, err := scanJSONString(clean, index)
			if err != nil {
				return 0, 0, false, err
			}
			if depth == 1 && string(clean[index+1:stringEnd-1]) == key {
				cursor := skipJSONSpace(clean, stringEnd)
				if cursor >= objectEnd || clean[cursor] != ':' {
					return 0, 0, false, errors.New("JSON object key is missing a colon")
				}
				valueStart := skipJSONSpace(clean, cursor+1)
				valueEnd, valueErr := scanJSONValue(clean, valueStart, objectEnd)
				return valueStart, valueEnd, true, valueErr
			}
			index = stringEnd
		case '{', '[':
			depth++
			index++
		case '}', ']':
			depth--
			index++
		default:
			index++
		}
	}
	return 0, 0, false, nil
}

func scanJSONValue(clean []byte, start, limit int) (int, error) {
	if start >= limit {
		return 0, errors.New("missing JSON value")
	}
	if clean[start] == '"' {
		return scanJSONString(clean, start)
	}
	if clean[start] == '{' || clean[start] == '[' {
		return matchingJSONDelimiter(clean, start)
	}
	end := start
	for end < limit && clean[end] != ',' && clean[end] != '}' && clean[end] != ']' {
		end++
	}
	return len(strings.TrimRight(string(clean[start:end]), " \t\r\n")) + start, nil
}

func matchingJSONDelimiter(clean []byte, start int) (int, error) {
	open := clean[start]
	close := byte('}')
	if open == '[' {
		close = ']'
	} else if open != '{' {
		return 0, errors.New("JSON value is not an object or array")
	}
	depth := 0
	for index := start; index < len(clean); index++ {
		if clean[index] == '"' {
			end, err := scanJSONString(clean, index)
			if err != nil {
				return 0, err
			}
			index = end - 1
			continue
		}
		switch clean[index] {
		case open:
			depth++
		case close:
			depth--
			if depth == 0 {
				return index, nil
			}
		}
	}
	return 0, errors.New("unterminated JSON object or array")
}

func scanJSONString(clean []byte, start int) (int, error) {
	escaped := false
	for index := start + 1; index < len(clean); index++ {
		switch {
		case escaped:
			escaped = false
		case clean[index] == '\\':
			escaped = true
		case clean[index] == '"':
			return index + 1, nil
		}
	}
	return 0, errors.New("unterminated JSON string")
}

func skipJSONSpace(body []byte, start int) int {
	for start < len(body) && (body[start] == ' ' || body[start] == '\t' || body[start] == '\r' || body[start] == '\n') {
		start++
	}
	return start
}

func bytesIndexByte(body []byte, want byte, start int) int {
	for index := start; index < len(body); index++ {
		if body[index] == want {
			return index
		}
	}
	return -1
}

func stripJSONComments(body []byte) ([]byte, error) {
	result := append([]byte(nil), body...)
	inString := false
	escaped := false
	for index := 0; index < len(result); index++ {
		char := result[index]
		if inString {
			switch {
			case escaped:
				escaped = false
			case char == '\\':
				escaped = true
			case char == '"':
				inString = false
			}
			continue
		}
		if char == '"' {
			inString = true
			continue
		}
		if char != '/' || index+1 >= len(result) {
			continue
		}
		switch result[index+1] {
		case '/':
			result[index], result[index+1] = ' ', ' '
			index += 2
			for index < len(result) && result[index] != '\n' {
				result[index] = ' '
				index++
			}
			index--
		case '*':
			result[index], result[index+1] = ' ', ' '
			index += 2
			closed := false
			for index < len(result) {
				if index+1 < len(result) && result[index] == '*' && result[index+1] == '/' {
					result[index], result[index+1] = ' ', ' '
					index++
					closed = true
					break
				}
				if result[index] != '\n' && result[index] != '\r' {
					result[index] = ' '
				}
				index++
			}
			if !closed {
				return nil, errors.New("unterminated block comment")
			}
		}
	}
	if inString {
		return nil, errors.New("unterminated string")
	}
	return result, nil
}

func equalJSON(left, right any) bool {
	return reflect.DeepEqual(left, right)
}

func mergeCodexConfig(path string, force bool) error {
	const section = "[mcp_servers.hovel]\ncommand = \"hovel\"\nargs = [\"mcp\"]\n"
	body, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return atomicWrite(path, []byte(section), false)
	}
	if err != nil {
		return err
	}
	text := string(body)
	start := strings.Index(text, "[mcp_servers.hovel]")
	if start < 0 {
		separator := ""
		if len(text) > 0 && !strings.HasSuffix(text, "\n") {
			separator = "\n"
		}
		return atomicWrite(path, []byte(text+separator+"\n"+section), true)
	}
	end := len(text)
	if offset := strings.Index(text[start+1:], "\n["); offset >= 0 {
		end = start + 1 + offset + 1
	}
	existing := text[start:end]
	if strings.Contains(existing, `command = "hovel"`) && strings.Contains(existing, `args = ["mcp"]`) {
		return nil
	}
	if !force {
		return fmt.Errorf("codex config %s already defines a different hovel MCP server; use --force to replace it", path)
	}
	return atomicWrite(path, []byte(text[:start]+section+text[end:]), true)
}

func atomicWrite(path string, body []byte, backup bool) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	temporary, err := os.CreateTemp(filepath.Dir(path), ".hovel-config-")
	if err != nil {
		return err
	}
	temporaryPath := temporary.Name()
	defer func() { ignoreError(os.Remove(temporaryPath)) }()
	if _, err := temporary.Write(body); err != nil {
		ignoreError(temporary.Close())
		return err
	}
	if err := temporary.Chmod(0o600); err != nil {
		ignoreError(temporary.Close())
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	backupPathName := ""
	if backup {
		backupPathName = nextBackupPath(path)
		if err := os.Rename(path, backupPathName); err != nil {
			return err
		}
	}
	if err := os.Rename(temporaryPath, path); err != nil {
		if backupPathName != "" {
			if restoreErr := os.Rename(backupPathName, path); restoreErr != nil {
				return fmt.Errorf("install config: %w; restore backup: %v", err, restoreErr)
			}
		}
		return err
	}
	return nil
}

func writeFormat(output io.Writer, format string, args ...any) error {
	_, err := fmt.Fprintf(output, format, args...)
	return err
}

func ignoreError(error) {}

func nextBackupPath(path string) string {
	backup := path + ".hovel-backup"
	for suffix := 2; ; suffix++ {
		if _, err := os.Stat(backup); errors.Is(err, os.ErrNotExist) {
			return backup
		}
		backup = fmt.Sprintf("%s.hovel-backup.%d", path, suffix)
	}
}
