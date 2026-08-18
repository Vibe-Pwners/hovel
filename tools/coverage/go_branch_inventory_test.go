package main

import (
	"go/ast"
	"go/parser"
	"go/token"
	"testing"
)

func TestCollectorEnumeratesDecisionOutcomes(t *testing.T) {
	const source = `package sample
func f(xs []int, a, b bool) {
	if a && b {}
	for a {}
	for range xs {}
	for range []int{1} {}
	switch len(xs) { case 1: }
	select { default: }
}`
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "sdk/go/sample.go", source, parser.SkipObjectResolution)
	if err != nil {
		t.Fatal(err)
	}
	c := collector{fset: fset, path: "sdk/go/sample.go", platform: "all"}
	ast.Inspect(file, c.visit)
	want := map[string]int{"if": 2, "and": 2, "for": 2, "range": 3, "switch": 2, "select": 1}
	got := map[string]int{}
	for _, edge := range c.edges {
		got[edge.Kind]++
	}
	for kind, count := range want {
		if got[kind] != count {
			t.Errorf("%s edges = %d, want %d", kind, got[kind], count)
		}
	}
}
