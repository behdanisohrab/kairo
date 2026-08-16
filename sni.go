package main

import (
	"bytes"
	"crypto/tls"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

// StartSNI is the router that makes the split routing real. One listener on
// :443, and every connection is decided by the name in its TLS handshake:
// our own hostname gets DoH and the API, anything else gets tunneled to the
// destination for allowlisted clients.
func StartSNI(s *State, handler http.Handler) {
	ln, err := net.Listen("tcp", cfg.Listen.HTTPS)
	if err != nil {
		log.Fatalf("sni: %v", err)
	}
	log.Printf("SNI router on %s", cfg.Listen.HTTPS)

	var tlsConfig *tls.Config
	if cfg.TLS.Cert != "" && cfg.TLS.Key != "" {
		cer, err := tls.LoadX509KeyPair(cfg.TLS.Cert, cfg.TLS.Key)
		if err != nil {
			log.Printf("SNI TLS termination disabled: %v", err)
		} else {
			tlsConfig = &tls.Config{
				Certificates: []tls.Certificate{cer},
				MinVersion:   tls.VersionTLS12,
			}
		}
	}
	if tlsConfig == nil && cfg.HostBackend == "" {
		log.Printf("warning: neither tls.cert/key nor host_backend is set; DoH/API over %s will not be served", cfg.Listen.HTTPS)
	}

	for {
		conn, err := ln.Accept()
		if err != nil {
			continue
		}
		go s.handleConn(conn, handler, tlsConfig)
	}
}

func (s *State) handleConn(clientConn net.Conn, handler http.Handler, tlsConfig *tls.Config) {
	defer clientConn.Close()

	_ = clientConn.SetReadDeadline(time.Now().Add(5 * time.Second))
	hello, peeked, err := peekClientHello(clientConn)
	if err != nil {
		return
	}
	_ = clientConn.SetReadDeadline(time.Time{})

	sni := strings.ToLower(strings.TrimSpace(hello.ServerName))
	if sni == "" {
		writeMalformed(clientConn)
		return
	}

	// Our own hostname: terminate TLS (or hand it to a reverse proxy) and
	// serve DoH and the API.
	if cfg.Host != "" && sni == cfg.Host {
		if tlsConfig != nil {
			s.serveTLS(clientConn, peeked, handler, tlsConfig)
		} else if cfg.HostBackend != "" {
			s.tunnel(clientConn, peeked, cfg.HostBackend)
		} else {
			log.Printf("no handler for host SNI %q", sni)
		}
		return
	}

	// Everything else is a split-routed destination, and the allowlist gate
	// applies again here. Trusting only the DNS answer would be naive.
	peerIP := remoteIPAddr(clientConn.RemoteAddr())
	if !s.isAllowedIP(peerIP) {
		log.Printf("rejected %s: SNI %s not allowlisted", peerIP, sni)
		return
	}

	s.tunnel(clientConn, peeked, net.JoinHostPort(sni, "443"))
}

// tunnel relays bytes to the backend, replaying the ClientHello bytes we
// already swallowed so the destination still sees the full handshake.
func (s *State) tunnel(clientConn net.Conn, peeked io.Reader, backendAddr string) {
	backend, err := net.DialTimeout("tcp", backendAddr, 5*time.Second)
	if err != nil {
		log.Printf("tunnel dial %s: %v", backendAddr, err)
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
		_, _ = io.Copy(backend, clientConn)
		if tc, ok := backend.(*net.TCPConn); ok {
			_ = tc.CloseWrite()
		}
	}()
	wg.Wait()
}

// serveTLS wraps the already-peeked connection so the TLS layer reads the
// ClientHello exactly once, then serves one HTTP session over it.
func (s *State) serveTLS(clientConn net.Conn, peeked io.Reader, handler http.Handler, tlsConfig *tls.Config) {
	wrap := &bufferedConn{
		Conn: clientConn,
		r:    io.MultiReader(peeked, clientConn),
	}
	tconn := tls.Server(wrap, tlsConfig)
	if err := tconn.Handshake(); err != nil {
		log.Printf("tls handshake for %s failed: %v", cfg.Host, err)
		return
	}
	srv := &http.Server{
		Handler:           handler,
		ReadHeaderTimeout: 10 * time.Second,
	}
	if err := srv.Serve(newSingleConnListener(tconn)); err != nil && err != net.ErrClosed {
		log.Printf("http over TLS for %s: %v", cfg.Host, err)
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
