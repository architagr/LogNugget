//go:build testing

package entry_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/architagr/lognugget/config"
	"github.com/architagr/lognugget/entry"
	"github.com/architagr/lognugget/enum"
	"github.com/architagr/lognugget/test/support"
)

// Test_ContextFieldsAppender_10Fields verifies that an appender writing 10
// context fields produces all 10 key-value pairs in the JSON log output.
func Test_ContextFieldsAppender_10Fields(t *testing.T) {
	support.NewConfigBuilder(t).
		MinLevel(enum.LevelDebug).
		Encoder(enum.EncoderJSON).
		Build()
	config.SetContextFieldsAppender(func(ctx context.Context, dst []byte) []byte {
		dst = append(dst, `,"trace_id":"abc123","span_id":"span789","request_id":"req-001"`...)
		dst = append(dst, `,"user_id":"usr-42","session_id":"sess-x","region":"us-east-1"`...)
		dst = append(dst, `,"service":"api-gateway","version":"1.2.3","env":"production","pod":"pod-abc"`...)
		return dst
	})
	spy := support.NewFakePreProc("p7-10fields")
	config.InitPreProcessors(spy)

	entry.NewLogEntry().Info(context.Background(), "ctx-test")

	raw := drainTimestampRec(t, spy)

	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatalf("invalid JSON: %v — output: %s", err, raw)
	}
	wantKeys := []string{
		"trace_id", "span_id", "request_id", "user_id", "session_id",
		"region", "service", "version", "env", "pod",
	}
	for _, k := range wantKeys {
		if _, ok := m[k]; !ok {
			t.Errorf("missing field %q in output: %s", k, raw)
		}
	}
}

// Test_ContextFieldsAppender_VsParser_SameOutput verifies that appender and
// parser paths produce the same keys for the same set of 10 fields (order may
// differ for the parser; the appender order is deterministic).
func Test_ContextFieldsAppender_VsParser_SameOutput(t *testing.T) {
	wantKeys := []string{
		"trace_id", "span_id", "request_id", "user_id", "session_id",
		"region", "service", "version", "env", "pod",
	}

	drain := func(t *testing.T) map[string]any {
		t.Helper()
		spy := support.NewFakePreProc("p7-vsparser")
		config.InitPreProcessors(spy)
		entry.NewLogEntry().Info(context.Background(), "ctx-test")
		raw := drainTimestampRec(t, spy)
		var m map[string]any
		if err := json.Unmarshal(raw, &m); err != nil {
			t.Fatalf("invalid JSON: %v — output: %s", err, raw)
		}
		return m
	}

	// Appender path
	support.NewConfigBuilder(t).MinLevel(enum.LevelDebug).Encoder(enum.EncoderJSON).Build()
	config.SetContextFieldsAppender(func(ctx context.Context, dst []byte) []byte {
		dst = append(dst, `,"trace_id":"abc123","span_id":"span789","request_id":"req-001"`...)
		dst = append(dst, `,"user_id":"usr-42","session_id":"sess-x","region":"us-east-1"`...)
		dst = append(dst, `,"service":"api-gateway","version":"1.2.3","env":"production","pod":"pod-abc"`...)
		return dst
	})
	appenderOut := drain(t)

	// Parser path
	support.NewConfigBuilder(t).MinLevel(enum.LevelDebug).Encoder(enum.EncoderJSON).Build()
	config.SetContextFieldsParser(func(ctx context.Context) map[string]any {
		return map[string]any{
			"trace_id": "abc123", "span_id": "span789", "request_id": "req-001",
			"user_id": "usr-42", "session_id": "sess-x", "region": "us-east-1",
			"service": "api-gateway", "version": "1.2.3", "env": "production", "pod": "pod-abc",
		}
	})
	parserOut := drain(t)

	for _, k := range wantKeys {
		if _, ok := appenderOut[k]; !ok {
			t.Errorf("appender: missing field %q", k)
		}
		if _, ok := parserOut[k]; !ok {
			t.Errorf("parser: missing field %q", k)
		}
	}
}
