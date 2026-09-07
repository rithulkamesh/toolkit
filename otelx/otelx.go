// Package otelx sets up OpenTelemetry traces and metrics in one call, with a
// switch for the exporter backend.
//
//	tel, err := otelx.Setup(ctx, otelx.Config{
//		ServiceName:     "billing",
//		ServiceVersion:  build.Version,
//		Environment:     "production",
//		TracesExporter:  otelx.OTLPGRPC,
//		MetricsExporter: otelx.Prometheus,
//	})
//	if err != nil {
//		return err
//	}
//	defer tel.Shutdown(context.Background())
//
//	if tel.MetricsHandler != nil {
//		mux.Handle("/metrics", tel.MetricsHandler)
//	}
//
// After Setup the OpenTelemetry globals (TracerProvider, MeterProvider, and a
// tracecontext+baggage propagator) are installed, so any library that calls
// otel.Tracer / otel.Meter is wired up too.
package otelx

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	otelmetric "go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	"go.opentelemetry.io/otel/trace"
)

// Exporter selects a telemetry backend.
type Exporter string

const (
	// None disables the signal (the OpenTelemetry no-op implementation).
	None Exporter = "none"
	// OTLPGRPC exports over OTLP/gRPC (default port 4317).
	OTLPGRPC Exporter = "otlp-grpc"
	// OTLPHTTP exports over OTLP/HTTP protobuf (default port 4318).
	OTLPHTTP Exporter = "otlp-http"
	// Stdout writes to Config.Writer (default os.Stdout). Development only.
	Stdout Exporter = "stdout"
	// Prometheus serves metrics from Providers.MetricsHandler for scraping.
	// Valid for MetricsExporter only.
	Prometheus Exporter = "prometheus"
)

// Config drives [Setup]. Only ServiceName is required.
type Config struct {
	ServiceName    string
	ServiceVersion string
	Environment    string // deployment.environment resource attribute

	// TracesExporter and MetricsExporter pick each signal's backend. Empty
	// means OTLPGRPC when an OTLP endpoint is configured (here or via
	// $OTEL_EXPORTER_OTLP_ENDPOINT), otherwise None.
	TracesExporter  Exporter
	MetricsExporter Exporter

	// OTLPEndpoint overrides $OTEL_EXPORTER_OTLP_ENDPOINT. "host:port" for gRPC,
	// "host:port" or a URL for HTTP.
	OTLPEndpoint string
	// OTLPInsecure disables transport security for OTLP (plaintext gRPC / http://).
	OTLPInsecure bool
	// OTLPHeaders are sent with every OTLP export (auth tokens, tenant ids).
	OTLPHeaders map[string]string

	// SampleRatio is the head-based sampling probability for root spans, 0..1.
	// Zero is treated as 1 (always sample); set a tiny positive number to
	// effectively disable. Child spans follow their parent.
	SampleRatio float64

	// MetricInterval is the OTLP metric push period. Default 60s. Ignored by
	// the Prometheus exporter (pull-based).
	MetricInterval time.Duration

	// ResourceAttributes are merged into the OpenTelemetry resource.
	ResourceAttributes map[string]string

	// Writer receives Stdout-exporter output. Default os.Stdout.
	Writer io.Writer
}

// Providers holds what [Setup] built.
type Providers struct {
	// Tracer and Meter are scoped to the service; they are also installed as
	// the OpenTelemetry globals.
	Tracer trace.Tracer
	Meter  otelmetric.Meter

	// MetricsHandler serves the Prometheus exposition format. Non-nil only when
	// MetricsExporter is Prometheus. Mount it at /metrics.
	MetricsHandler http.Handler

	shutdown []func(context.Context) error
}

// Shutdown flushes and stops every provider Setup created. It is safe to call
// more than once and returns the joined errors of all shutdown steps.
func (p *Providers) Shutdown(ctx context.Context) error {
	fns := p.shutdown
	p.shutdown = nil
	var errs []error
	for i := len(fns) - 1; i >= 0; i-- {
		if err := fns[i](ctx); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

// Setup builds the providers, installs the OpenTelemetry globals, and returns a
// handle whose Shutdown must be called on exit.
func Setup(ctx context.Context, cfg Config) (*Providers, error) {
	if cfg.ServiceName == "" {
		return nil, errors.New("otelx: Config.ServiceName is required")
	}
	cfg.defaults()

	res, err := buildResource(ctx, cfg)
	if err != nil {
		return nil, err
	}

	p := &Providers{}

	tp, err := buildTracerProvider(ctx, cfg, res, p)
	if err != nil {
		return nil, err
	}
	mp, err := buildMeterProvider(ctx, cfg, res, p)
	if err != nil {
		_ = p.Shutdown(ctx)
		return nil, err
	}

	otel.SetTracerProvider(tp)
	otel.SetMeterProvider(mp)
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{}, propagation.Baggage{},
	))
	otel.SetErrorHandler(otel.ErrorHandlerFunc(func(error) {})) // callers own their logging

	p.Tracer = tp.Tracer(cfg.ServiceName)
	p.Meter = mp.Meter(cfg.ServiceName)
	return p, nil
}

func (cfg *Config) defaults() {
	if cfg.MetricInterval <= 0 {
		cfg.MetricInterval = 60 * time.Second
	}
	if cfg.Writer == nil {
		cfg.Writer = os.Stdout
	}
	endpointConfigured := cfg.OTLPEndpoint != "" ||
		os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT") != "" ||
		os.Getenv("OTEL_EXPORTER_OTLP_TRACES_ENDPOINT") != "" ||
		os.Getenv("OTEL_EXPORTER_OTLP_METRICS_ENDPOINT") != ""
	fallback := None
	if endpointConfigured {
		fallback = OTLPGRPC
	}
	if cfg.TracesExporter == "" {
		cfg.TracesExporter = fallback
	}
	if cfg.MetricsExporter == "" {
		cfg.MetricsExporter = fallback
	}
}

func buildResource(ctx context.Context, cfg Config) (*resource.Resource, error) {
	attrs := []attribute.KeyValue{
		attribute.String("service.name", cfg.ServiceName),
	}
	if cfg.ServiceVersion != "" {
		attrs = append(attrs, attribute.String("service.version", cfg.ServiceVersion))
	}
	if cfg.Environment != "" {
		attrs = append(attrs, attribute.String("deployment.environment", cfg.Environment))
	}
	for k, v := range cfg.ResourceAttributes {
		attrs = append(attrs, attribute.String(k, v))
	}
	res, err := resource.New(ctx,
		resource.WithFromEnv(),
		resource.WithTelemetrySDK(),
		resource.WithHost(),
		resource.WithProcessPID(),
		resource.WithProcessRuntimeName(),
		resource.WithProcessRuntimeVersion(),
		resource.WithAttributes(attrs...),
	)
	if err != nil {
		return nil, fmt.Errorf("otelx: build resource: %w", err)
	}
	return res, nil
}
