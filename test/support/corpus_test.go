//go:build testing

// Package support_test exercises the corpus and golden test helpers.
package support_test

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/architagr/lognugget/test/support"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestJSONCorpus_SameSeedProducesIdenticalBytes verifies the determinism
// contract: two calls with the same seed and count yield byte-identical slices.
func TestJSONCorpus_SameSeedProducesIdenticalBytes(t *testing.T) {
	t.Parallel()

	first := support.JSONCorpus(42, 100)
	second := support.JSONCorpus(42, 100)

	require.Len(t, first, 100)
	require.Len(t, second, 100)
	for i := range first {
		assert.True(t, bytes.Equal(first[i], second[i]),
			"item %d differed between two calls with the same seed", i)
	}
}

// TestJSONCorpus_ReturnsNNonEmptyItems checks that n=1000 yields exactly
// 1000 non-empty byte slices, each a valid JSON object.
func TestJSONCorpus_ReturnsNNonEmptyItems(t *testing.T) {
	t.Parallel()

	items := support.JSONCorpus(42, 1000)

	require.Len(t, items, 1000, "must return exactly n items")
	for i, item := range items {
		require.NotEmpty(t, item, "item %d must not be empty", i)
		assert.True(t, json.Valid(item), "item %d must be valid JSON: %s", i, item)
	}
}

// TestJSONCorpus_DifferentSeedsDifferentOutput confirms that distinct seeds
// yield distinct corpora so tests that depend on variety are not silently
// collapsed into identical data.
func TestJSONCorpus_DifferentSeedsDifferentOutput(t *testing.T) {
	t.Parallel()

	a := support.JSONCorpus(1, 10)
	b := support.JSONCorpus(2, 10)

	// At least one item must differ; if all matched that would be a collision
	// severe enough to break corpus-diversity guarantees.
	anyDiffer := false
	for i := range a {
		if !bytes.Equal(a[i], b[i]) {
			anyDiffer = true
			break
		}
	}
	assert.True(t, anyDiffer, "corpora with different seeds must not be identical")
}

// TestJSONCorpus_ZeroNReturnsEmptySlice guards against a nil vs empty-slice
// distinction that can cause range panics in callers.
func TestJSONCorpus_ZeroNReturnsEmptySlice(t *testing.T) {
	t.Parallel()

	items := support.JSONCorpus(42, 0)
	assert.NotNil(t, items, "must return non-nil slice for n=0")
	assert.Empty(t, items)
}
