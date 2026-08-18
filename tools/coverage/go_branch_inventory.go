package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type edge struct {
	ID       string `json:"id"`
	Path     string `json:"path"`
	Line     int    `json:"line"`
	Column   int    `json:"column"`
	Kind     string `json:"kind"`
	Outcome  string `json:"outcome"`
	Platform string `json:"platform"`
	Covered  bool   `json:"covered"`
}

type manifest struct {
	SchemaVersion string `json:"schema_version"`
	Language      string `json:"language"`
	Scope         string `json:"scope"`
	Edges         []edge `json:"edges"`
}

type collector struct {
	fset     *token.FileSet
	path     string
	platform string
	edges    []edge
}

func main() {
	output := flag.String("output", "coverage/go-sdk-branches.manifest.json", "workspace-relative output path")
	results := flag.String("results", "coverage/go-sdk-branches.results.json", "workspace-relative report metric path")
	hitsRoot := flag.String("hits-root", "", "directory containing go-branch-hits.jsonl test outputs")
	scope := flag.String("scope", "Go SDK", "human-readable compatibility scope")
	flag.Parse()
	workspace := os.Getenv("BUILD_WORKSPACE_DIRECTORY")
	if workspace == "" {
		workspace = "."
	}
	inputs := flag.Args()
	if len(inputs) == 0 {
		inputs = []string{"sdk/go/hovel", "sdk/go/hoveltest"}
	}
	names, err := sourceFiles(inputs)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	var edges []edge
	for _, name := range names {
		fset := token.NewFileSet()
		file, err := parser.ParseFile(fset, name, nil, parser.SkipObjectResolution)
		if err != nil {
			fmt.Fprintf(os.Stderr, "parse %s: %v\n", name, err)
			os.Exit(1)
		}
		path := filepath.ToSlash(name)
		if index := strings.Index(path, "sdk/go/"); index >= 0 {
			path = path[index:]
		}
		c := collector{fset: fset, path: path, platform: sourcePlatform(name)}
		ast.Inspect(file, c.visit)
		edges = append(edges, c.edges...)
	}
	sort.Slice(edges, func(i, j int) bool { return edges[i].ID < edges[j].ID })
	for i := 1; i < len(edges); i++ {
		if edges[i-1].ID == edges[i].ID {
			fmt.Fprintf(os.Stderr, "duplicate edge ID: %s\n", edges[i].ID)
			os.Exit(1)
		}
	}
	hitsPath := *hitsRoot
	if hitsPath != "" && !filepath.IsAbs(hitsPath) {
		hitsPath = filepath.Join(workspace, hitsPath)
	}
	hits, err := readHits(hitsPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	covered := 0
	for index := range edges {
		_, edges[index].Covered = hits[edges[index].ID]
		if edges[index].Covered {
			covered++
		}
	}

	data, err := json.MarshalIndent(manifest{
		SchemaVersion: "hovel.go-branch-manifest/v1",
		Language:      "go",
		Scope:         *scope,
		Edges:         edges,
	}, "", "  ")
	if err != nil {
		panic(err)
	}
	destination := filepath.Join(workspace, filepath.FromSlash(*output))
	if err := os.MkdirAll(filepath.Dir(destination), 0o755); err != nil {
		panic(err)
	}
	if err := os.WriteFile(destination, append(data, '\n'), 0o644); err != nil {
		panic(err)
	}
	metricData, err := json.MarshalIndent([]map[string]any{{
		"name": "Go SDK branches", "metric_type": "branch", "language": "go",
		"platforms": []string{"linux", "windows"}, "covered": covered, "total": len(edges), "minimum": 100.0,
	}}, "", "  ")
	if err != nil {
		panic(err)
	}
	resultDestination := filepath.Join(workspace, filepath.FromSlash(*results))
	if err := os.WriteFile(resultDestination, append(metricData, '\n'), 0o644); err != nil {
		panic(err)
	}
	fmt.Printf("Go SDK branches: %d/%d (%.2f%%; %s)\n", covered, len(edges), percentage(covered, len(edges)), destination)
	if len(edges) == 0 || covered != len(edges) {
		os.Exit(1)
	}
}

func readHits(root string) (map[string]bool, error) {
	hits := map[string]bool{}
	if root == "" {
		return hits, nil
	}
	resolved, err := filepath.EvalSymlinks(root)
	if err != nil {
		return nil, fmt.Errorf("resolve hits root %s: %w", root, err)
	}
	root = resolved
	err = filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || entry.Name() != "go-branch-hits.jsonl" {
			return nil
		}
		file, err := os.Open(path)
		if err != nil {
			return err
		}
		defer file.Close()
		scanner := bufio.NewScanner(file)
		for scanner.Scan() {
			var id string
			if err := json.Unmarshal(scanner.Bytes(), &id); err != nil {
				return fmt.Errorf("decode %s: %w", path, err)
			}
			hits[id] = true
		}
		return scanner.Err()
	})
	return hits, err
}

