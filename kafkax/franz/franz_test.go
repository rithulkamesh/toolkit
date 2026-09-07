package franz

import (
	"testing"
	"time"

	"github.com/twmb/franz-go/pkg/kgo"

	"github.com/rithulkamesh/toolkit/kafkax"
)

func TestRecordEnvelopeRoundTrip(t *testing.T) {
	p := &Producer{source: "billing"}

	in := kafkax.NewEnvelope("invoice.paid", []byte(`{"amount":100}`))
	in.Subject = "inv_42"
	in.Key = []byte("inv_42")
	in.SetHeader("x-tenant", "acme")
	tc := kafkax.TraceContext{
		TraceID:  "4bf92f3577b34da6a3ce929d0e0e4736",
		ParentID: "00f067aa0ba902b7",
		Sampled:  true,
	}
	tc.Inject(&in)

	rec := p.record("invoices", in)
	if rec.Topic != "invoices" || string(rec.Key) != "inv_42" || string(rec.Value) != `{"amount":100}` {
		t.Fatalf("record core fields wrong: %+v", rec)
	}

	out := envelope(rec)
	if out.ID != in.ID || out.Type != "invoice.paid" || out.Source != "billing" || out.Subject != "inv_42" {
		t.Fatalf("envelope meta lost: %+v", out)
	}
	if out.Header("x-tenant") != "acme" {
		t.Fatalf("user header lost: %+v", out.Headers)
	}
	if got, ok := kafkax.ExtractTraceContext(out); !ok || got.TraceID != tc.TraceID || !got.Sampled {
		t.Fatalf("trace context did not survive: %+v ok=%v", got, ok)
	}
	if out.Time.IsZero() || !out.Time.Equal(in.Time.Truncate(0)) && out.Time.Sub(in.Time).Abs() > time.Second {
		t.Fatalf("time not carried: in=%v out=%v", in.Time, out.Time)
	}
}

func TestEnvelopeFallbackID(t *testing.T) {
	rec := &kgo.Record{Topic: "t", Partition: 2, Offset: 99, Value: []byte("x")}
	if got := envelope(rec).ID; got != "t-2-99" {
		t.Fatalf("fallback id = %q, want t-2-99", got)
	}
}

func TestNewConsumerValidates(t *testing.T) {
	if _, err := NewConsumer(Options{Brokers: []string{"b:9092"}, Topics: []string{"t"}}); err == nil {
		t.Error("missing Group should error")
	}
	if _, err := NewConsumer(Options{Brokers: []string{"b:9092"}, Group: "g"}); err == nil {
		t.Error("missing Topics should error")
	}
	if _, err := NewConsumer(Options{Group: "g", Topics: []string{"t"}}); err == nil {
		t.Error("missing Brokers should error")
	}
}
