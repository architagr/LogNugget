# LogNugget cookbook

Six runnable programs, smallest first. Each one prints what it does, so you can
read the source and the output side by side.

```bash
cd examples/cookbook
go run ./01-quickstart
```

| # | Example | Answers |
|---|---------|---------|
| 01 | [`01-quickstart`](01-quickstart) | What is the least I have to write? |
| 02 | [`02-fields`](02-fields) | How do I attach data to a record — and what if I have none? |
| 03 | [`03-context`](03-context) | How do trace IDs and per-request fields get in? |
| 04 | [`04-configuration`](04-configuration) | What can I configure, and what does each knob change? |
| 05 | [`05-hooks`](05-hooks) | How do I send records to more than one place? |
| 06 | [`06-tuning`](06-tuning) | How do I trade latency for fewer writes? |

## 01 — quickstart

```go
import (
    "github.com/architagr/lognugget/entry"
    "github.com/architagr/lognugget/lognugget"
)

func main() {
    defer lognugget.Shutdown()
    entry.NewLogEntry().Info(context.Background(), "server started")
}
```

Importing `lognugget` builds the pipeline. `Shutdown` drains it. That is the
whole setup.

## 02 — fields

Three ways to attach data:

- **Typed chain methods** — `Str`, `Int`, `Uint`, `Float64`, `Bool`, `Err`,
  `Any`. These write straight into the record buffer with no boxing. Use these.
- **Variadic `model.LogAttr`** — for fields assembled elsewhere and passed as a
  slice.
- **No fields at all** — a bare message is a complete record.

A field whose key collides with a core key (`time`, `level`, `message`,
`error`, `caller`) is written as `custom.<key>` rather than emitted twice.

## 03 — context

Three strategies, highest precedence first:

| Setter | Shape | When |
|--------|-------|------|
| `SetContextFieldsAppender` | `func(ctx, []byte) []byte` | Fixed-shape fields, you own the escaping |
| `SetContextFields` | `func(ctx, *config.CtxFields)` | **Recommended.** Typed methods, encoding handled for you |
| `SetContextFieldsParser` | `func(ctx) map[string]any` | Legacy. Allocates a map per record |

Register one at startup, not per request.

## 04 — configuration

Every setter is optional; the defaults work unconfigured. They are
startup-time settings.

| Setter | Default | Changes |
|--------|---------|---------|
| `SetMinLevel` | `Info` | Records below this are rejected by an atomic gate |
| `SetTimeFormat` | `time.RFC3339` | Timestamp layout |
| `SetAddSource` | `false` | Adds the call site — the most expensive option there is |
| `SetEncoderType` | `JSON` | `JSON` or `Text` |
| `SetDefaultFields` | built-ins | Renames core keys to match an existing schema |
| `SetStaticEnvFieldsParser` | none | Fields evaluated once, included in every record |
| `SetOutput` | `os.Stdout` | Where the built-in collector writes |

## 05 — hooks

A hook is `PublishLogMessage([]byte)` plus `Name() string`. Register it against
a level, or against `LevelUnSet` to receive everything:

```go
pipelineStage.EventPreProcessorObj.RegisterHook(enum.LevelError, errorHook)
```

Records fan out to every matching hook, so an error reaches both the
`LevelUnSet` hooks and the `LevelError` ones. Registering a hook whose `Name()`
matches an existing one at that level replaces it.

**The `[]byte` a hook receives is borrowed.** The dispatcher recycles that
buffer as soon as the call returns; copy the bytes if you keep them.

## 06 — tuning

Three knobs decide how long a record waits and how many writes it costs:

| Knob | Meaning |
|------|---------|
| `entry.GenerateInitialPool(n)` | Pre-allocated `LogEntry` objects. `GOMAXPROCS × 64` is a good default |
| `config.SetLogBufferMaxSize(n)` | Flush once `n` records are buffered |
| `config.SetRate(d)` | Flush at least every `d` |

Whichever trigger fires first wins. From the example's own output — 500
records, one collector:

```
configuration                        writes      bytes
bucket=1   rate=1s   (no batching)      500      41390
bucket=20  rate=1s   (default)           25      41390
bucket=500 rate=5s   (throughput)         1      41390
```

Each flush is a single `Write` carrying the whole newline-delimited batch, so a
bigger bucket means proportionally fewer writes for the same bytes.

Whatever you pick, call `lognugget.Shutdown()` before exit: with a 5 s rate, a
clean exit can still drop five seconds of logs.
