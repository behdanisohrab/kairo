// Package server implements Kairo's DNS servers (plain, DoT, DoH) and the HTTP
// API surface, wired together by a single Server value that carries the
// configuration, policy state and rate limiters.
package server

import (
	"crypto/tls"
	"log/slog"
	"net"
	"net/http"
	"time"

	"github.com/prometheus/client_golang/prometheus/promhttp"

	"golang.org/x/time/rate"

	"kairo/internal/config"
	"kairo/internal/metrics"
	"kairo/internal/state"
)

// GetCertificate supplies the TLS certificate for DoT (and, via the SNI router,
// for DoH/API). It matches tls.Config.GetCertificate.
type GetCertificate func(*tls.ClientHelloInfo) (*tls.Certificate, error)

// Server carries everything a request needs to be answered: the config, the
// policy state, the version string, the rate limiters and the metrics.
type Server struct {
	cfg        *config.Config
	st         *state.State
	Version    string
	Metrics    *metrics.Metrics
	getCert    GetCertificate
	start      time.Time
	dnsLimiter *rate.Limiter
	apiLimiter *rate.Limiter
}

// New wires up a Server from a loaded config, policy state, metric registry and
// a TLS certificate source. getCert may be nil, in which case DoT is disabled.
func New(cfg *config.Config, st *state.State, version string, m *metrics.Metrics, getCert GetCertificate) *Server {
	return &Server{
		cfg:        cfg,
		st:         st,
		Version:    version,
		Metrics:    m,
		getCert:    getCert,
		start:      time.Now(),
		dnsLimiter: rate.NewLimiter(rate.Limit(cfg.Rate.DNS), cfg.Rate.DNSBurst),
		apiLimiter: rate.NewLimiter(rate.Limit(cfg.Rate.API), cfg.Rate.APIBurst),
	}
}

// StartDNS runs the plain DNS servers (UDP and TCP on the same port).
func (s *Server) StartDNS(listen string) {
	udp := &dnsServer{Server: s, Net: "udp"}
	tcp := &dnsServer{Server: s, Net: "tcp"}
	slog.Info("starting plain DNS", "protocol", "udp", "addr", listen)
	slog.Info("starting plain DNS", "protocol", "tcp", "addr", listen)
	go func() {
		if err := udp.ListenAndServe(listen); err != nil {
			slog.Error("plain DNS udp", "error", err)
		}
	}()
	go func() {
		if err := tcp.ListenAndServe(listen); err != nil {
			slog.Error("plain DNS tcp", "error", err)
		}
	}()
}

// StartDoT runs the DNS-over-TLS listener on cfg.Listen.DoT, if a certificate
// source is configured.
func (s *Server) StartDoT() {
	if s.getCert == nil {
		slog.Warn("DoT disabled: no certificate source configured", "addr", s.cfg.Listen.DoT)
		return
	}
	ln, err := tls.Listen("tcp", s.cfg.Listen.DoT, &tls.Config{
		GetCertificate: s.getCert,
		MinVersion:     tls.VersionTLS12,
	})
	if err != nil {
		slog.Error("DoT listen", "addr", s.cfg.Listen.DoT, "error", err)
		return
	}
	slog.Info("starting DoT", "addr", s.cfg.Listen.DoT)
	for {
		conn, err := ln.Accept()
		if err != nil {
			continue
		}
		go s.handleDoTConn(conn)
	}
}

// StartHTTP runs the loopback backend for reverse proxies. Same endpoints as
// the SNI router, just without the TLS gymnastics.
func (s *Server) StartHTTP() {
	ln, err := net.Listen("tcp", s.cfg.Listen.HTTP)
	if err != nil {
		slog.Error("HTTP listen", "addr", s.cfg.Listen.HTTP, "error", err)
		return
	}
	slog.Info("starting HTTP reverse-proxy backend", "addr", s.cfg.Listen.HTTP)
	srv := &http.Server{
		Handler:           s.BuildHandler(),
		ReadHeaderTimeout: 10 * time.Second,
	}
	if err := srv.Serve(ln); err != nil {
		slog.Error("HTTP serve", "error", err)
	}
}

// StartMetrics serves the Prometheus scrape endpoint on its own listener, so it
// is never exposed on the public DoH/API/SNI surface.
func (s *Server) StartMetrics() {
	ln, err := net.Listen("tcp", s.cfg.Listen.Metrics)
	if err != nil {
		slog.Error("metrics listen", "addr", s.cfg.Listen.Metrics, "error", err)
		return
	}
	slog.Info("starting metrics endpoint", "addr", s.cfg.Listen.Metrics)
	mux := http.NewServeMux()
	mux.Handle("/metrics", promhttp.HandlerFor(s.Metrics.Reg, promhttp.HandlerOpts{}))
	srv := &http.Server{
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
	}
	if err := srv.Serve(ln); err != nil {
		slog.Error("metrics serve", "error", err)
	}
}
