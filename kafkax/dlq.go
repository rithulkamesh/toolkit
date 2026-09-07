package kafkax

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"time"
)

// Dead-letter headers added to a message when it is routed to the DLQ.
const (
	HeaderDLQError           = "x-dlq-error"
	HeaderDLQOriginTopic     = "x-dlq-origin-topic"
	HeaderDLQOriginPartition = "x-dlq-origin-partition"
	HeaderDLQOriginOffset    = "x-dlq-origin-offset"
	HeaderDLQAttempts        = "x-dlq-attempts"
	HeaderDLQFailedAt        = "x-dlq-failed-at"
)

// DLQ wraps a [Handler] so that a message which keeps failing is republished
// to a dead-letter topic instead of stalling the partition.
//
//	h := kafkax.DLQ{Producer: prod, MaxRetries: 3}.Wrap(processOrder)
//	consumer.Run(ctx, h)
//
// On error, Wrap retries in-process (immediately) up to MaxRetries. If it still
// fails, the original message — annotated with x-dlq-* headers — is published
// to Topic(originTopic) and the handler returns nil, letting the consumer
// commit and advance. A publish failure is returned so the consumer can stop
// rather than silently drop the message.
type DLQ struct {
	// Producer receives dead-lettered messages. Required.
	Producer Producer
	// Topic maps an origin topic to its DLQ topic. Defaults to origin+".DLQ".
	Topic func(origin string) string
	// MaxRetries is the number of extra in-process attempts after the first
	// failure. 0 dead-letters on the first error.
	MaxRetries int
	// ShouldDLQ decides, per error, whether to dead-letter (true) or propagate
	// the error to the consumer (false). Defaults to always dead-letter.
	ShouldDLQ func(error) bool
	// OnDLQ, if set, is called just before a message is published to the DLQ.
	OnDLQ func(d Delivery, err error)
}

// Wrap returns the wrapped [Handler]. It panics if Producer is nil.
func (q DLQ) Wrap(next Handler) Handler {
	if q.Producer == nil {
		panic("kafkax: DLQ.Wrap needs a Producer")
	}
	topicFn := q.Topic
	if topicFn == nil {
		topicFn = func(origin string) string { return origin + ".DLQ" }
	}
	shouldDLQ := q.ShouldDLQ
	if shouldDLQ == nil {
		shouldDLQ = func(error) bool { return true }
	}

	return HandlerFunc(func(ctx context.Context, d Delivery) error {
		var err error
		for attempt := 0; attempt <= q.MaxRetries; attempt++ {
			err = next.Handle(ctx, d)
			if err == nil || errors.Is(err, ErrDropped) {
				return nil
			}
			if ctx.Err() != nil {
				return err
			}
		}
		if !shouldDLQ(err) {
			return err
		}
		if q.OnDLQ != nil {
			q.OnDLQ(d, err)
		}

		dead := d.Envelope
		dead.SetHeader(HeaderDLQError, err.Error())
		dead.SetHeader(HeaderDLQOriginTopic, d.Topic)
		dead.SetHeader(HeaderDLQOriginPartition, strconv.Itoa(int(d.Partition)))
		dead.SetHeader(HeaderDLQOriginOffset, strconv.FormatInt(d.Offset, 10))
		dead.SetHeader(HeaderDLQAttempts, strconv.Itoa(q.MaxRetries+1))
		dead.SetHeader(HeaderDLQFailedAt, time.Now().UTC().Format(time.RFC3339))

		if perr := q.Producer.Publish(ctx, topicFn(d.Topic), dead); perr != nil {
			return fmt.Errorf("kafkax: dead-letter publish for %s failed (original error: %v): %w", d.ID, err, perr)
		}
		return nil
	})
}

// ErrDropped can be returned by a [Handler] to signal "discard without
// dead-lettering". DLQ.Wrap treats it as success.
var ErrDropped = errors.New("kafkax: message dropped by handler")
