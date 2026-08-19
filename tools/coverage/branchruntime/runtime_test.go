package branchruntime

import "testing"

func TestBooleanHelpersPreserveShortCircuitSemantics(t *testing.T) {
	calls := 0
	right := func() bool { calls++; return true }
	if And("and-eval", "and-skip", false, right) || calls != 0 {
		t.Fatal("And evaluated a skipped operand")
	}
	if !And("and-eval", "and-skip", true, right) || calls != 1 {
		t.Fatal("And did not evaluate its right operand")
	}
	if !Or("or-eval", "or-skip", true, right) || calls != 1 {
		t.Fatal("Or evaluated a skipped operand")
	}
	if !Or("or-eval", "or-skip", false, right) || calls != 2 {
		t.Fatal("Or did not evaluate its right operand")
	}
	if !Bool("true", "false", true) || Bool("true", "false", false) {
		t.Fatal("Bool changed its input")
	}
}
