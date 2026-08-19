// Package sni implements the SNI router that makes split routing real: one
// listener on :443 where every connection is decided by the name in its TLS
// handshake.
package sni

import (
	"bufio"
	"bytes"
	"crypto/tls"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"kairo/internal/config"
	"kairo/internal/metrics"
	"kairo/internal/netutil"
	"kairo/internal/state"
)

// Start runs the SNI router on cfg.Listen.HTTPS, serving the given HTTP handler
// for our own hostname and tunnelling everything else to allowlisted clients.
// getCert supplies the certificate for the host SNI; it may be nil, in which
// case TLS termination is skipped and host SNI must go to host_backend instead.
func Start(cfg *config.Config, st *state.State, m *metrics.Metrics, handler http.Handler, getCert func(*tls.ClientHelloInfo) (*tls.Certificate, error)) {
	ln, err := net.Listen("tcp", cfg.Listen.HTTPS)
	if err != nil {
		slog.Error("SNI router listen", "addr", cfg.Listen.HTTPS, "error", err)
		return
	}
	slog.Info("starting SNI router", "addr", cfg.Listen.HTTPS)

	var tlsConfig *tls.Config
	if getCert != nil {
		tlsConfig = &tls.Config{
			GetCertificate: getCert,
			MinVersion:     tls.VersionTLS12,
		}
	}
	if tlsConfig == nil && cfg.HostBackend == "" {
		slog.Warn("neither a certificate source nor host_backend is set; DoH/API will not be served", "addr", cfg.Listen.HTTPS)
	}

	for {
		conn, err := ln.Accept()
		if err != nil {
			continue
		}
		go handleConn(cfg, st, m, conn, handler, tlsConfig)
	}
}

func handleConn(cfg *config.Config, st *state.State, m *metrics.Metrics, clientConn net.Conn, handler http.Handler, tlsConfig *tls.Config) {
	defer clientConn.Close()

	// Everything is read through a bufio.Reader so a PROXY protocol header can
	// be consumed without losing any ClientHello bytes that nginx may already
	// have flushed into the buffer.
	reader := bufio.NewReader(clientConn)

	peerIP := netutil.RemoteIPAddr(clientConn.RemoteAddr())
	if cfg.ProxyProtocol && peerIP != nil && peerIP.IsLoopback() {
		src, err := readProxyHeader(reader)
		if err != nil {
			slog.Warn("proxy protocol error", "peer", peerIP, "error", err)
			recordSNI(m, "proxy_error")
			return
		}
		if src != nil {
			peerIP = src
		}
	}

	_ = clientConn.SetReadDeadline(time.Now().Add(5 * time.Second))
	hello, peeked, err := peekClientHello(reader)
	if err != nil {
		return
	}
	_ = clientConn.SetReadDeadline(time.Time{})

	sniName := strings.ToLower(strings.TrimSpace(hello.ServerName))
	if sniName == "" {
		recordSNI(m, "malformed")
		writeMalformed(clientConn)
		return
	}

	// Our own hostname: terminate TLS (or hand it to a reverse proxy) and
	// serve DoH and the API.
	if cfg.Host != "" && sniName == cfg.Host {
		recordSNI(m, "host")
		if tlsConfig != nil {
			serveTLS(cfg, clientConn, reader, peeked, handler, tlsConfig)
		} else if cfg.HostBackend != "" {
			tunnel(clientConn, reader, peeked, cfg.HostBackend)
		} else {
			slog.Warn("no handler for host SNI", "sni", sniName)
		}
		return
	}

	// Everything else is a split-routed destination, and the allowlist gate
	// applies again here. Trusting only the DNS answer would be naive.
	if !st.IsAllowedIP(peerIP) {
		recordSNI(m, "rejected")
		slog.Info("SNI connection rejected: not allowlisted", "peer", peerIP, "sni", sniName)
		return
	}

	recordSNI(m, "tunneled")
	tunnel(clientConn, reader, peeked, st.TunnelAddr(sniName))
}

// recordSNI counts one SNI router decision by outcome.
func recordSNI(m *metrics.Metrics, outcome string) {
	if m != nil {
		m.SNIConnections.WithLabelValues(outcome).Inc()
	}
}

// readProxyHeader parses a PROXY protocol v1 header, if the connection starts
// with one, and returns the source address it advertises. A nil source with no
// error means the connection was not a PROXY connection at all.
func readProxyHeader(r *bufio.Reader) (net.IP, error) {
	head, err := r.Peek(6)
	if err != nil {
		return nil, err
	}
	if !bytes.HasPrefix(head, []byte("PROXY ")) {
		return nil, nil
	}
	line, err := r.ReadString('\n')
	if err != nil {
		return nil, fmt.Errorf("read PROXY header: %w", err)
	}
	fields := strings.Fields(strings.TrimSpace(line))
	if len(fields) < 2 || fields[0] != "PROXY" {
		return nil, fmt.Errorf("malformed PROXY header")
	}
	switch fields[1] {
	case "UNKNOWN":
		return nil, nil
	case "TCP4", "TCP6":
	default:
		return nil, fmt.Errorf("unsupported PROXY protocol %q", fields[1])
	}
	if len(fields) != 6 {
		return nil, fmt.Errorf("malformed PROXY header")
	}
	ip := net.ParseIP(fields[2])
	if ip == nil {
		return nil, fmt.Errorf("invalid source IP in PROXY header")
	}
	return ip, nil
}

