// Package metrics defines and exposes Prometheus metrics for Kairo.
package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
)

// Metrics bundles the Prometheus registry and the metric handles the rest of
// the codebase increments. The registry also carries the Go runtime and process
// collectors so a scrape gives a full picture.
type Metrics struct {
	Reg *prometheus.Registry

	// DNS
	DNSQueries     *prometheus.CounterVec
	DNSRateLimited prometheus.Counter

	// SNI router
	SNIConnections *prometheus.CounterVec

	// HTTP / API
	HTTPRequests  *prometheus.CounterVec
	APIRateLimited prometheus.Counter

	// State gauges (wired to live counters via gauge functions)
	AllowlistedClients prometheus.GaugeFunc
	RestrictedDomains  prometheus.GaugeFunc
}

// New builds the metric registry and registers all collectors. allowlisted and
// restricted are functions reading the current policy counts, so the gauges
// always reflect live state.
func New(allowlisted, restricted func() int) *Metrics {
	reg := prometheus.NewRegistry()
	reg.MustRegister(collectors.NewGoCollector())
	reg.MustRegister(collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}))

	m := &Metrics{
		Reg: reg,

		DNSQueries: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "kairo_dns_queries_total",
			Help: "DNS queries answered, by record type and routing outcome.",
		}, []string{"type", "outcome"}),
		DNSRateLimited: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "kairo_dns_rate_limited_total",
			Help: "DNS queries refused because of the rate limiter.",
		}),

		SNIConnections: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "kairo_sni_connections_total",
			Help: "TLS connections handled by the SNI router, by outcome.",
		}, []string{"outcome"}),

		HTTPRequests: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "kairo_http_requests_total",
			Help: "HTTP requests served by the built-in handler, by path and status code.",
		}, []string{"path", "code"}),
		APIRateLimited: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "kairo_api_rate_limited_total",
			Help: "API requests refused because of the rate limiter.",
		}),
	}

	reg.MustRegister(m.DNSQueries, m.DNSRateLimited, m.SNIConnections, m.HTTPRequests, m.APIRateLimited)

	m.AllowlistedClients = prometheus.NewGaugeFunc(prometheus.GaugeOpts{
		Name: "kairo_allowlisted_clients",
		Help: "Current number of allowlisted client IPs.",
	}, func() float64 { return float64(allowlisted()) })
	m.RestrictedDomains = prometheus.NewGaugeFunc(prometheus.GaugeOpts{
		Name: "kairo_restricted_domains",
		Help: "Current number of restricted domains.",
	}, func() float64 { return float64(restricted()) })
	reg.MustRegister(m.AllowlistedClients, m.RestrictedDomains)

	return m
}
