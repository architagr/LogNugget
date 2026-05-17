//go:build testing

// Package support provides test builders, lightweight fakes/spies, and
// corpus/golden helpers for the lognugget logger. It compiles only under
// the `testing` build tag, preventing accidental import from production
// binaries. Key entry points: NewConfigBuilder, JSONCorpus, Golden.
package support

import (
	"encoding/json"
	"fmt"
	"math/rand"
)

// JSONCorpus returns n JSON-encoded byte slices generated deterministically
// from seed. Each item is a JSON object with "msg", "level", and "rand"
// fields. Identical seed+n inputs always produce identical output, making
// corpora safe for snapshot and benchmark baselines.
//
// Passing n=0 returns a non-nil empty slice; callers may range over it safely.
func JSONCorpus(seed int64, n int) [][]byte {
	r := rand.New(rand.NewSource(seed)) //nolint:gosec // why: test corpus; cryptographic randomness would break determinism
	out := make([][]byte, n)
	for i := 0; i < n; i++ {
		m := map[string]any{
			"msg":   fmt.Sprintf("seed-%d-item-%d", seed, i),
			"level": "debug",
			"rand":  r.Int63(),
		}
		b, _ := json.Marshal(m)
		out[i] = b
	}
	return out
}
