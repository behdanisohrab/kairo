package sni

import (
	"bufio"
	"crypto/tls"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"

	"kairo/internal/config"
	"kairo/internal/state"
)

func dummyHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {})
}

func newBufReader(s string) *bufio.Reader {
	return bufio.NewReader(strings.NewReader(s))
}

func newTestState(t *testing.T) *state.State {
	t.Helper()
	dir := t.TempDir()
	st, err := state.NewState(&config.Config{DataDir: dir})
	if err != nil {
		t.Fatalf("NewState: %v", err)
	}
	return st
}

// clientHelloBytes builds a real TLS ClientHello for the given server name,
// the way a browser would send it.
func clientHelloBytes(serverName string) ([]byte, error) {
	clientSide, serverSide := net.Pipe()
	done := make(chan struct{})
	go func() {
		tc := tls.Client(clientSide, &tls.Config{InsecureSkipVerify: true, ServerName: serverName})
		_ = tc.Handshake()
		_ = tc.Close()
		close(done)
	}()
	hdr := make([]byte, 5)
	if _, err := io.ReadFull(serverSide, hdr); err != nil {
		return nil, err
	}
	body := make([]byte, int(hdr[3])<<8|int(hdr[4]))
	if _, err := io.ReadFull(serverSide, body); err != nil {
		return nil, err
	}
	clientSide.Close()
	serverSide.Close()
	<-done
	return append(hdr, body...), nil
}

// startRouter accepts one connection and feeds it to handleConn. stop() waits
// until handleConn returns, so call it after closing the client connection.
func startRouter(cfg *config.Config, st *state.State) (addr string, stop func()) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		panic(err)
	}
	done := make(chan struct{})
	go func() {
		conn, err := ln.Accept()
		if err == nil {
			handleConn(cfg, st, nil, conn, dummyHandler(), nil)
		}
		close(done)
	}()
	return ln.Addr().String(), func() {
		ln.Close()
		<-done
	}
}

// spinTunnelTarget returns a listener that records the first bytes it
// receives, which is enough to prove the ClientHello was replayed.
func spinTunnelTarget(t *testing.T) (addr string, recv chan []byte) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	recv = make(chan []byte, 1)
	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		buf := make([]byte, 4096)
		_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))
		n, _ := conn.Read(buf)
		recv <- buf[:n]
	}()
	return ln.Addr().String(), recv
}

func TestProxyProtocolGate(t *testing.T) {
	cfg := &config.Config{ProxyProtocol: true, Host: "dns.test", VPSIP: "203.0.113.10"}
	st := newTestState(t)
	if _, err := st.AddAllowed(net.ParseIP("198.51.100.7")); err != nil {
		t.Fatalf("AddAllowed: %v", err)
	}

	hello, err := clientHelloBytes("example.com")
	if err != nil {
		t.Fatalf("clientHelloBytes: %v", err)
	}

	t.Run("allowlisted source in PROXY header is tunneled", func(t *testing.T) {
		backendAddr, recv := spinTunnelTarget(t)
		st.TunnelAddr = func(string) string { return backendAddr }

		routerAddr, stop := startRouter(cfg, st)
		conn, err := net.Dial("tcp", routerAddr)
		if err != nil {
			t.Fatal(err)
		}
		_, _ = fmt.Fprintf(conn, "PROXY TCP4 198.51.100.7 84.245.19.105 5555 443\r\n")
		_, _ = conn.Write(hello)

		select {
		case got := <-recv:
			if len(got) < len(hello) {
				t.Fatalf("tunnel target received %d bytes, want at least %d", len(got), len(hello))
			}
			if string(got[:len(hello)]) != string(hello) {
				t.Errorf("replayed bytes differ from the ClientHello, and no PROXY header may reach the target")
			}
		case <-time.After(3 * time.Second):
			t.Fatal("tunnel target never received a connection")
		}
		conn.Close()
		stop()
	})

	t.Run("unallowlisted source in PROXY header is rejected", func(t *testing.T) {
		_, recv := spinTunnelTarget(t)
		st.TunnelAddr = func(string) string { return "127.0.0.1:1" }

		routerAddr, stop := startRouter(cfg, st)
		conn, err := net.Dial("tcp", routerAddr)
		if err != nil {
			t.Fatal(err)
		}
		_, _ = fmt.Fprintf(conn, "PROXY TCP4 198.51.100.8 84.245.19.105 5555 443\r\n")
		_, _ = conn.Write(hello)

		select {
		case <-recv:
			t.Error("tunnel target was reached by an unallowlisted client")
		case <-time.After(500 * time.Millisecond):
		}
		conn.Close()
		stop()
	})
}

