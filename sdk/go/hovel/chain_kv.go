package hovel

import (
	"errors"
	"fmt"
	"strings"
)

var ErrChainKVUnavailable = errors.New("hovel: chain kv is not available in this runtime")

type ChainKVBinding struct {
	Key         string `json:"key"`
	ConfigKey   string `json:"configKey,omitempty"`
	StepID      string `json:"stepId,omitempty"`
	Description string `json:"description,omitempty"`
	Required    bool   `json:"required,omitempty"`
}

type ChainKVContract struct {
	Requires []ChainKVBinding `json:"requires,omitempty"`
	Produces []ChainKVBinding `json:"produces,omitempty"`
}

// ChainKVContractProvider is optional. Existing modules do not need to
// implement it; modules use it to declare validated survey-to-exploit handoffs.
type ChainKVContractProvider interface {
	ChainKVContract() ChainKVContract
}

type ChainKVResolution struct {
	Value  any
	Source string
	Found  bool
}

type chainKVMutation struct {
	Operation string `json:"operation"`
	Key       string `json:"key"`
	Value     string `json:"value,omitempty"`
}

type ChainKV struct {
	available bool
	target    string
	revision  uint64
	entries   map[string]string
	mutations []chainKVMutation
}

func newChainKV(target string, revision uint64, entries map[string]string) *ChainKV {
	cloned := make(map[string]string, len(entries))
	for key, value := range entries {
		cloned[key] = value
	}
	return &ChainKV{available: true, target: target, revision: revision, entries: cloned}
}

func unavailableChainKV(target string) *ChainKV {
	return &ChainKV{target: target, entries: map[string]string{}}
}

func (k *ChainKV) Available() bool { return k != nil && k.available }

func (k *ChainKV) Revision() uint64 {
	if k == nil {
		return 0
	}
	return k.revision
}

func (k *ChainKV) Get(key string) (string, bool) {
	if k == nil || !k.available {
		return "", false
	}
	value, ok := k.entries[k.expand(key)]
	return value, ok
}

func (k *ChainKV) Exists(key string) bool {
	_, ok := k.Get(key)
	return ok
}

func (k *ChainKV) Set(key, value string) error {
	if !k.Available() {
		return ErrChainKVUnavailable
	}
	key = k.expand(key)
	if strings.TrimSpace(key) == "" {
		return errors.New("hovel: chain kv key is required")
	}
	k.entries[key] = value
	k.mutations = append(k.mutations, chainKVMutation{Operation: "set", Key: key, Value: value})
	return nil
}

func (k *ChainKV) Delete(key string) error {
	if !k.Available() {
		return ErrChainKVUnavailable
	}
	key = k.expand(key)
	if strings.TrimSpace(key) == "" {
		return errors.New("hovel: chain kv key is required")
	}
	delete(k.entries, key)
	k.mutations = append(k.mutations, chainKVMutation{Operation: "delete", Key: key})
	return nil
}

func (k *ChainKV) expand(key string) string {
	return strings.ReplaceAll(key, "{target}", percentEncodeChainKVTarget(k.target))
}

func percentEncodeChainKVTarget(value string) string {
	var out strings.Builder
	for _, b := range []byte(value) {
		if b >= 'a' && b <= 'z' || b >= 'A' && b <= 'Z' || b >= '0' && b <= '9' || strings.ContainsRune("-_.~", rune(b)) {
			out.WriteByte(b)
		} else {
			_, _ = fmt.Fprintf(&out, "%%%02X", b)
		}
	}
	return out.String()
}

func (k *ChainKV) wireMutations() map[string]any {
	if k == nil || !k.available || len(k.mutations) == 0 {
		return nil
	}
	return map[string]any{"baseRevision": k.revision, "operations": append([]chainKVMutation(nil), k.mutations...)}
}

func (c *Context) ChainKV() *ChainKV {
	if c.chainKV == nil {
		c.chainKV = unavailableChainKV(c.Target)
	}
	return c.chainKV
}

func (c *Context) ResolveInput(configKey, kvKey string, def any) ChainKVResolution {
	if value, ok := c.Inputs[configKey]; ok {
		return ChainKVResolution{Value: value, Source: "input", Found: true}
	}
	if value, ok := c.TargetConfig[configKey]; ok {
		return ChainKVResolution{Value: value, Source: "target-config", Found: true}
	}
	if value, ok := c.ChainConfig[configKey]; ok {
		return ChainKVResolution{Value: value, Source: "chain-config", Found: true}
	}
	if value, ok := c.ChainKV().Get(kvKey); ok {
		return ChainKVResolution{Value: value, Source: "chain-kv", Found: true}
	}
	return ChainKVResolution{Value: def, Source: "default", Found: false}
}
