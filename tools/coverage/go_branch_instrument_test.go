package main

import (
	"strings"
	"testing"
)

func TestInstrumentGoSourcePreservesDecisionsAndAddsStableEdges(t *testing.T) {
	const source = `package sample
func decide(a, b bool, value int) bool {
	if a && b { return true }
	for value > 0 { value-- }
	for _, item := range []int{value} { value += item }
	switch value { case 1: return false }
	return false
}`
	output, count, err := instrumentGoSource("sdk/go/sample.go", []byte(source))
	if err != nil {
		t.Fatal(err)
	}
	if count != 9 {
		t.Fatalf("edge count = %d, want 9", count)
	}
	text := string(output)
	for _, want := range []string{
		`branchcov.And("sdk/go/sample.go:3:7:and:rhs-evaluated"`,
		`branchcov.Bool("sdk/go/sample.go:3:5:if:true"`,
		`branchcov.Bool("sdk/go/sample.go:4:6:for:enter"`,
		`branchcov.Hit("sdk/go/sample.go:5:2:range:enter")`,
		`branchcov.Hit("sdk/go/sample.go:6:17:switch:case-1")`,
		`branchcov.Hit("sdk/go/sample.go:6:2:switch:no-match")`,
	} {
		if !strings.Contains(text, want) {
			t.Errorf("instrumented source missing %s:\n%s", want, text)
		}
	}
	if strings.Contains(text, `sdk/go/sample.go:5:2:range:empty`) {
		t.Errorf("instrumented statically non-empty range has an impossible empty edge:\n%s", text)
	}
}
