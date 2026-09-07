// Package kafkax is the transport-agnostic core for event messaging: a
// CloudEvents-shaped [Envelope], [Producer] and [Consumer] interfaces, W3C
// trace-context propagation over message headers, and a dead-letter-queue
// [Handler] wrapper.
//
// It imports nothing outside the standard library. A concrete broker binding —
// github.com/rithulkamesh/toolkit/kafkax/franz for franz-go — implements
// [Producer] and [Consumer]; tests and in-process fan-out can implement them
// directly.
package kafkax

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"
)

// Envelope is a single message: metadata plus an opaque payload. The field
// names mirror the CloudEvents attributes so a fleet of services can agree on
// one shape.
type Envelope struct {
	ID      string            `json:"id"`
	Type    string            `json:"type"`
	Source  string            `json:"source,omitempty"`
	Subject string            `json:"subject,omitempty"`
	Time    time.Time         `json:"time"`
	Key     []byte            `json:"key,omitempty"`
	Headers map[string]string `json:"headers,omitempty"`
	Data    []byte            `json:"data,omitempty"`
}

// NewEnvelope returns an Envelope of the given type carrying data, with a fresh
// random [Envelope.ID] and [Envelope.Time] set to now.
func NewEnvelope(typ string, data []byte) Envelope {
	return Envelope{ID: newID(), Type: typ, Time: time.Now().UTC(), Data: data}
}

// SetJSON marshals v into [Envelope.Data] and is the counterpart of
// [Envelope.Bind].
func (e *Envelope) SetJSON(v any) error {
	b, err := json.Marshal(v)
	if err != nil {
		return fmt.Errorf("kafkax: encode payload: %w", err)
	}
	e.Data = b
	return nil
}

// Bind unmarshals [Envelope.Data] into v.
func (e Envelope) Bind(v any) error {
	if err := json.Unmarshal(e.Data, v); err != nil {
		return fmt.Errorf("kafkax: decode payload: %w", err)
	}
	return nil
}

// Header returns the named header, or "".
func (e Envelope) Header(key string) string { return e.Headers[key] }

// SetHeader sets a header, allocating the map on first use.
func (e *Envelope) SetHeader(key, value string) {
	if e.Headers == nil {
		e.Headers = make(map[string]string, 4)
	}
	e.Headers[key] = value
}

// Delivery is an [Envelope] received from a topic, with the broker coordinates
// a [Handler] needs for routing decisions (notably dead-lettering).
type Delivery struct {
	Envelope
	Topic     string
	Partition int32
	Offset    int64
}

// Producer publishes envelopes to a topic.
type Producer interface {
	// Publish sends every msg to topic and returns once the broker has
	// acknowledged them, or at the first error.
	Publish(ctx context.Context, topic string, msgs ...Envelope) error
	Close() error
}

// Consumer pulls messages from one or more topics and dispatches them to a
// [Handler].
type Consumer interface {
	// Run blocks, calling h.Handle for each message until ctx is cancelled
	// (returns nil) or a fatal transport error occurs. Offset commits happen
	// after Handle returns nil; a non-nil error is left to the Handler to
	// absorb (see DLQ.Wrap) — an error that reaches Run stops it.
	Run(ctx context.Context, h Handler) error
	Close() error
}

// Handler processes one delivered message.
type Handler interface {
	Handle(ctx context.Context, d Delivery) error
}

// HandlerFunc adapts a function to [Handler].
type HandlerFunc func(ctx context.Context, d Delivery) error

// Handle calls f.
func (f HandlerFunc) Handle(ctx context.Context, d Delivery) error { return f(ctx, d) }

func newID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		// crypto/rand should never fail; fall back to a time-based id.
		return fmt.Sprintf("t-%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(b[:])
}