func TestRejectsProxyHeaderWhenDisabled(t *testing.T) {
	cfg := &config.Config{ProxyProtocol: false, Host: "dns.test", VPSIP: "203.0.113.10"}
	st := newTestState(t)
	if _, err := st.AddAllowed(net.ParseIP("198.51.100.7")); err != nil {
		t.Fatalf("AddAllowed: %v", err)
	}

	hello, err := clientHelloBytes("example.com")
	if err != nil {
		t.Fatalf("clientHelloBytes: %v", err)
	}

	_, recv := spinTunnelTarget(t)
	st.TunnelAddr = func(string) string { return "127.0.0.1:1" }

	routerAddr, stop := startRouter(cfg, st)
	conn, err := net.Dial("tcp", routerAddr)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = fmt.Fprintf(conn, "PROXY TCP4 198.51.100.7 84.245.19.105 5555 443\r\n")
	_, _ = conn.Write(hello)

	select {
	case <-recv:
		t.Error("PROXY header must be treated as garbage when proxy_protocol is off")
	case <-time.After(500 * time.Millisecond):
	}
	conn.Close()
	stop()
}

func TestReadProxyHeader(t *testing.T) {
	t.Run("plain TLS bytes are not a PROXY header", func(t *testing.T) {
		r := newBufReader("\x16\x03\x01...tls")
		ip, err := readProxyHeader(r)
		if err != nil {
			t.Fatalf("readProxyHeader: %v", err)
		}
		if ip != nil {
			t.Errorf("ip = %v, want nil", ip)
		}
		rest, _ := io.ReadAll(r)
		if string(rest) != "\x16\x03\x01...tls" {
			t.Errorf("rest = %q, want original bytes", rest)
		}
	})

	t.Run("parses TCP4 header", func(t *testing.T) {
		r := newBufReader("PROXY TCP4 198.51.100.7 84.245.19.105 50210 443\r\nTLSBYTES")
		ip, err := readProxyHeader(r)
		if err != nil {
			t.Fatalf("readProxyHeader: %v", err)
		}
		if ip == nil || ip.String() != "198.51.100.7" {
			t.Errorf("ip = %v, want 198.51.100.7", ip)
		}
		rest, _ := io.ReadAll(r)
		if string(rest) != "TLSBYTES" {
			t.Errorf("rest = %q, want TLSBYTES", rest)
		}
	})

	t.Run("parses TCP6 header", func(t *testing.T) {
		r := newBufReader("PROXY TCP6 2001:db8::7 2001:db8::1 1234 443\r\n")
		ip, err := readProxyHeader(r)
		if err != nil {
			t.Fatalf("readProxyHeader: %v", err)
		}
		if ip == nil || ip.String() != "2001:db8::7" {
			t.Errorf("ip = %v, want 2001:db8::7", ip)
		}
	})

	t.Run("UNKNOWN form carries no address", func(t *testing.T) {
		r := newBufReader("PROXY UNKNOWN\r\n")
		ip, err := readProxyHeader(r)
		if err != nil {
			t.Fatalf("readProxyHeader: %v", err)
		}
		if ip != nil {
			t.Errorf("ip = %v, want nil", ip)
		}
	})

	for _, bad := range []string{"PROXY TCP4 nope 1.2.3.4 1 2\r\n", "PROXY TCP7 1.2.3.4 1.2.3.4 1 2\r\n", "PROXY TCP4 1.2.3.4 1.2.3.4 1\r\n"} {
		t.Run("rejects garbage "+bad[:20], func(t *testing.T) {
			r := newBufReader(bad)
			if _, err := readProxyHeader(r); err == nil {
				t.Errorf("readProxyHeader(%q) should fail", bad)
			}
		})
	}
}
