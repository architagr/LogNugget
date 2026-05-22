# P7 — First-Class OTel ContextFieldsAppender + Tracing Benchmark

Owner-role: engineer.
Issue: [#118](https://github.com/architagr/LogNugget/issues/118).
Branch: `feat/118-p7-otel-context-appender` (cut from `feat/111-v3-performance`, PRs back to `feat/111-v3-performance`).
Depends: P2 merged to feat/111-v3-performance (the benchmark verifies the zero-alloc path which goes through `GetHotSnapshot`).
LOC est: 150.

## Summary

The existing `Benchmark_Log_Parallel_10CtxFields` benchmarks use `config.SetContextFieldsParser` (the legacy map-based path: ~100 ns, 4 allocs per call). This story:

1. Updates both parallel benchmarks to use `config.SetContextFieldsAppender` (zero-alloc, the path added in V2 P3).
2. Adds a new `Benchmark_Log_Parallel_OtelCtx` benchmark that extracts OTel trace/span IDs via `ContextFieldsAppender`.
3. Creates `examples/otel-appender/main.go` — a runnable example showing the OTel integration pattern.

No OTel SDK is added to the core `go.mod`. The `examples/otel-appender/` directory has its own `go.mod` that imports `go.opentelemetry.io/otel/trace`.

**Saving:** ~100 ns, 4 allocs/op (by switching `Benchmark_Log_Parallel_10CtxFields` from map parser to appender).

## Files touched

- `entry/parallel_bench_test.go` — update `BenchmarkLognugget_Parallel_10CtxFields` to use `SetContextFieldsAppender`
- `test/benchmark/log_parallel_bench_test.go` — update `Benchmark_Log_Parallel_10CtxFields` to use `SetContextFieldsAppender`
- NEW `test/benchmark/otel_bench_test.go` — `Benchmark_Log_Parallel_OtelCtx` (uses build tag `//go:build otel_bench`)
- NEW `examples/otel-appender/main.go` — runnable OTel appender demo
- NEW `examples/otel-appender/go.mod` — standalone module with OTel dependency
- NEW `examples/otel-appender/go.sum`

## ContextFieldsAppender pattern for 10 ctx fields

Replace the parser registration:
```go
// BEFORE (legacy map path — allocates map + []string)
config.SetContextFieldsParser(func(ctx context.Context) map[string]any {
    return map[string]any{
        "trace_id": "abc123", "span_id": "span789", // ... 10 fields
    }
})

// AFTER (zero-alloc appender path)
config.SetContextFieldsAppender(func(ctx context.Context, dst []byte) []byte {
    dst = append(dst, `,"trace_id":"abc123","span_id":"span789","request_id":"req-001"`...)
    dst = append(dst, `,"user_id":"usr-42","session_id":"sess-x","region":"us-east-1"`...)
    dst = append(dst, `,"service":"api-gateway","version":"1.2.3","env":"production","pod":"pod-abc"`...)
    return dst
})
```

Note: in the benchmark the fields are fixed constants. In production, values come from `context.Context`.

## OTel appender pattern (examples/otel-appender/)

```go
import "go.opentelemetry.io/otel/trace"

config.SetContextFieldsAppender(func(ctx context.Context, dst []byte) []byte {
    span := trace.SpanFromContext(ctx)
    if !span.IsRecording() {
        return dst
    }
    sc := span.SpanContext()
    dst = append(dst, `,"trace_id":"`...)
    dst = append(dst, sc.TraceID().String()...)
    dst = append(dst, `","span_id":"`...)
    dst = append(dst, sc.SpanID().String()...)
    dst = append(dst, '"')
    return dst
})
```

## Tests-first gate

1. `Test_ContextFieldsAppender_10Fields` — set an appender that writes 10 fields; log a message; verify all 10 fields appear in the output.
2. `Test_ContextFieldsAppender_NilContext` — appender is not called when `ctx == nil` (existing `AppendContextFields` guard covers this; verify it still holds).
3. `Test_ContextFieldsAppender_VsParser_SameOutput` — for the same 10 fields, parser and appender produce the same key-value pairs in the output (order may differ for parser; appender order is deterministic).
4. `BenchmarkLognugget_Parallel_10CtxFields` (updated): ≤ 1 alloc/op (appender path).
5. `Benchmark_Log_Parallel_10CtxFields` (updated): ≤ 1 alloc/op.

## Acceptance criteria

1. Both `Benchmark_Log_Parallel_10CtxFields` benchmarks use `SetContextFieldsAppender`.
2. `Benchmark_Log_Parallel_OtelCtx` exists under `//go:build otel_bench` tag.
3. `examples/otel-appender/main.go` compiles with `go build ./examples/otel-appender/`.
4. `BenchmarkLognugget_Parallel_10CtxFields`: ≤ 1 alloc/op.
5. Existing parser tests and `Test_ContextFields_LegacyParser` pass unchanged (parser still works).

## Assignment

Assigned to: agent-engineer
Branch: feat/118-p7-otel-context-appender
Target PR: feat/111-v3-performance
Date: 2026-05-22
PL instruction: Write the tests first. The key test is `Test_ContextFieldsAppender_10Fields` verifying output correctness. The benchmark update is the main deliverable — confirm ≤ 1 alloc/op on `BenchmarkLognugget_Parallel_10CtxFields` before marking ready. The OTel example must have its own go.mod and must not add any dependencies to the core module.
