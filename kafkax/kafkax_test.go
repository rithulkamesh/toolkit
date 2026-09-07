package kafkax

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"
)

func TestEnvelopeJSONAndBind(t *testing.T) {
	type payload struct {
		Name string `json:"name"`
	}
	e := NewEnvelope("user.created", nil)
	if e.ID == "" || e.Time.IsZero() {
		t.Fatal("NewEnvelope should set ID and Time")
	}
	if err := e.SetJSON(payload{Name: "ada"}); err != nil {
		t.Fatal(err)
	}

	raw, err := json.Marshal(e)
	if err != nil {
		t.Fatal(err)
	}
	var got Envelope
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatal(err)
	}
	var p payload
	if err := got.Bind(&p); err != nil || p.Name != "ada" {
		t.Fatalf("Bind = %+v, %v", p, err)
	}
	if got.ID != e.ID || got.Type != "user.created" {
		t.Fatalf("round-trip lost fields: %+v", got)
	}
}

func TestNewEnvelopeIDsUnique(t *testing.T) {
	seen := make(map[string]bool)
	for i := 0; i < 1000; i++ {
		id := NewEnvelope("t", nil).ID
		if seen[id] {
			t.Fatalf("duplicate id %q", id)
		}
		seen[id] = true
	}
}

func TestTraceParentRoundTrip(t *testing.T) {
	in := "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01"
	tc, err := ParseTraceParent(in)
	if err != nil {
		t.Fatal(err)
	}
	if tc.TraceID != "4bf92f3577b34da6a3ce929d0e0e4736" || tc.ParentID != "00f067aa0ba902b7" || !tc.Sampled {
		t.Fatalf("parsed wrong: %+v", tc)
	}
	if got := tc.TraceParent(); got != in {
		t.Fatalf("TraceParent() = %q, want %q", got, in)
	}
}

func TestParseTraceParentRejects(t *testing.T) {
	bad := []string{
		"",
		"00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7",    // 3 fields
		"01-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01", // version
		"00-00000000000000000000000000000000-00f067aa0ba902b7-01", // zero trace-id
		"00-4bf92f3577b34da6a3ce929d0e0e4736-0000000000000000-01", // zero parent-id
		"00-xyz92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01", // non-hex
	}
	for _, s := range bad {
		if _, err := ParseTraceParent(s); err == nil {
			t.Errorf("ParseTraceParent(%q) should have failed", s)
		}
	}
}

func TestTraceContextInjectExtract(t *testing.T) {
	tc := TraceContext{
		TraceID:  "4bf92f3577b34da6a3ce929d0e0e4736",
		ParentID: "00f067aa0ba902b7",
		Sampled:  true,
		State:    "vendor=abc",
	}
	e := NewEnvelope("t", nil)
	tc.Inject(&e)

	got, ok := ExtractTraceContext(e)
	if !ok {
		t.Fatal("ExtractTraceContext returned ok=false")
	}
	if got.TraceID != tc.TraceID || got.ParentID != tc.ParentID || !got.Sampled || got.State != "vendor=abc" {
		t.Fatalf("extracted %+v, want %+v", got, tc)
	}

	if _, ok := ExtractTraceContext(NewEnvelope("t", nil)); ok {
		t.Fatal("no traceparent header should give ok=false")
	}
}

// recordingProducer captures Publish calls.
type recordingProducer struct {
	mu   sync.Mutex
	sent []struct {
		topic string
		msg   Envelope
	}
	err error
}

func (p *recordingProducer) Publish(_ context.Context, topic string, msgs ...Envelope) error {
	if p.err != nil {
		return p.err
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	for _, m := range msgs {
		p.sent = append(p.sent, struct {
			topic string
			msg   Envelope
		}{topic, m})
	}
	return nil
}
func (p *recordingProducer) Close() error { return nil }

func TestDLQPassthroughOnSuccess(t *testing.T) {
	prod := &recordingProducer{}
	var calls int
	h := DLQ{Producer: prod, MaxRetries: 3}.Wrap(HandlerFunc(func(context.Context, Delivery) error {
		calls++
		return nil
	}))
	if err := h.Handle(context.Background(), Delivery{Topic: "orders"}); err != nil {
		t.Fatal(err)
	}
	if calls != 1 || len(prod.sent) != 0 {
		t.Fatalf("calls=%d sent=%d, want 1 and 0", calls, len(prod.sent))
	}
}

func TestDLQRoutesAfterRetries(t *testing.T) {
	prod := &recordingProducer{}
	var calls int
	boom := errors.New("processing failed")
	h := DLQ{Producer: prod, MaxRetries: 2}.Wrap(HandlerFunc(func(context.Context, Delivery) error {
		calls++
		return boom
	}))

	d := Delivery{Envelope: NewEnvelope("order.placed", []byte("{}")), Topic: "orders", Partition: 3, Offset: 42}
	if err := h.Handle(context.Background(), d); err != nil {
		t.Fatalf("DLQ.Wrap should absorb the error, got %v", err)
	}
	if calls != 3 {
		t.Fatalf("attempts = %d, want 3 (1 + MaxRetries)", calls)
	}
	if len(prod.sent) != 1 {
		t.Fatalf("expected 1 dead-letter, got %d", len(prod.sent))
	}
	got := prod.sent[0]
	if got.topic != "orders.DLQ" {
		t.Fatalf("dlq topic = %q", got.topic)
	}
	if got.msg.Header(HeaderDLQError) != "processing failed" ||
		got.msg.Header(HeaderDLQOriginTopic) != "orders" ||
		got.msg.Header(HeaderDLQOriginPartition) != "3" ||
		got.msg.Header(HeaderDLQOriginOffset) != "42" ||
		got.msg.Header(HeaderDLQAttempts) != "3" {
		t.Fatalf("dlq headers wrong: %v", got.msg.Headers)
	}
}

func TestDLQDroppedIsSuccess(t *testing.T) {
	prod := &recordingProducer{}
	h := DLQ{Producer: prod}.Wrap(HandlerFunc(func(context.Context, Delivery) error {
		return ErrDropped
	}))
	if err := h.Handle(context.Background(), Delivery{Topic: "t"}); err != nil {
		t.Fatal(err)
	}
	if len(prod.sent) != 0 {
		t.Fatal("ErrDropped must not dead-letter")
	}
}

func TestDLQShouldDLQFalsePropagates(t *testing.T) {
	prod := &recordingProducer{}
	boom := errors.New("transient")
	h := DLQ{Producer: prod, ShouldDLQ: func(error) bool { return false }}.
		Wrap(HandlerFunc(func(context.Context, Delivery) error { return boom }))
	if err := h.Handle(context.Background(), Delivery{Topic: "t"}); !errors.Is(err, boom) {
		t.Fatalf("want the original error back, got %v", err)
	}
	if len(prod.sent) != 0 {
		t.Fatal("ShouldDLQ=false must not dead-letter")
	}
}

func TestDLQPublishFailureSurfaces(t *testing.T) {
	prod := &recordingProducer{err: errors.New("broker down")}
	h := DLQ{Producer: prod}.Wrap(HandlerFunc(func(context.Context, Delivery) error {
		return errors.New("handler failed")
	}))
	err := h.Handle(context.Background(), Delivery{Envelope: NewEnvelope("t", nil), Topic: "t"})
	if err == nil {
		t.Fatal("a failed dead-letter publish must be returned, not swallowed")
	}
}
