# otelx

`import "github.com/rithulkamesh/toolkit/otelx"`

OpenTelemetry traces + metrics in one call, with a switch for the exporter
backend.

```go
tel, err := otelx.Setup(ctx, otelx.Config{
	ServiceName:     "billing",
	ServiceVersion:  build.Version,
	Environment:     "production",
	TracesExporter:  otelx.OTLPGRPC,
	MetricsExporter: otelx.Prometheus,
	SampleRatio:     0.1,
})
if err != nil {
	return err
}
defer tel.Shutdown(context.Background())

if tel.MetricsHandler != nil {
	mux.Handle("/metrics", tel.MetricsHandler) // Prometheus scrape
}
```

After `Setup` the OpenTelemetry globals (`TracerProvider`, `MeterProvider`, a
`tracecontext`+`baggage` propagator) are installed, so any library that calls
`otel.Tracer` / `otel.Meter` is wired up too.

| Exporter | Traces | Metrics |
|---|---|---|
| `OTLPGRPC` | ✓ (port 4317) | ✓ |
| `OTLPHTTP` | ✓ (port 4318) | ✓ |
| `Prometheus` | — | ✓ via `Providers.MetricsHandler` |
| `Stdout` | ✓ | ✓ (dev) |
| `None` | no-op | no-op |

`TracesExporter` / `MetricsExporter` default to `OTLPGRPC` when an OTLP
endpoint is configured (here or via `$OTEL_EXPORTER_OTLP_ENDPOINT`), else
`None`.
