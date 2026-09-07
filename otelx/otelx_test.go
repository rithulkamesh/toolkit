package otelx

import (
	"bytes"
	"context"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

func TestSetupNone(t *testing.T) {
	ctx := context.Background()
	tel, err := Setup(ctx, Config{ServiceName: "svc", TracesExporter: None, MetricsExporter: None})
	if err != nil {
		t.Fatal(err)
	}
	if tel.Tracer == nil || tel.Meter == nil {
		t.Fatal("Tracer/Meter must be usable even with the None exporter")
	}
	if tel.MetricsHandler != nil {
		t.Fatal("None metrics exporter should not produce a handler")
	}

	// Using the instruments must not panic.
	_, span := tel.Tracer.Start(ctx, "op")
	span.End()
	c, err := tel.Meter.Int64Counter("noop_counter")
	if err != nil {
		t.Fatal(err)
	}
	c.Add(ctx, 1)

	if err := tel.Shutdown(ctx); err != nil {
		t.Fatal(err)
	}
	if err := tel.Shutdown(ctx); err != nil {
		t.Fatalf("Shutdown must be idempotent, got %v", err)
	}
}

func TestSetupPrometheus(t *testing.T) {
	ctx := context.Background()
	tel, err := Setup(ctx, Config{
		ServiceName:     "svc",
		TracesExporter:  None,
		MetricsExporter: Prometheus,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer tel.Shutdown(ctx)

	if tel.MetricsHandler == nil {
		t.Fatal("Prometheus metrics exporter must expose a handler")
	}

	counter, err := tel.Meter.Int64Counter("widgets_made_total")
	if err != nil {
		t.Fatal(err)
	}
	counter.Add(ctx, 7)

	rec := httptest.NewRecorder()
	tel.MetricsHandler.ServeHTTP(rec, httptest.NewRequest("GET", "/metrics", nil))
	if rec.Code != 200 {
		t.Fatalf("scrape status = %d", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "widgets_made_total") {
		t.Fatalf("scrape missing the metric:\n%s", body)
	}
}

func TestSetupStdout(t *testing.T) {
	ctx := context.Background()
	var mu sync.Mutex
	buf := &lockedBuffer{mu: &mu}

	tel, err := Setup(ctx, Config{
		ServiceName:     "svc",
		TracesExporter:  Stdout,
		MetricsExporter: Stdout,
		Writer:          buf,
	})
	if err != nil {
		t.Fatal(err)
	}

	_, span := tel.Tracer.Start(ctx, "unit-work")
	span.End()
	counter, _ := tel.Meter.Int64Counter("things_total")
	counter.Add(ctx, 1)

	if err := tel.Shutdown(ctx); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	if !strings.Contains(out, "unit-work") {
		t.Fatalf("stdout trace exporter wrote no span:\n%s", out)
	}
	if !strings.Contains(out, "things_total") {
		t.Fatalf("stdout metric exporter wrote no metric:\n%s", out)
	}
}

func TestSetupValidation(t *testing.T) {
	ctx := context.Background()
	if _, err := Setup(ctx, Config{}); err == nil {
		t.Error("missing ServiceName should error")
	}
	if _, err := Setup(ctx, Config{ServiceName: "s", TracesExporter: Prometheus, MetricsExporter: None}); err == nil {
		t.Error("Prometheus is not a valid traces exporter")
	}
	if _, err := Setup(ctx, Config{ServiceName: "s", TracesExporter: "carrier-pigeon", MetricsExporter: None}); err == nil {
		t.Error("unknown traces exporter should error")
	}
	if _, err := Setup(ctx, Config{ServiceName: "s", TracesExporter: None, MetricsExporter: "smoke-signal"}); err == nil {
		t.Error("unknown metrics exporter should error")
	}
}

func TestDefaultsExporterSelection(t *testing.T) {
	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "")
	t.Setenv("OTEL_EXPORTER_OTLP_TRACES_ENDPOINT", "")
	t.Setenv("OTEL_EXPORTER_OTLP_METRICS_ENDPOINT", "")

	cfg := Config{ServiceName: "s"}
	cfg.defaults()
	if cfg.TracesExporter != None || cfg.MetricsExporter != None {
		t.Fatalf("no endpoint => None, got traces=%q metrics=%q", cfg.TracesExporter, cfg.MetricsExporter)
	}

	cfg2 := Config{ServiceName: "s", OTLPEndpoint: "collector:4317"}
	cfg2.defaults()
	if cfg2.TracesExporter != OTLPGRPC || cfg2.MetricsExporter != OTLPGRPC {
		t.Fatalf("endpoint set => OTLPGRPC, got traces=%q metrics=%q", cfg2.TracesExporter, cfg2.MetricsExporter)
	}
}

func TestSampleRatio(t *testing.T) {
	cases := map[float64]float64{-1: 1, 0: 1, 0.25: 0.25, 1: 1, 2: 1}
	for in, want := range cases {
		if got := sampleRatio(in); got != want {
			t.Errorf("sampleRatio(%v) = %v, want %v", in, got, want)
		}
	}
}

type lockedBuffer struct {
	mu  *sync.Mutex
	buf bytes.Buffer
}

func (b *lockedBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *lockedBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}
