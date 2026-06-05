package handlers

import (
	"github.com/gofiber/fiber/v3"
	"github.com/gofiber/fiber/v3/middleware/adaptor"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	fedmetrics "github.com/lebe-dev/turboist/internal/federation/metrics"
)

// MetricsHandler exposes the Prometheus exposition at GET /metrics (Federation v1
// F6.5, US-8.2). It is mounted publicly (like /healthz) — federation metrics carry
// no secrets, only operational counters/gauges — and renders the registry the
// federation collectors are registered into via prometheus/client_golang's own
// promhttp handler, so the exposition format is exactly what scrapers expect.
//
// The handler is constructed only when federation is enabled (the collectors
// exist); when federation is off there is no /metrics route (nothing to expose).
type MetricsHandler struct {
	registry *prometheus.Registry
}

// NewMetricsHandler constructs the metrics handler over the federation collectors'
// registry.
func NewMetricsHandler(collectors *fedmetrics.Collectors) *MetricsHandler {
	return &MetricsHandler{registry: collectors.Registry()}
}

// RegisterPublic mounts GET /metrics onto app (before RegisterSPA so the SPA
// fallback does not swallow it). The promhttp handler is bridged into Fiber via
// the standard net/http adaptor.
func (h *MetricsHandler) RegisterPublic(app *fiber.App) {
	promHandler := promhttp.HandlerFor(h.registry, promhttp.HandlerOpts{})
	app.Get("/metrics", adaptor.HTTPHandler(promHandler))
}
