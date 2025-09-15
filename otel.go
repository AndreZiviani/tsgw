package main

import (
	"context"
	"fmt"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetricgrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/metric/noop"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.21.0"
	oteltrace "go.opentelemetry.io/otel/trace"
	tracenoop "go.opentelemetry.io/otel/trace/noop"
	"google.golang.org/grpc/credentials"

	"github.com/rs/zerolog/log"
)

// OpenTelemetry holds the OpenTelemetry components
type OpenTelemetry struct {
	TracerProvider oteltrace.TracerProvider
	MeterProvider  metric.MeterProvider
	Tracer         oteltrace.Tracer
	Meter          metric.Meter
}

// SetupOpenTelemetry initializes OpenTelemetry if enabled in configuration
func SetupOpenTelemetry(ctx context.Context, config *Config) (*OpenTelemetry, error) {
	if !config.OpenTelemetry.Enabled {
		log.Info().Msg("OpenTelemetry is disabled")
		return &OpenTelemetry{
			TracerProvider: tracenoop.NewTracerProvider(),
			MeterProvider:  noop.NewMeterProvider(),
			Tracer:         tracenoop.NewTracerProvider().Tracer("tsgw"),
			Meter:          noop.NewMeterProvider().Meter("tsgw"),
		}, nil
	}

	if config.OpenTelemetry.Endpoint == "" {
		return nil, fmt.Errorf("OpenTelemetry endpoint is required when OpenTelemetry is enabled")
	}

	log.Info().
		Str("service_name", config.OpenTelemetry.ServiceName).
		Str("endpoint", config.OpenTelemetry.Endpoint).
		Str("protocol", config.OpenTelemetry.Protocol).
		Bool("insecure", config.OpenTelemetry.Insecure).
		Msg("Setting up OpenTelemetry")

	// Create resource
	res, err := resource.New(ctx,
		resource.WithAttributes(
			semconv.ServiceNameKey.String(config.OpenTelemetry.ServiceName),
			semconv.ServiceVersionKey.String("1.0.0"),
		),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create resource: %w", err)
	}

	// Setup trace provider
	traceExporter, err := createTraceExporter(ctx, config)
	if err != nil {
		return nil, fmt.Errorf("failed to create trace exporter: %w", err)
	}

	tracerProvider := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(traceExporter),
		sdktrace.WithResource(res),
	)

	// Setup meter provider
	metricExporter, err := createMetricExporter(ctx, config)
	if err != nil {
		return nil, fmt.Errorf("failed to create metric exporter: %w", err)
	}

	meterProvider := sdkmetric.NewMeterProvider(
		sdkmetric.WithReader(sdkmetric.NewPeriodicReader(metricExporter)),
		sdkmetric.WithResource(res),
	)

	// Set global providers
	otel.SetTracerProvider(tracerProvider)
	otel.SetMeterProvider(meterProvider)

	// Create tracer and meter
	tracer := tracerProvider.Tracer("tsgw")
	meter := meterProvider.Meter("tsgw")

	log.Info().Msg("OpenTelemetry setup completed")

	return &OpenTelemetry{
		TracerProvider: tracerProvider,
		MeterProvider:  meterProvider,
		Tracer:         tracer,
		Meter:          meter,
	}, nil
}

// createTraceExporter creates the appropriate trace exporter based on configuration
func createTraceExporter(ctx context.Context, config *Config) (sdktrace.SpanExporter, error) {
	var opts []otlptracegrpc.Option

	if config.OpenTelemetry.Insecure {
		opts = append(opts, otlptracegrpc.WithInsecure())
	} else {
		opts = append(opts, otlptracegrpc.WithTLSCredentials(credentials.NewClientTLSFromCert(nil, "")))
	}

	if config.OpenTelemetry.Endpoint != "" {
		opts = append(opts, otlptracegrpc.WithEndpoint(config.OpenTelemetry.Endpoint))
	}

	// Add headers if configured
	if len(config.OpenTelemetry.Headers) > 0 {
		opts = append(opts, otlptracegrpc.WithHeaders(config.OpenTelemetry.Headers))
	}

	return otlptracegrpc.New(ctx, opts...)
}

// createMetricExporter creates the appropriate metric exporter based on configuration
func createMetricExporter(ctx context.Context, config *Config) (sdkmetric.Exporter, error) {
	var opts []otlpmetricgrpc.Option

	if config.OpenTelemetry.Insecure {
		opts = append(opts, otlpmetricgrpc.WithInsecure())
	} else {
		opts = append(opts, otlpmetricgrpc.WithTLSCredentials(credentials.NewClientTLSFromCert(nil, "")))
	}

	if config.OpenTelemetry.Endpoint != "" {
		opts = append(opts, otlpmetricgrpc.WithEndpoint(config.OpenTelemetry.Endpoint))
	}

	// Add headers if configured
	if len(config.OpenTelemetry.Headers) > 0 {
		opts = append(opts, otlpmetricgrpc.WithHeaders(config.OpenTelemetry.Headers))
	}

	return otlpmetricgrpc.New(ctx, opts...)
}

// Shutdown gracefully shuts down OpenTelemetry components
func (ot *OpenTelemetry) Shutdown(ctx context.Context) error {
	log.Info().Msg("Shutting down OpenTelemetry")

	var errs []error

	// Shutdown tracer provider
	if tp, ok := ot.TracerProvider.(*sdktrace.TracerProvider); ok {
		if err := tp.Shutdown(ctx); err != nil {
			errs = append(errs, fmt.Errorf("failed to shutdown tracer provider: %w", err))
		}
	}

	// Shutdown meter provider
	if mp, ok := ot.MeterProvider.(*sdkmetric.MeterProvider); ok {
		if err := mp.Shutdown(ctx); err != nil {
			errs = append(errs, fmt.Errorf("failed to shutdown meter provider: %w", err))
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("OpenTelemetry shutdown errors: %v", errs)
	}

	log.Info().Msg("OpenTelemetry shutdown completed")
	return nil
}