// tunnel relays bytes to the backend, replaying the ClientHello bytes we
// already swallowed so the destination still sees the full handshake. src is
// where the rest of the client's bytes come from; it may buffer more than
// clientConn exposes, so the raw conn is never read directly.
func tunnel(clientConn net.Conn, src io.Reader, peeked io.Reader, backendAddr string) {
	backend, err := net.DialTimeout("tcp", backendAddr, 5*time.Second)
	if err != nil {
		slog.Error("tunnel dial failed", "backend", backendAddr, "error", err)
		return
	}
	defer backend.Close()

	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		_, _ = io.Copy(clientConn, backend)
		if tc, ok := clientConn.(*net.TCPConn); ok {
			_ = tc.CloseWrite()
		}
	}()
	go func() {
		defer wg.Done()
		if peeked != nil {
			_, _ = io.Copy(backend, peeked)
		}
		_, _ = io.Copy(backend, src)
		if tc, ok := backend.(*net.TCPConn); ok {
			_ = tc.CloseWrite()
		}
	}()
	wg.Wait()
}

// serveTLS wraps the already-peeked connection so the TLS layer reads the
// ClientHello exactly once, then serves one HTTP session over it.
func serveTLS(cfg *config.Config, clientConn net.Conn, src io.Reader, peeked io.Reader, handler http.Handler, tlsConfig *tls.Config) {
	wrap := &bufferedConn{
		Conn: clientConn,
		r:    io.MultiReader(peeked, src),
	}
	tconn := tls.Server(wrap, tlsConfig)
	if err := tconn.Handshake(); err != nil {
		slog.Warn("TLS handshake failed", "host", cfg.Host, "error", err)
		return
	}
	srv := &http.Server{
		Handler:           handler,
		ReadHeaderTimeout: 10 * time.Second,
	}
	if err := srv.Serve(newSingleConnListener(tconn)); err != nil && err != net.ErrClosed {
		slog.Error("HTTP over TLS failed", "host", cfg.Host, "error", err)
	}
}

// ---------------------------------------------------------------------------
// ClientHello peeking
// ---------------------------------------------------------------------------

func peekClientHello(reader io.Reader) (*tls.ClientHelloInfo, *bytes.Buffer, error) {
	peeked := new(bytes.Buffer)
	hello, err := readClientHello(io.TeeReader(reader, peeked))
	if err != nil {
		return nil, nil, err
	}
	return hello, peeked, nil
}

func readClientHello(reader io.Reader) (*tls.ClientHelloInfo, error) {
	ch := make(chan *tls.ClientHelloInfo, 1)
	tlsConfig := &tls.Config{
		GetConfigForClient: func(argHello *tls.ClientHelloInfo) (*tls.Config, error) {
			select {
			case ch <- argHello:
			default:
			}
			return nil, nil
		},
	}
	_ = tls.Server(readOnlyConn{reader: reader}, tlsConfig).Handshake()
	select {
	case hello := <-ch:
		return hello, nil
	case <-time.After(5 * time.Second):
		return nil, fmt.Errorf("timeout waiting for ClientHello")
	}
}

// readOnlyConn is the read-only face we give the TLS stack while peeking.
type readOnlyConn struct {
	reader io.Reader
}

func (c readOnlyConn) Read(p []byte) (int, error)         { return c.reader.Read(p) }
func (c readOnlyConn) Write(_ []byte) (int, error)        { return 0, io.ErrClosedPipe }
func (c readOnlyConn) Close() error                       { return nil }
func (c readOnlyConn) LocalAddr() net.Addr                { return nil }
func (c readOnlyConn) RemoteAddr() net.Addr               { return nil }
func (c readOnlyConn) SetDeadline(_ time.Time) error      { return nil }
func (c readOnlyConn) SetReadDeadline(_ time.Time) error  { return nil }
func (c readOnlyConn) SetWriteDeadline(_ time.Time) error { return nil }

// bufferedConn replays the peeked bytes before falling through to the wire.
type bufferedConn struct {
	net.Conn
	r io.Reader
}

func (c *bufferedConn) Read(p []byte) (int, error) { return c.r.Read(p) }

// singleConnListener turns one TLS connection into something http.Server.Serve
// will accept. The first Accept returns the connection; later calls block until
// the connection closes instead of erroring out, so cleanup never races the
// serving goroutine. Took longer to get right than it looks.
type singleConnListener struct {
	conn   net.Conn
	closed chan struct{}
	once   sync.Once
}

func newSingleConnListener(conn net.Conn) *singleConnListener {
	l := &singleConnListener{closed: make(chan struct{})}
	l.conn = &trackedConn{Conn: conn, onClose: func() { l.once.Do(func() { close(l.closed) }) }}
	return l
}

func (l *singleConnListener) Accept() (net.Conn, error) {
	if l.conn != nil {
		c := l.conn
		l.conn = nil
		return c, nil
	}
	<-l.closed
	return nil, net.ErrClosed
}

func (l *singleConnListener) Close() error {
	l.once.Do(func() { close(l.closed) })
	return nil
}

func (l *singleConnListener) Addr() net.Addr {
	if l.conn != nil {
		return l.conn.LocalAddr()
	}
	return dummyAddr{}
}

type trackedConn struct {
	net.Conn
	onClose func()
}

func (c *trackedConn) Close() error {
	err := c.Conn.Close()
	if c.onClose != nil {
		onClose := c.onClose
		c.onClose = nil
		onClose()
	}
	return err
}

type dummyAddr struct{}

func (dummyAddr) Network() string { return "tcp" }
func (dummyAddr) String() string  { return "" }

// writeMalformed sends a plain HTTP error for connections without an SNI.
func writeMalformed(conn net.Conn) {
	response := "HTTP/1.1 502 Bad Gateway\r\n" +
		"Content-Type: text/plain; charset=utf-8\r\n" +
		"Content-Length: 19\r\n" +
		"Connection: close\r\n" +
		"\r\n" +
		"malformed TLS request"
	_, _ = conn.Write([]byte(response))
}
