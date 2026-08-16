package main

import (
	"crypto/tls"
	"fmt"
	"io"
	"net"
	"testing"
	"time"
)

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
func startRouter(s *State) (addr string, stop func()) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		panic(err)
	}
	done := make(chan struct{})
	go func() {
		conn, err := ln.Accept()
		if err == nil {
			s.handleConn(conn, buildHandler(s), nil)
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

func TestSNIProxyProtocolGate(t *testing.T) {
	cfg = &Config{ProxyProtocol: true, Host: "dns.test", VPSIP: "203.0.113.10"}
	s := newTestState(t)
	if _, err := s.AddAllowed(net.ParseIP("198.51.100.7")); err != nil {
		t.Fatalf("AddAllowed: %v", err)
	}

	hello, err := clientHelloBytes("example.com")
	if err != nil {
		t.Fatalf("clientHelloBytes: %v", err)
	}

	t.Run("allowlisted source in PROXY header is tunneled", func(t *testing.T) {
		backendAddr, recv := spinTunnelTarget(t)
		s.tunnelAddr = func(string) string { return backendAddr }

		routerAddr, stop := startRouter(s)
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
		s.tunnelAddr = func(string) string { return "127.0.0.1:1" }

		routerAddr, stop := startRouter(s)
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

func TestSNIRejectsProxyHeaderWhenDisabled(t *testing.T) {
	cfg = &Config{ProxyProtocol: false, Host: "dns.test", VPSIP: "203.0.113.10"}
	s := newTestState(t)
	if _, err := s.AddAllowed(net.ParseIP("198.51.100.7")); err != nil {
		t.Fatalf("AddAllowed: %v", err)
	}

	hello, err := clientHelloBytes("example.com")
	if err != nil {
		t.Fatalf("clientHelloBytes: %v", err)
	}

	_, recv := spinTunnelTarget(t)
	s.tunnelAddr = func(string) string { return "127.0.0.1:1" }

	routerAddr, stop := startRouter(s)
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
