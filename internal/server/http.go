package server

import (
	"encoding/base64"
	"embed"
	"html/template"
	"io"
	"log/slog"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/miekg/dns"

	"kairo/internal/netutil"
)

//go:embed status.tmpl
var statusFiles embed.FS

func (s *Server) BuildHandler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/dns-query", s.handleDoH)
	mux.HandleFunc("/api/", s.handleAPI)
	mux.HandleFunc("/healthz", s.handleHealth)
	mux.HandleFunc("/", s.handleStatus)
	return s.logRequests(mux)
}

// statusRecorder captures the status code written for metrics and logging.
type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(code int) {
	r.status = code
	r.ResponseWriter.WriteHeader(code)
}

func (s *Server) logRequests(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rec, r)
		elapsed := time.Since(start).Round(time.Millisecond)

		path := r.URL.Path
		if path == "/dns-query" {
			return // too noisy
		}
		if s.Metrics != nil {
			s.Metrics.HTTPRequests.WithLabelValues(path, strconv.Itoa(rec.status)).Inc()
		}
		slog.Info("http request", "method", r.Method, "path", path, "status", rec.status, "duration", elapsed.String(), "remote", r.RemoteAddr)
	})
}

// ---------------------------------------------------------------------------
// DNS-over-HTTPS
// ---------------------------------------------------------------------------

// handleDoH implements RFC 8484: GET ?dns= or POST with an application/dns-message body.
func (s *Server) handleDoH(w http.ResponseWriter, r *http.Request) {
	if !s.dnsLimiter.Allow() {
		if s.Metrics != nil {
			s.Metrics.DNSRateLimited.Inc()
		}
		http.Error(w, "rate limit exceeded", http.StatusTooManyRequests)
		return
	}

	var body []byte
	var err error
	switch r.Method {
	case http.MethodGet:
		q := r.URL.Query().Get("dns")
		if q == "" {
			http.Error(w, "missing 'dns' query parameter", http.StatusBadRequest)
			return
		}
		body, err = base64.RawURLEncoding.DecodeString(q)
	case http.MethodPost:
		body, err = io.ReadAll(io.LimitReader(r.Body, 65536))
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	var req dns.Msg
	if err := req.Unpack(body); err != nil {
		http.Error(w, "invalid DNS message", http.StatusBadRequest)
		return
	}

	resp := s.processQuery(&req, s.clientIP(r))
	out, err := resp.Pack()
	if err != nil {
		http.Error(w, "failed to pack response", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/dns-message")
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write(out)
}

// clientIP gets the real client, trusting X-Forwarded-For only from a local
// reverse proxy. Anywhere else a spoofed header just gets ignored.
func (s *Server) clientIP(r *http.Request) net.IP {
	ip := netutil.ParseHostIP(r.RemoteAddr)
	if ip == nil {
		return nil
	}
	if ip.IsLoopback() {
		if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
			first := strings.TrimSpace(strings.SplitN(xff, ",", 2)[0])
			if fwd := net.ParseIP(first); fwd != nil {
				return fwd
			}
		}
	}
	return ip
}

// ---------------------------------------------------------------------------
// Health + status
// ---------------------------------------------------------------------------

func (s *Server) handleHealth(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	_, _ = w.Write([]byte("ok\n"))
}

type statusData struct {
	Version          string
	Host             string
	VPSIP            string
	Uptime           string
	AllowedCount     int
	RestrictedCount  int
	DoHEndpoint      string
	PlainDNSEndpoint string
	DoTEndpoint      string
}

func (s *Server) handleStatus(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}

	data := statusData{
		Version:          s.Version,
		Host:             s.cfg.Host,
		VPSIP:            s.cfg.VPSIP,
		Uptime:           time.Since(s.start).Round(time.Second).String(),
		AllowedCount:     s.st.AllowedCount(),
		RestrictedCount:  s.st.RestrictedCount(),
		DoHEndpoint:      "https://" + s.cfg.Host + "/dns-query",
		PlainDNSEndpoint: s.cfg.VPSIP + ":" + portOf(s.cfg.Listen.DNS),
		DoTEndpoint:      "tls://" + s.cfg.Host + ":" + portOf(s.cfg.Listen.DoT),
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = statusTemplate.Execute(w, data)
}

func portOf(addr string) string {
	if i := strings.LastIndexByte(addr, ':'); i >= 0 {
		return addr[i+1:]
	}
	return addr
}

var statusTemplate = template.Must(template.New("status").ParseFS(statusFiles, "status.tmpl"))
