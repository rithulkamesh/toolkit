# kafkax/franz

`import "github.com/rithulkamesh/toolkit/kafkax/franz"`

Binds [`kafkax`](..) to [franz-go](https://github.com/twmb/franz-go):
`Producer` and `Consumer` against a real Kafka cluster.

```go
prod, err := franz.NewProducer(franz.Options{
	Brokers: []string{"kafka:9092"},
	Source:  "billing",
})
cons, err := franz.NewConsumer(franz.Options{
	Brokers: []string{"kafka:9092"},
	Group:   "billing-workers",
	Topics:  []string{"orders"},
})

cons.Run(ctx, kafkax.DLQ{Producer: prod, MaxRetries: 3}.Wrap(handle))
```

- `Envelope` metadata rides as `ce_*` record headers, so non-kafkax consumers
  still see id / type / source / time.
- The consumer disables auto-commit and commits a fetch batch's offsets only
  after every record in it has been handled without error (at-least-once).
- `Options.Extra []kgo.Opt` is the escape hatch for SASL, TLS, and tuning.
