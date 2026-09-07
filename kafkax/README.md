# kafkax

`import "github.com/rithulkamesh/toolkit/kafkax"`

Transport-agnostic core for event messaging. Standard library only.

- **`Envelope`** — a CloudEvents-shaped message: `ID`, `Type`, `Source`,
  `Subject`, `Time`, `Key`, `Headers`, `Data`. `NewEnvelope`, `SetJSON`/`Bind`.
- **`Producer` / `Consumer`** — the two interfaces a broker binding implements.
  A `Consumer` dispatches each message to a `Handler` as a `Delivery`
  (`Envelope` + topic / partition / offset).
- **W3C trace context** — `TraceContext` parses and renders `traceparent`,
  `Inject`/`ExtractTraceContext` move it across a broker hop on message headers,
  with no tracing-SDK dependency.
- **`DLQ`** — wraps a `Handler` so a message that keeps failing is retried
  in-process, then republished to a dead-letter topic with `x-dlq-*` headers,
  instead of stalling the partition.

```go
h := kafkax.DLQ{Producer: prod, MaxRetries: 3}.Wrap(kafkax.HandlerFunc(
	func(ctx context.Context, d kafkax.Delivery) error {
		if tc, ok := kafkax.ExtractTraceContext(d.Envelope); ok {
			ctx = startConsumerSpan(ctx, tc)
		}
		return process(ctx, d)
	},
))
consumer.Run(ctx, h)
```

Broker binding: [`kafkax/franz`](./franz) (franz-go).
