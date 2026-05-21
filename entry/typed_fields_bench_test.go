//go:build testing

package entry_test

import (
	"context"
	"testing"

	"github.com/architagr/lognugget/config"
	"github.com/architagr/lognugget/entry"
	"github.com/architagr/lognugget/enum"
	"github.com/architagr/lognugget/model"
	"github.com/architagr/lognugget/test/support"
)

// BenchmarkLogEntry_TypedFields_10 measures the hot path for 10 typed fields
// supplied via chain methods (Str/Int). Goal: 0 allocs/op for the field-append
// segment (P2 AC-4). Full log-call alloc count may be > 0 from mandatory fields
// and channel dispatch; P4 addresses those remaining sources.
func BenchmarkLogEntry_TypedFields_10(b *testing.B) {
	config.TestResetConfig()
	support.NewConfigBuilder(b).
		MinLevel(enum.LevelDebug).
		Encoder(enum.EncoderJSON).
		Build()
	sink := support.NewFakePreProc("typed-bench-sink")
	config.InitPreProcessors(sink)
	b.ReportAllocs()
	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		entry.NewLogEntry().
			Str("k1", "v1").
			Str("k2", "v2").
			Int("n1", 1).
			Int("n2", 2).
			Str("k3", "v3").
			Str("k4", "v4").
			Int("n3", 3).
			Int("n4", 4).
			Str("k5", "v5").
			Bool("ok", true).
			Info(ctx, "bench-typed")
	}
}

// BenchmarkLogEntry_LegacyAttrs_10 measures the hot path for 10 KindAny attrs
// supplied as variadic fields. This is the regression guard for P2 AC-5: alloc
// count must not exceed the P1 baseline.
func BenchmarkLogEntry_LegacyAttrs_10(b *testing.B) {
	config.TestResetConfig()
	support.NewConfigBuilder(b).
		MinLevel(enum.LevelDebug).
		Encoder(enum.EncoderJSON).
		Build()
	sink := support.NewFakePreProc("legacy-bench-sink")
	config.InitPreProcessors(sink)
	b.ReportAllocs()
	ctx := context.Background()

	fields := []model.LogAttr{
		{Key: "k1", Value: "v1"},
		{Key: "k2", Value: "v2"},
		{Key: "n1", Value: 1},
		{Key: "n2", Value: 2},
		{Key: "k3", Value: "v3"},
		{Key: "k4", Value: "v4"},
		{Key: "n3", Value: 3},
		{Key: "n4", Value: 4},
		{Key: "k5", Value: "v5"},
		{Key: "ok", Value: true},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		entry.NewLogEntry().Info(ctx, "bench-legacy", fields...)
	}
}
