package main

import (
	"flag"
	"fmt"
	"go/ast"
	"go/format"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/tools/go/ast/astutil"
)

const branchRuntimeImport = "github.com/vibepwners/hovel/tools/coverage/branchruntime"

func instrumentMain() int {
	input := flag.String("input", "", "Go source file to instrument")
	output := flag.String("output", "", "instrumented Go source destination")
	logicalPath := flag.String("logical-path", "", "stable repository-relative source path")
	flag.Parse()
	if *input == "" || *output == "" {
		fmt.Fprintln(os.Stderr, "--input and --output are required")
		return 2
	}
	data, err := os.ReadFile(*input)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	path := *logicalPath
	if path == "" {
		path = filepath.ToSlash(*input)
	}
	instrumented, _, err := instrumentGoSource(path, data)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	if err := os.MkdirAll(filepath.Dir(*output), 0o755); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	if err := os.WriteFile(*output, instrumented, 0o644); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	return 0
}

func instrumentGoSource(path string, source []byte) ([]byte, int, error) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, path, source, parser.ParseComments|parser.SkipObjectResolution)
	if err != nil {
		return nil, 0, fmt.Errorf("parse %s: %w", path, err)
	}
	path = filepath.ToSlash(path)
	count := 0
	decisionPositions := map[ast.Node]token.Pos{}
	edgeID := func(position token.Pos, kind, outcome string) string {
		pos := fset.Position(position)
		return fmt.Sprintf("%s:%d:%d:%s:%s", path, pos.Line, pos.Column, kind, outcome)
	}
	pair := func(position token.Pos, kind, first, second string, expression ast.Expr) ast.Expr {
		count += 2
		return branchCall("Bool", stringLiteral(edgeID(position, kind, first)), stringLiteral(edgeID(position, kind, second)), expression)
	}

	astutil.Apply(file, func(cursor *astutil.Cursor) bool {
		switch node := cursor.Node().(type) {
		case *ast.IfStmt:
			decisionPositions[node] = node.Cond.Pos()
		case *ast.ForStmt:
			if node.Cond != nil {
				decisionPositions[node] = node.Cond.Pos()
			}
		case *ast.RangeStmt:
			decisionPositions[node] = node.For
		}
		return true
	}, func(cursor *astutil.Cursor) bool {
		switch node := cursor.Node().(type) {
		case *ast.BinaryExpr:
			if node.Op != token.LAND && node.Op != token.LOR {
				return true
			}
			kind, method := "and", "And"
			if node.Op == token.LOR {
				kind, method = "or", "Or"
			}
			count += 2
			cursor.Replace(branchCall(
				method,
				stringLiteral(edgeID(node.OpPos, kind, "rhs-evaluated")),
				stringLiteral(edgeID(node.OpPos, kind, "rhs-skipped")),
				node.X,
				&ast.FuncLit{
					Type: &ast.FuncType{Params: &ast.FieldList{}, Results: &ast.FieldList{List: []*ast.Field{{Type: ast.NewIdent("bool")}}}},
					Body: &ast.BlockStmt{List: []ast.Stmt{&ast.ReturnStmt{Results: []ast.Expr{node.Y}}}},
				},
			))
		case *ast.IfStmt:
			node.Cond = pair(decisionPositions[node], "if", "true", "false", node.Cond)
		case *ast.ForStmt:
			if node.Cond != nil {
				node.Cond = pair(decisionPositions[node], "for", "enter", "exit", node.Cond)
			}
		case *ast.RangeStmt:
			position := decisionPositions[node]
			if staticallyNonEmptyRange(node.X) {
				node.Body.List = append([]ast.Stmt{hitStatement(edgeID(position, "range", "enter"))}, node.Body.List...)
				count++
				return true
			}
			pos := fset.Position(position)
			seen := ast.NewIdent(fmt.Sprintf("_hovelBranchRangeSeen_%d_%d", pos.Line, pos.Column))
			node.Body.List = append([]ast.Stmt{&ast.IfStmt{
				Cond: &ast.UnaryExpr{Op: token.NOT, X: seen},
				Body: &ast.BlockStmt{List: []ast.Stmt{
					hitStatement(edgeID(position, "range", "enter")),
					&ast.AssignStmt{Lhs: []ast.Expr{seen}, Tok: token.ASSIGN, Rhs: []ast.Expr{ast.NewIdent("true")}},
				}},
			}}, node.Body.List...)
			cursor.Replace(&ast.BlockStmt{List: []ast.Stmt{
				&ast.AssignStmt{Lhs: []ast.Expr{seen}, Tok: token.DEFINE, Rhs: []ast.Expr{ast.NewIdent("false")}},
				node,
				&ast.IfStmt{
					Cond: &ast.UnaryExpr{Op: token.NOT, X: seen},
					Body: &ast.BlockStmt{List: []ast.Stmt{hitStatement(edgeID(position, "range", "empty"))}},
				},
			}})
			count += 2
		case *ast.SwitchStmt:
			instrumentCaseClauses(node.Switch, node.Body, "switch", edgeID, &count)
		case *ast.TypeSwitchStmt:
			instrumentCaseClauses(node.Switch, node.Body, "type-switch", edgeID, &count)
		case *ast.SelectStmt:
			for index, statement := range node.Body.List {
				clause := statement.(*ast.CommClause)
				outcome := fmt.Sprintf("case-%d", index+1)
				if clause.Comm == nil {
					outcome = "default"
				}
				clause.Body = append([]ast.Stmt{hitStatement(edgeID(clause.Case, "select", outcome))}, clause.Body...)
				count++
			}
		}
		return true
	})
	if count > 0 {
		astutil.AddNamedImport(fset, file, "branchcov", branchRuntimeImport)
	}
	var output strings.Builder
	if err := format.Node(&output, fset, file); err != nil {
		return nil, 0, fmt.Errorf("format %s: %w", path, err)
	}
	return []byte(output.String()), count, nil
}

func staticallyNonEmptyRange(expression ast.Expr) bool {
	literal, ok := expression.(*ast.CompositeLit)
	return ok && len(literal.Elts) > 0
}

func instrumentCaseClauses(
	position token.Pos,
	body *ast.BlockStmt,
	kind string,
	edgeID func(token.Pos, string, string) string,
	count *int,
) {
	hasDefault := false
	for index, statement := range body.List {
		clause := statement.(*ast.CaseClause)
		outcome := fmt.Sprintf("case-%d", index+1)
		if clause.List == nil {
			outcome = "default"
			hasDefault = true
		}
		clause.Body = append([]ast.Stmt{hitStatement(edgeID(clause.Case, kind, outcome))}, clause.Body...)
		*count++
	}
	if !hasDefault {
		body.List = append(body.List, &ast.CaseClause{
			Body: []ast.Stmt{hitStatement(edgeID(position, kind, "no-match"))},
		})
		*count++
	}
}

func branchCall(method string, arguments ...ast.Expr) ast.Expr {
	return &ast.CallExpr{Fun: &ast.SelectorExpr{X: ast.NewIdent("branchcov"), Sel: ast.NewIdent(method)}, Args: arguments}
}

func hitStatement(id string) ast.Stmt {
	return &ast.ExprStmt{X: branchCall("Hit", stringLiteral(id))}
}

func stringLiteral(value string) ast.Expr {
	return &ast.BasicLit{Kind: token.STRING, Value: fmt.Sprintf("%q", value)}
}
