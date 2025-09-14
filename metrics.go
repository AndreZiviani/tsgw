package main

import (
	"context"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

// Metrics holds all the custom metrics for the application
type Metrics struct {
	// Route metrics
	ActiveRoutes metric.Int64ObservableGauge

	// HTTP metrics
	RequestCount    metric.Int64Counter
	RequestDuration metric.Float64Histogram

	// Tailscale metrics
	TailscaleConnections metric.Int64ObservableGauge
	ServerStartupTime    metric.Float64Histogram
}

// InitMetrics initializes all custom metrics
func InitMetrics(meter metric.Meter) (*Metrics, error) {
	activeRoutes, err := meter.Int64ObservableGauge(
		"tsgw.routes.active",
		metric.WithDescription("Number of active routes"),
	)
	if err != nil {
		return nil, err
	}

	requestCount, err := meter.Int64Counter(
		"tsgw.http.requests.total",
		metric.WithDescription("Total number of HTTP requests"),
	)
	if err != nil {
		return nil, err
	}

	requestDuration, err := meter.Float64Histogram(
		"tsgw.http.request.duration",
		metric.WithDescription("HTTP request duration in seconds"),
		metric.WithUnit("s"),
	)
	if err != nil {
		return nil, err
	}

	tailscaleConnections, err := meter.Int64ObservableGauge(
		"tsgw.tailscale.connections.active",
		metric.WithDescription("Number of active Tailscale connections"),
	)
	if err != nil {
		return nil, err
	}

	serverStartupTime, err := meter.Float64Histogram(
		"tsgw.server.startup.duration",
		metric.WithDescription("Time taken to start a Tailscale server"),
		metric.WithUnit("s"),
	)
	if err != nil {
		return nil, err
	}

	metrics := &Metrics{
		ActiveRoutes:         activeRoutes,
		RequestCount:         requestCount,
		RequestDuration:      requestDuration,
		TailscaleConnections: tailscaleConnections,
		ServerStartupTime:    serverStartupTime,
	}

	return metrics, nil
}

// RecordHTTPRequest records an HTTP request metric
func (m *Metrics) RecordHTTPRequest(ctx context.Context, method, host string, statusCode int, duration time.Duration) {
	attrs := []attribute.KeyValue{
		attribute.String("method", method),
		attribute.String("host", host),
		attribute.Int("status_code", statusCode),
	}

	m.RequestCount.Add(ctx, 1, metric.WithAttributes(attrs...))
	m.RequestDuration.Record(ctx, duration.Seconds(), metric.WithAttributes(attrs...))
}

// RecordServerStartup records server startup time
func (m *Metrics) RecordServerStartup(ctx context.Context, routeName string, duration time.Duration) {
	attrs := []attribute.KeyValue{
		attribute.String("route", routeName),
	}

	m.ServerStartupTime.Record(ctx, duration.Seconds(), metric.WithAttributes(attrs...))
}

// UpdateActiveRoutes updates the active routes gauge
func (m *Metrics) UpdateActiveRoutes(ctx context.Context, count int) {
	// This would be called periodically to update the gauge
	// In a real implementation, you'd register a callback with the meter
	_ = ctx
	_ = count
}

// UpdateTailscaleConnections updates the Tailscale connections gauge
func (m *Metrics) UpdateTailscaleConnections(ctx context.Context, count int) {
	// This would be called periodically to update the gauge
	// In a real implementation, you'd register a callback with the meter
	_ = ctx
	_ = count
}
