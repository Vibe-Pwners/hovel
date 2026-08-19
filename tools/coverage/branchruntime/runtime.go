// Package branchruntime records exact decision edges for generated compatibility
// test targets. It is imported only by shadow sources and is not shipped in an
// SDK or module binary.
package branchruntime

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

var recorder = struct {
	sync.Mutex
	seen map[string]struct{}
}{seen: map[string]struct{}{}}

// Hit records an edge once per test process.
func Hit(id string) {
	recorder.Lock()
	defer recorder.Unlock()
	if _, exists := recorder.seen[id]; exists {
		return
	}
	recorder.seen[id] = struct{}{}
	directory := os.Getenv("TEST_UNDECLARED_OUTPUTS_DIR")
	if directory == "" {
		return
	}
	path := filepath.Join(directory, "go-branch-hits.jsonl")
	file, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		panic(fmt.Sprintf("open Go branch evidence: %v", err))
	}
	encoded, err := json.Marshal(id)
	if err == nil {
		_, err = file.Write(append(encoded, '\n'))
	}
	closeErr := file.Close()
	if err != nil {
		panic(fmt.Sprintf("write Go branch evidence: %v", err))
	}
	if closeErr != nil {
		panic(fmt.Sprintf("close Go branch evidence: %v", closeErr))
	}
}

// Bool records one of the two outcomes without changing the value.
func Bool(trueID, falseID string, value bool) bool {
	if value {
		Hit(trueID)
	} else {
		Hit(falseID)
	}
	return value
}

// And preserves Go's left-to-right, short-circuit && evaluation.
func And(evaluatedID, skippedID string, left bool, right func() bool) bool {
	if !left {
		Hit(skippedID)
		return false
	}
	Hit(evaluatedID)
	return right()
}

// Or preserves Go's left-to-right, short-circuit || evaluation.
func Or(evaluatedID, skippedID string, left bool, right func() bool) bool {
	if left {
		Hit(skippedID)
		return true
	}
	Hit(evaluatedID)
	return right()
}
