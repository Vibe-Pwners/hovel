package hovel

import (
	"reflect"
	"testing"
)

type contextCoverageNode struct {
	Secret CredentialBytes      `json:"secret"`
	Next   *contextCoverageNode `json:"next"`
	Skip   string               `json:"-"`
	plain  string
}

func TestContextAndStructuredLoggingBranchContract(t *testing.T) {
	var nilLogger *Logger
	nilLogger.Info("ignored")
	(&Logger{}).Info("ignored")

	var records []logRecord
	logger := &Logger{name: "contract", emit: func(record logRecord) { records = append(records, record) }}
	logger.Debug("debug")
	logger.Info("info", "odd")
	logger.Warn("warning", 7, "value")
	logger.Error("error", "secret", CredentialBytes("private"))
	if len(records) != 4 || records[0].Fields != nil || len(records[1].Fields) != 0 {
		t.Fatalf("unexpected structured records: %#v", records)
	}

	node := &contextCoverageNode{Secret: CredentialBytes("secret"), Skip: "skip", plain: "plain"}
	node.Next = node
	mapCycle := map[string]any{}
	mapCycle["self"] = mapCycle
	sliceCycle := make([]any, 1)
	sliceCycle[0] = sliceCycle
	values := []any{
		nil,
		(*contextCoverageNode)(nil),
		new(int),
		node,
		map[string]any{},
		map[string]any{"plain": 1},
		map[string]any{"secret": CredentialBytes("secret")},
		mapCycle,
		[]string{"plain"},
		[]any{},
		[]CredentialBytes{CredentialBytes("secret")},
		sliceCycle,
		[1]string{"plain"},
		[1]CredentialBytes{CredentialBytes("secret")},
		contextCoverageNode{},
		struct{}{},
		make(chan int),
		(func())(nil),
	}
	for _, value := range values {
		_ = sanitizeLogField(value)
	}

	hidden := reflect.ValueOf(struct{ hidden string }{hidden: "value"}).Field(0)
	if value, changed := (&logFieldSanitizer{visiting: map[logFieldVisit]struct{}{}}).sanitize(hidden); value != nil || changed {
		t.Fatalf("unexported field sanitized as %#v, %v", value, changed)
	}
	for _, field := range []reflect.StructField{
		{Name: "Default"},
		{Name: "Named", Tag: `json:"renamed,omitempty"`},
		{Name: "Skipped", Tag: `json:"-"`},
	} {
		_, _ = logFieldJSONName(field)
	}

	ctx := &Context{
		Inputs:       map[string]any{"value": "input", "number": 7},
		TargetConfig: map[string]any{"value": "target", "target": "target"},
		ChainConfig:  map[string]any{"value": "chain", "chain": "chain"},
	}
	checks := map[string]any{"value": "input", "target": "target", "chain": "chain", "missing": "default"}
	for key, want := range checks {
		if got := ctx.Input(key, "default"); got != want {
			t.Errorf("Input(%q) = %#v, want %#v", key, got, want)
		}
	}
	if ctx.InputString("number", "") != "7" || ctx.InputString("missing", "default") != "default" {
		t.Fatal("InputString precedence or coercion changed")
	}
	if _, err := ctx.OpenSession(nil); err == nil {
		t.Fatal("OpenSession accepted a runtime without session support")
	}
}
