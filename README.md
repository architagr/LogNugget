[![codecov](https://codecov.io/gh/architagr/LogNugget/branch/main/graph/badge.svg?token=9VPDuFbSyQ)](https://codecov.io/gh/architagr/LogNugget)

# LogNugget

**Bite-sized, context-aware logging for Go** — because every request deserves its own story.

**LogNugget** is a high-performance, memory-efficient logging library for Go, designed to minimize IO bottlenecks and memory pressure while enabling rich contextual logging with trace/span support out of the box.

---

## Why LogNugget?

### Traditional Go loggers often

1. **Block the main application** waiting for synchronous IO writes, slowing down request handling.
2. Lack a **built-in, standardized way to attach trace/span IDs** or cross-request correlation data.
3. Allocate new memory for every log message, increasing GC load during high-throughput logging.

### LogNugget solves these by

- Using a batched event pipeline to group log writes.
- Leveraging sync.Pool to recycle log message objects and reduce GC pressure.
- Separating log construction from log output, minimizing disruptions to application flow.
- Making trace, span, and contextual logging first-class citizens.

---

## Architecture Overview

When the application starts, LogNugget’s `init()` sets up an event pipeline that processes and batches log events before writing them to output streams.

### Event Pipeline Stages

1. `eventPreProcessingStream`

   - Shapes the raw log message into the final format based on configured encoding (JSON or plain text).

   - Applies static and context-derived fields (hostname, request ID, trace ID, etc.).

2. `eventHookProcessingStream`

   - Sends the processed event to:

     - All registered dynamic hooks (custom outputs).
     - A **default collector** (the main buffered sink).

3. Buffered Collectors

   - Each collector maintains a slice of log messages.
   - A ticker periodically swaps the current buffer with a fresh one.
   - The swapped buffer is then asynchronously flushed to the target `io.Writer`.
   - This batching reduces IO calls and prevents stalls in the main application thread.
   - If a buffer reaches maximum capacity before the ticker fires, it’s flushed immediately.

---

## Logger Configuration

Each logger instance can be configured with the following setters:

1. `SetMinLevel(level Level)` – Minimum log level (e.g., Debug, Info, Warn, Error).
2. `SetTimeFormat(format string)` – Custom timestamp format (default: RFC3339).
3. `SetEncoderType(type EncoderType)` – Output encoding: JSON or Text.
4. `SetAddSource(enabled bool)` – Whether to include caller function and file info.
5. `SetOutput(w io.Writer)` – Output target for the default collector.
6. `SetLogBuffer(size int)` – Max buffer size before forced flush.
7. `SetRate(interval time.Duration)` – Flush interval for batched logs.
8. `SetStaticEnvFieldsParser(fn func() map[string]any)` – Attach environment/static fields (hostname, service name, etc.).
9. `SetContextFieldsParser(fn func(ctx context.Context) map[string]any)` – Extract and attach fields from request context (trace ID, user ID, etc.).
10. `SetDefaultFields(mapping map[string]string)` – Rename default log field keys (message → msg, timestamp → ts, etc.).

All these settings have sensible defaults, allowing zero-config usage

---

## Hooks Support

LogNugget supports hooks — custom functions or writers triggered for every processed log event:

- Can be used for sending logs to external systems (ELK, Loki, Datadog, etc.).
- Can run asynchronously to avoid blocking the main app.
- Multiple hooks can be attached dynamically.

---

## Key Advantages

- Non-blocking logging — main flow is never stalled by IO.
- Low GC overhead — sync.Pool ensures message object reuse.
- Rich context — easy trace/span integration.
- Customizable format — JSON or text output with field remapping.
- Batched delivery — reduces IO calls.
- Extensible hooks — plug in any additional log consumers.

---

## Example Usage

```go
logger := lognugget.NewLogger().
    SetMinLevel(lognugget.Info).
    SetEncoderType(lognugget.JSON).
    SetAddSource(true).
    SetLogBuffer(100).
    SetRate(2 * time.Second).
    SetStaticEnvFieldsParser(func() map[string]any {
        return map[string]any{
            "service": "checkout",
            "host":    os.Getenv("HOSTNAME"),
        }
    }).
    SetContextFieldsParser(func(ctx context.Context) map[string]any {
        return map[string]any{
            "trace_id": ctx.Value("trace_id"),
        }
    })

logger.Info(ctx, "Order placed", lognugget.Field("order_id", 12345))
```

---

## Future Enhancements

- OpenTelemetry integration for automated trace/span extraction.
- Configurable log rotation strategies.
- Built-in structured JSON parsing & filtering for high-volume log streams.
  `
