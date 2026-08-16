package main

import (
	"encoding/base64"
	"html/template"
	"io"
	"log"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/miekg/dns"
)

// StartHTTP runs the loopback backend for reverse proxies. Same endpoints as
// the SNI router, just without the TLS gymnastics.
func StartHTTP(s *State) {
	ln, err := net.Listen("tcp", cfg.Listen.HTTP)
	if err != nil {
		log.Fatalf("http: %v", err)
	}
	log.Printf("HTTP (reverse-proxy backend) on %s", cfg.Listen.HTTP)
	srv := &http.Server{
		Handler:           buildHandler(s),
		ReadHeaderTimeout: 10 * time.Second,
	}
	if err := srv.Serve(ln); err != nil {
		log.Fatalf("http serve: %v", err)
	}
}

func buildHandler(s *State) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/dns-query", s.handleDoH)
	mux.HandleFunc("/api/", s.handleAPI)
	mux.HandleFunc("/healthz", s.handleHealth)
	mux.HandleFunc("/", s.handleStatus)
	return logRequests(mux)
}

func logRequests(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		next.ServeHTTP(w, r)
		if r.URL.Path == "/dns-query" {
			return // too noisy
		}
		log.Printf("%s %s %s (%s)", r.RemoteAddr, r.Method, r.URL.Path, time.Since(start).Round(time.Millisecond))
	})
}

// ---------------------------------------------------------------------------
// DNS-over-HTTPS
// ---------------------------------------------------------------------------

// handleDoH implements RFC 8484: GET ?dns= or POST with an application/dns-message body.
func (s *State) handleDoH(w http.ResponseWriter, r *http.Request) {
	if !dnsLimiter.Allow() {
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
func (s *State) clientIP(r *http.Request) net.IP {
	ip := parseHostIP(r.RemoteAddr)
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

func (s *State) handleHealth(w http.ResponseWriter, _ *http.Request) {
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

func (s *State) handleStatus(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}

	data := statusData{
		Version:          version,
		Host:             cfg.Host,
		VPSIP:            cfg.VPSIP,
		Uptime:           time.Since(startTime).Round(time.Second).String(),
		AllowedCount:     s.AllowedCount(),
		RestrictedCount:  s.RestrictedCount(),
		DoHEndpoint:      "https://" + cfg.Host + "/dns-query",
		PlainDNSEndpoint: cfg.VPSIP + ":" + portOf(cfg.Listen.DNS),
		DoTEndpoint:      "tls://" + cfg.Host + ":" + portOf(cfg.Listen.DoT),
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

var statusTemplate = template.Must(template.New("status").Parse(`<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>Kairo</title>
<style>
  :root{--bg:#0d1117;--card:#161b22;--border:#30363d;--fg:#e6edf3;--muted:#8b949e;--accent:#58a6ff;--good:#3fb950}
  *{box-sizing:border-box}
  body{margin:0;background:var(--bg);color:var(--fg);font:15px/1.6 -apple-system,"Segoe UI",Roboto,Helvetica,Arial,sans-serif}
  .wrap{max-width:760px;margin:0 auto;padding:48px 20px}
  h1{font-size:22px;margin:0 0 4px}
  .sub{color:var(--muted);margin:0 0 28px}
  .grid{display:grid;grid-template-columns:repeat(auto-fit,minmax(200px,1fr));gap:14px;margin-bottom:28px}
  .card{background:var(--card);border:1px solid var(--border);border-radius:10px;padding:16px}
  .card .k{color:var(--muted);font-size:12px;text-transform:uppercase;letter-spacing:.05em}
  .card .v{font-size:16px;font-weight:600;word-break:break-all}
  .badge{display:inline-block;padding:2px 10px;border-radius:999px;background:rgba(63,185,80,.15);color:var(--good);font-size:12px;font-weight:600}
  h2{font-size:14px;margin:26px 0 10px;color:var(--muted);text-transform:uppercase;letter-spacing:.08em}
  code{background:#0d1117;border:1px solid var(--border);border-radius:6px;padding:2px 6px;font-size:13px}
  ul{margin:6px 0 0;padding-left:20px}
  li{margin:6px 0}
  .foot{color:var(--muted);font-size:12px;margin-top:32px}
</style>
</head>
<body>
<div class="wrap">
  <h1>Kairo</h1>
  <p class="sub">Transparent domain-based split routing: DNS policy, DoH/DoT and the SNI proxy in a single binary.</p>

  <div class="badge">operational</div>
  <div class="grid">
    <div class="card"><div class="k">Version</div><div class="v">{{.Version}}</div></div>
    <div class="card"><div class="k">Host</div><div class="v">{{.Host}}</div></div>
    <div class="card"><div class="k">VPS IP</div><div class="v">{{.VPSIP}}</div></div>
    <div class="card"><div class="k">Uptime</div><div class="v">{{.Uptime}}</div></div>
    <div class="card"><div class="k">Allowlisted clients</div><div class="v">{{.AllowedCount}}</div></div>
    <div class="card"><div class="k">Restricted domains</div><div class="v">{{.RestrictedCount}}</div></div>
  </div>

  <h2>DNS endpoints</h2>
  <ul>
    <li>Plain DNS: <code>{{.PlainDNSEndpoint}}</code></li>
    <li>DNS over HTTPS: <code>{{.DoHEndpoint}}</code></li>
    <li>DNS over TLS: <code>{{.DoTEndpoint}}</code></li>
  </ul>

  <h2>API</h2>
  <ul>
    <li>Allow a client: <code>POST /api/allow?key=...&amp;ip=1.2.3.4</code></li>
    <li>List allowlist: <code>GET /api/allow?key=...</code></li>
    <li>Remove a client: <code>DELETE /api/allow?key=...&amp;ip=1.2.3.4</code></li>
    <li>Manage domains: <code>/api/restricted?key=...&amp;domain=example.com</code></li>
    <li>Generate allowlist from ip_source: <code>POST /api/generate?key=...</code></li>
    <li>Status: <code>GET /api/status?key=...</code></li>
  </ul>

  <p class="foot">Health check: <code>/healthz</code> · Configure clients to use {{.VPSIP}} as their DNS server.</p>
</div>
</body>
</html>
`))
