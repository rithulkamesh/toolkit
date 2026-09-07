package otelx

import (
	"context"
	"fmt"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetricgrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetrichttp"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	promexp "go.opentelemetry.io/otel/exporters/prometheus"
	"go.opentelemetry.io/otel/exporters/stdout/stdoutmetric"
	"go.opentelemetry.io/otel/exporters/stdout/stdouttrace"
	otelmetric "go.opentelemetry.io/otel/metric"
	metricnoop "go.opentelemetry.io/otel/metric/noop"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"
	tracenoop "go.opentelemetry.io/otel/trace/noop"
)

func buildTracerProvider(ctx context.Context, cfg Config, res *resource.Resource, p *Providers) (trace.TracerProvider, error) {
	if cfg.TracesExporter == None {
		return tracenoop.NewTracerProvider(), nil
	}

	var (
		exp sdktrace.SpanExporter
		err error
	)
	switch cfg.TracesExporter {
	case OTLPGRPC:
		opts := []otlptracegrpc.Option{}
		if cfg.OTLPEndpoint != "" {
			opts = append(opts, otlptracegrpc.WithEndpoint(cfg.OTLPEndpoint))
		}
		if cfg.OTLPInsecure {
			opts = append(opts, otlptracegrpc.WithInsecure())
		}
		if len(cfg.OTLPHeaders) > 0 {
			opts = append(opts, otlptracegrpc.WithHeaders(cfg.OTLPHeaders))
		}
		exp, err = otlptracegrpc.New(ctx, opts...)
	case OTLPHTTP:
		opts := []otlptracehttp.Option{}
		if cfg.OTLPEndpoint != "" {
			opts = append(opts, otlptracehttp.WithEndpoint(cfg.OTLPEndpoint))
		}
		if cfg.OTLPInsecure {
			opts = append(opts, otlptracehttp.WithInsecure())
		}
		if len(cfg.OTLPHeaders) > 0 {
			opts = append(opts, otlptracehttp.WithHeaders(cfg.OTLPHeaders))
		}
		exp, err = otlptracehttp.New(ctx, opts...)
	case Stdout:
		exp, err = stdouttrace.New(stdouttrace.WithWriter(cfg.Writer))
	case Prometheus:
		return nil, fmt.Errorf("otelx: TracesExporter %q is invalid (Prometheus exports metrics only)", cfg.TracesExporter)
	default:
		return nil, fmt.Errorf("otelx: unknown TracesExporter %q", cfg.TracesExporter)
	}
	if err != nil {
		return nil, fmt.Errorf("otelx: build trace exporter: %w", err)
	}

	sampler := sdktrace.ParentBased(sdktrace.TraceIDRatioBased(sampleRatio(cfg.SampleRatio)))
	tp := sdktrace.NewTracerProvider(
		sdktrace.WithResource(res),
		sdktrace.WithBatcher(exp),
		sdktrace.WithSampler(sampler),
	)
	p.shutdown = append(p.shutdown, tp.Shutdown)
	return tp, nil
}

func buildMeterProvider(ctx context.Context, cfg Config, res *resource.Resource, p *Providers) (otelmetric.MeterProvider, error) {
	if cfg.MetricsExporter == None {
		return metricnoop.NewMeterProvider(), nil
	}

	var (
		reader sdkmetric.Reader
		err    error
	)
	switch cfg.MetricsExporter {
	case OTLPGRPC:
		opts := []otlpmetricgrpc.Option{}
		if cfg.OTLPEndpoint != "" {
			opts = append(opts, otlpmetricgrpc.WithEndpoint(cfg.OTLPEndpoint))
		}
		if cfg.OTLPInsecure {
			opts = append(opts, otlpmetricgrpc.WithInsecure())
		}
		if len(cfg.OTLPHeaders) > 0 {
			opts = append(opts, otlpmetricgrpc.WithHeaders(cfg.OTLPHeaders))
		}
		var exp *otlpmetricgrpc.Exporter
		if exp, err = otlpmetricgrpc.New(ctx, opts...); err == nil {
			reader = sdkmetric.NewPeriodicReader(exp, sdkmetric.WithInterval(cfg.MetricInterval))
		}
	case OTLPHTTP:
		opts := []otlpmetrichttp.Option{}
		if cfg.OTLPEndpoint != "" {
			opts = append(opts, otlpmetrichttp.WithEndpoint(cfg.OTLPEndpoint))
		}
		if cfg.OTLPInsecure {
			opts = append(opts, otlpmetrichttp.WithInsecure())
		}
		if len(cfg.OTLPHeaders) > 0 {
			opts = append(opts, otlpmetrichttp.WithHeaders(cfg.OTLPHeaders))
		}
		var exp *otlpmetrichttp.Exporter
		if exp, err = otlpmetrichttp.New(ctx, opts...); err == nil {
			reader = sdkmetric.NewPeriodicReader(exp, sdkmetric.WithInterval(cfg.MetricInterval))
		}
	case Stdout:
		var exp sdkmetric.Exporter
		if exp, err = stdoutmetric.New(stdoutmetric.WithWriter(cfg.Writer)); err == nil {
			reader = sdkmetric.NewPeriodicReader(exp, sdkmetric.WithInterval(cfg.MetricInterval))
		}
	case Prometheus:
		reg := prometheus.NewRegistry()
		var exp *promexp.Exporter
		if exp, err = promexp.New(promexp.WithRegisterer(reg)); err == nil {
			reader = exp
			p.MetricsHandler = promhttp.HandlerFor(reg, promhttp.HandlerOpts{})
		}
	default:
		return nil, fmt.Errorf("otelx: unknown MetricsExporter %q", cfg.MetricsExporter)
	}
	if err != nil {
		return nil, fmt.Errorf("otelx: build metric reader: %w", err)
	}

	mp := sdkmetric.NewMeterProvider(
		sdkmetric.WithResource(res),
		sdkmetric.WithReader(reader),
	)
	p.shutdown = append(p.shutdown, mp.Shutdown)
	return mp, nil
}

// sampleRatio maps Config.SampleRatio to a TraceIDRatioBased fraction: 0 (or
// negative) and anything >= 1 mean "always sample".
func sampleRatio(r float64) float64 {
	if r <= 0 || r >= 1 {
		return 1
	}
	return r
}