func percentage(covered, total int) float64 {
	if total == 0 {
		return 0
	}
	return float64(covered) * 100 / float64(total)
}

func sourceFiles(inputs []string) ([]string, error) {
	var names []string
	for _, input := range inputs {
		info, err := os.Stat(input)
		if err != nil {
			return nil, fmt.Errorf("inspect %s: %w", input, err)
		}
		if !info.IsDir() {
			if strings.HasSuffix(input, ".go") && !strings.HasSuffix(input, "_test.go") {
				names = append(names, input)
			}
			continue
		}
		err = filepath.WalkDir(input, func(path string, entry os.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if !entry.IsDir() && strings.HasSuffix(path, ".go") && !strings.HasSuffix(path, "_test.go") {
				names = append(names, path)
			}
			return nil
		})
		if err != nil {
			return nil, fmt.Errorf("walk %s: %w", input, err)
		}
	}
	sort.Strings(names)
	return names, nil
}

func (c *collector) visit(node ast.Node) bool {
	switch n := node.(type) {
	case *ast.IfStmt:
		c.pair(n.Cond.Pos(), "if", "true", "false")
	case *ast.ForStmt:
		if n.Cond != nil {
			c.pair(n.Cond.Pos(), "for", "enter", "exit")
		}
	case *ast.RangeStmt:
		if staticallyNonEmptyRange(n.X) {
			c.add(n.For, "range", "enter")
		} else {
			c.pair(n.For, "range", "enter", "empty")
		}
	case *ast.BinaryExpr:
		if n.Op == token.LAND {
			c.pair(n.OpPos, "and", "rhs-evaluated", "rhs-skipped")
		} else if n.Op == token.LOR {
			c.pair(n.OpPos, "or", "rhs-evaluated", "rhs-skipped")
		}
	case *ast.SwitchStmt:
		c.clauses(n.Switch, n.Body.List, "switch")
	case *ast.TypeSwitchStmt:
		c.clauses(n.Switch, n.Body.List, "type-switch")
	case *ast.SelectStmt:
		c.selectClauses(n.Body.List)
	}
	return true
}

func staticallyNonEmptyRange(expression ast.Expr) bool {
	literal, ok := expression.(*ast.CompositeLit)
	return ok && len(literal.Elts) > 0
}

func (c *collector) selectClauses(statements []ast.Stmt) {
	for index, statement := range statements {
		clause, ok := statement.(*ast.CommClause)
		if !ok {
			continue
		}
		outcome := fmt.Sprintf("case-%d", index+1)
		if clause.Comm == nil {
			outcome = "default"
		}
		c.add(clause.Case, "select", outcome)
	}
}

func (c *collector) clauses(position token.Pos, statements []ast.Stmt, kind string) {
	hasDefault := false
	for index, statement := range statements {
		clause, ok := statement.(*ast.CaseClause)
		if !ok {
			continue
		}
		outcome := fmt.Sprintf("case-%d", index+1)
		if clause.List == nil {
			outcome = "default"
			hasDefault = true
		}
		c.add(clause.Case, kind, outcome)
	}
	if kind != "select" && !hasDefault {
		c.add(position, kind, "no-match")
	}
}

func (c *collector) pair(position token.Pos, kind, first, second string) {
	c.add(position, kind, first)
	c.add(position, kind, second)
}

func (c *collector) add(position token.Pos, kind, outcome string) {
	pos := c.fset.Position(position)
	id := fmt.Sprintf("%s:%d:%d:%s:%s", c.path, pos.Line, pos.Column, kind, outcome)
	c.edges = append(c.edges, edge{
		ID: id, Path: c.path, Line: pos.Line, Column: pos.Column,
		Kind: kind, Outcome: outcome, Platform: c.platform,
	})
}

func sourcePlatform(path string) string {
	base := filepath.Base(path)
	switch {
	case strings.HasSuffix(base, "_linux.go"):
		return "linux"
	case strings.HasSuffix(base, "_windows.go"):
		return "windows"
	case strings.HasSuffix(base, "_unsupported.go"):
		return "unsupported"
	default:
		return "all"
	}
}
