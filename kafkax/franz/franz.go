// Package franz binds github.com/rithulkamesh/toolkit/kafkax to franz-go,
// implementing [kafkax.Producer] and [kafkax.Consumer] against a real Kafka
// cluster.
//
//	prod, err := franz.NewProducer(franz.Options{
//		Brokers: []string{"kafka:9092"},
//		Source:  "billing",
//	})
//	cons, err := franz.NewConsumer(franz.Options{
//		Brokers: []string{"kafka:9092"},
//		Group:   "billing-workers",
//		Topics:  []string{"orders"},
//	})
//	cons.Run(ctx, kafkax.DLQ{Producer: prod, MaxRetries: 3}.Wrap(handle))
package franz

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/twmb/franz-go/pkg/kgo"

	"github.com/rithulkamesh/toolkit/kafkax"
)

// Envelope metadata is carried as record headers so non-kafkax consumers can
// still read it.
const (
	metaID      = "ce_id"
	metaType    = "ce_type"
	metaSource  = "ce_source"
	metaSubject = "ce_subject"
	metaTime    = "ce_time"
)

// Options configures [NewProducer] and [NewConsumer]. Group and Topics apply
// to consumers only.
type Options struct {
	Brokers  []string
	ClientID string

	// Source stamps the ce_source header on produced envelopes that don't set
	// their own Source.
	Source string

	// Group is the consumer group id. Required for [NewConsumer].
	Group string
	// Topics to subscribe to. Required for [NewConsumer].
	Topics []string

	// TLS, when non-nil, dials brokers over TLS with this config.
	TLS *tls.Config

	// Extra is an escape hatch for franz-go options not surfaced here (SASL,
	// tuning, instrumentation). Applied last, so it can override.
	Extra []kgo.Opt
}

func (o Options) baseOpts() ([]kgo.Opt, error) {
	if len(o.Brokers) == 0 {
		return nil, errors.New("kafkax/franz: no brokers")
	}
	opts := []kgo.Opt{kgo.SeedBrokers(o.Brokers...)}
	if o.ClientID != "" {
		opts = append(opts, kgo.ClientID(o.ClientID))
	}
	if o.TLS != nil {
		opts = append(opts, kgo.DialTLSConfig(o.TLS))
	}
	return opts, nil
}

// Producer implements [kafkax.Producer].
type Producer struct {
	cl     *kgo.Client
	source string
}

// NewProducer connects a producer.
func NewProducer(o Options) (*Producer, error) {
	opts, err := o.baseOpts()
	if err != nil {
		return nil, err
	}
	opts = append(opts, o.Extra...)
	cl, err := kgo.NewClient(opts...)
	if err != nil {
		return nil, fmt.Errorf("kafkax/franz: new client: %w", err)
	}
	return &Producer{cl: cl, source: o.Source}, nil
}

// Publish sends msgs to topic and blocks until the broker acknowledges them.
func (p *Producer) Publish(ctx context.Context, topic string, msgs ...kafkax.Envelope) error {
	if len(msgs) == 0 {
		return nil
	}
	recs := make([]*kgo.Record, len(msgs))
	for i, m := range msgs {
		recs[i] = p.record(topic, m)
	}
	if err := p.cl.ProduceSync(ctx, recs...).FirstErr(); err != nil {
		return fmt.Errorf("kafkax/franz: publish to %s: %w", topic, err)
	}
	return nil
}

// Close flushes and shuts the client down.
func (p *Producer) Close() error { p.cl.Close(); return nil }

