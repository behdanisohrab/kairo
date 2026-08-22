package server

import (
	"encoding/base64"
	"embed"
	"html/template"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
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

	// Serve static files from web/dist/ if it exists
	if webFS, err := s.webFS(); err == nil {
		mux.Handle("/", webFS)
		slog.Info("serving web UI", "dir", s.cfg.WebDir)
	} else {
		slog.Warn("web UI not found, serving status page", "dir", s.cfg.WebDir, "error", err)
		mux.HandleFunc("/", s.handleStatus)
	}

	h := s.securityHeaders(mux)
	h = s.corsMiddleware(h)
	return s.logRequests(h)
}

func (s *Server) securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Frame-Options", "SAMEORIGIN")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")
		w.Header().Set("Permissions-Policy", "camera=(), microphone=(), geolocation=()")
		// CSP: allow self, inline styles, google fonts, and data for favicon
		// Keep permissive for API but strict for UI - API JSON doesn't use CSP
		if strings.HasPrefix(r.URL.Path, "/api/") || r.URL.Path == "/dns-query" {
			w.Header().Set("Content-Security-Policy", "default-src 'none'; frame-ancestors 'none'")
		} else {
			w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self' 'unsafe-inline'; style-src 'self' 'unsafe-inline' https://fonts.googleapis.com; font-src 'self' https://fonts.gstatic.com; img-src 'self' data:; connect-src 'self'; frame-ancestors 'none'")
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		if origin == "" {
			next.ServeHTTP(w, r)
			return
		}
		// Allow same-host and configured admin_url/doh_url origins
		allowed := false
		if s.cfg.AdminURL != "" && isOriginAllowed(origin, s.cfg.AdminURL) {
			allowed = true
		}
		if s.cfg.DoHURL != "" && isOriginAllowed(origin, s.cfg.DoHURL) {
			allowed = true
		}
		// Also allow host itself
		if s.cfg.Host != "" && strings.Contains(origin, s.cfg.Host) {
			allowed = true
		}
		if allowed {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Access-Control-Allow-Credentials", "true")
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, DELETE, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-API-Key")
			w.Header().Set("Vary", "Origin")
		}
		if r.Method == http.MethodOptions {
			if allowed {
				w.WriteHeader(http.StatusNoContent)
			} else {
				w.WriteHeader(http.StatusForbidden)
			}
			return
		}
		next.ServeHTTP(w, r)
	})
}

func isOriginAllowed(origin, targetURL string) bool {
	if origin == targetURL {
		return true
	}
	// Compare hosts
	oHost := origin
	if idx := strings.Index(oHost, "://"); idx >= 0 {
		oHost = oHost[idx+3:]
	}
	if idx := strings.IndexByte(oHost, '/'); idx >= 0 {
		oHost = oHost[:idx]
	}
	tHost := targetURL
	if idx := strings.Index(tHost, "://"); idx >= 0 {
		tHost = tHost[idx+3:]
	}
	if idx := strings.IndexByte(tHost, '/'); idx >= 0 {
		tHost = tHost[:idx]
	}
	return oHost == tHost || oHost == strings.TrimPrefix(tHost, "www.")
}

// webFS returns an http.FileSystem that serves the built frontend from web/dist/.
func (s *Server) webFS() (http.Handler, error) {
	info, err := os.Stat(s.cfg.WebDir)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() {
		return nil, err
	}

	fileServer := http.FileServer(http.Dir(s.cfg.WebDir))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Try to serve the file; if not found, serve index.html (SPA fallback)
		path := s.cfg.WebDir + r.URL.Path
		if _, err := os.Stat(path); os.IsNotExist(err) {
			r.URL.Path = "/"
		}
		fileServer.ServeHTTP(w, r)
	}), nil
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
