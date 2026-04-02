package app

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.37.0"
	"go.opentelemetry.io/otel/trace"
)

type Telemetry struct {
	tracerProvider *sdktrace.TracerProvider
}

func NewTelemetry(cfg Config) (*Telemetry, error) {
	otel.SetTextMapPropagator(
		propagation.NewCompositeTextMapPropagator(
			propagation.TraceContext{},
			propagation.Baggage{},
		),
	)

	if !cfg.OTELEnabled {
		return &Telemetry{}, nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	exporter, err := newOTLPTraceExporter(ctx, cfg.OTELExporterOTLPEndpoint)
	if err != nil {
		return nil, err
	}

	res, err := resource.Merge(
		resource.Default(),
		resource.NewSchemaless(
			semconv.ServiceName(cfg.OTELServiceName),
			semconv.DeploymentEnvironmentName(cfg.AppEnv),
			attribute.String("app.name", cfg.AppName),
		),
	)
	if err != nil {
		return nil, err
	}

	provider := sdktrace.NewTracerProvider(
		sdktrace.WithSampler(sdktrace.ParentBased(sdktrace.AlwaysSample())),
		sdktrace.WithResource(res),
		sdktrace.WithBatcher(
			exporter,
			sdktrace.WithBatchTimeout(2*time.Second),
			sdktrace.WithExportTimeout(5*time.Second),
		),
	)
	otel.SetTracerProvider(provider)
	return &Telemetry{tracerProvider: provider}, nil
}

func (t *Telemetry) Shutdown(ctx context.Context) error {
	if t == nil || t.tracerProvider == nil {
		return nil
	}
	return t.tracerProvider.Shutdown(ctx)
}

func WrapHandlerWithTelemetry(handler http.Handler) http.Handler {
	return otelhttp.NewHandler(
		handler,
		"ops-agent-http",
		otelhttp.WithSpanNameFormatter(func(_ string, r *http.Request) string {
			return r.Method + " " + r.URL.Path
		}),
	)
}

func NewTracingHTTPClient(base *http.Client) *http.Client {
	if base == nil {
		base = &http.Client{}
	}
	clone := *base
	transport := clone.Transport
	if transport == nil {
		transport = http.DefaultTransport
	}
	clone.Transport = otelhttp.NewTransport(transport)
	return &clone
}

func StartSpan(ctx context.Context, name string, opts ...trace.SpanStartOption) (context.Context, trace.Span) {
	return otel.Tracer("ops-agent-copilot").Start(ctx, name, opts...)
}

func SpanTraceIDFromContext(ctx context.Context) string {
	spanContext := trace.SpanContextFromContext(ctx)
	if !spanContext.IsValid() {
		return ""
	}
	return spanContext.TraceID().String()
}

func BusinessTraceIDFromContext(ctx context.Context) string {
	if traceID := TraceIDFromContext(ctx); traceID != "" {
		return traceID
	}
	return SpanTraceIDFromContext(ctx)
}

func AnnotateCurrentSpan(ctx context.Context, attrs ...attribute.KeyValue) {
	if len(attrs) == 0 {
		return
	}
	trace.SpanFromContext(ctx).SetAttributes(attrs...)
}

func RecordSpanError(span trace.Span, err error) {
	if err == nil {
		return
	}
	span.RecordError(err)
	span.SetStatus(codes.Error, err.Error())
}

func newOTLPTraceExporter(ctx context.Context, rawEndpoint string) (sdktrace.SpanExporter, error) {
	endpoint := strings.TrimSpace(rawEndpoint)
	if endpoint == "" {
		endpoint = "http://127.0.0.1:4318"
	}

	options := make([]otlptracehttp.Option, 0, 4)
	if strings.Contains(endpoint, "://") {
		parsed, err := url.Parse(endpoint)
		if err != nil {
			return nil, fmt.Errorf("parse OTEL exporter endpoint failed: %w", err)
		}
		if parsed.Host == "" {
			return nil, fmt.Errorf("invalid OTEL exporter endpoint: %s", endpoint)
		}
		options = append(options, otlptracehttp.WithEndpoint(parsed.Host))
		if strings.EqualFold(parsed.Scheme, "http") {
			options = append(options, otlptracehttp.WithInsecure())
		}
		if path := strings.TrimSpace(parsed.Path); path != "" && path != "/" {
			options = append(options, otlptracehttp.WithURLPath(path))
		}
	} else {
		options = append(options, otlptracehttp.WithEndpoint(endpoint))
	}

	return otlptracehttp.New(ctx, options...)
}