func (p *Producer) record(topic string, e kafkax.Envelope) *kgo.Record {
	source := e.Source
	if source == "" {
		source = p.source
	}

	headers := make([]kgo.RecordHeader, 0, len(e.Headers)+5)
	// User headers first, sorted for deterministic output.
	keys := make([]string, 0, len(e.Headers))
	for k := range e.Headers {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		headers = append(headers, kgo.RecordHeader{Key: k, Value: []byte(e.Headers[k])})
	}
	addMeta := func(k, v string) {
		if v != "" {
			headers = append(headers, kgo.RecordHeader{Key: k, Value: []byte(v)})
		}
	}
	addMeta(metaID, e.ID)
	addMeta(metaType, e.Type)
	addMeta(metaSource, source)
	addMeta(metaSubject, e.Subject)
	if !e.Time.IsZero() {
		addMeta(metaTime, e.Time.UTC().Format(time.RFC3339Nano))
	}

	rec := &kgo.Record{Topic: topic, Key: e.Key, Value: e.Data, Headers: headers}
	if !e.Time.IsZero() {
		rec.Timestamp = e.Time
	}
	return rec
}

// Consumer implements [kafkax.Consumer].
type Consumer struct {
	cl *kgo.Client
}

// NewConsumer connects a consumer. Options.Group and Options.Topics are required.
func NewConsumer(o Options) (*Consumer, error) {
	if o.Group == "" {
		return nil, errors.New("kafkax/franz: Options.Group is required for a consumer")
	}
	if len(o.Topics) == 0 {
		return nil, errors.New("kafkax/franz: Options.Topics is required for a consumer")
	}
	opts, err := o.baseOpts()
	if err != nil {
		return nil, err
	}
	opts = append(opts,
		kgo.ConsumerGroup(o.Group),
		kgo.ConsumeTopics(o.Topics...),
		kgo.DisableAutoCommit(), // we commit after each batch is handled
	)
	opts = append(opts, o.Extra...)
	cl, err := kgo.NewClient(opts...)
	if err != nil {
		return nil, fmt.Errorf("kafkax/franz: new client: %w", err)
	}
	return &Consumer{cl: cl}, nil
}

// Run polls for records and dispatches each to h, committing a batch's offsets
// once every record in it has been handled without error. It returns nil when
// ctx is cancelled, and an error on a fatal fetch, a handler error, or a
// commit failure.
func (c *Consumer) Run(ctx context.Context, h kafkax.Handler) error {
	for {
		if ctx.Err() != nil {
			return nil
		}
		fetches := c.cl.PollFetches(ctx)
		if fetches.IsClientClosed() {
			return nil
		}
		if errs := fetches.Errors(); len(errs) > 0 {
			if ctx.Err() != nil {
				return nil
			}
			return fmt.Errorf("kafkax/franz: fetch %s: %w", errs[0].Topic, errs[0].Err)
		}

		var batch []*kgo.Record
		fetches.EachRecord(func(rec *kgo.Record) { batch = append(batch, rec) })

		for _, rec := range batch {
			d := kafkax.Delivery{
				Envelope:  envelope(rec),
				Topic:     rec.Topic,
				Partition: rec.Partition,
				Offset:    rec.Offset,
			}
			if err := h.Handle(ctx, d); err != nil {
				return fmt.Errorf("kafkax/franz: handler for %s[%d]@%d: %w", rec.Topic, rec.Partition, rec.Offset, err)
			}
		}
		if len(batch) > 0 {
			if err := c.cl.CommitRecords(ctx, batch...); err != nil {
				return fmt.Errorf("kafkax/franz: commit: %w", err)
			}
		}
	}
}

// Close leaves the group and shuts the client down.
func (c *Consumer) Close() error { c.cl.Close(); return nil }

func envelope(rec *kgo.Record) kafkax.Envelope {
	e := kafkax.Envelope{
		Key:  rec.Key,
		Data: rec.Value,
		Time: rec.Timestamp,
	}
	for _, hdr := range rec.Headers {
		v := string(hdr.Value)
		switch hdr.Key {
		case metaID:
			e.ID = v
		case metaType:
			e.Type = v
		case metaSource:
			e.Source = v
		case metaSubject:
			e.Subject = v
		case metaTime:
			if t, err := time.Parse(time.RFC3339Nano, v); err == nil {
				e.Time = t
			}
		default:
			e.SetHeader(hdr.Key, v)
		}
	}
	if e.ID == "" {
		e.ID = fmt.Sprintf("%s-%d-%d", rec.Topic, rec.Partition, rec.Offset)
	}
	return e
}
