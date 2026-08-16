# Kairo security model

This document describes what Kairo protects, what it assumes about the network,
and the limits of the protection it provides. Read it before deploying the
service where you rely on it.

## What the service does and does not do

Kairo is a routing aid, not an encrypted tunnel and not a firewall. It changes
the DNS answers and the connection path for a limited set of domains and a
limited set of clients. Everything else is left to the normal network stack.
That distinction matters when you plan what you expect from it.

The parts that are protected are the DNS resolution itself and the identity of
the routed sessions. The DNS servers are encrypted when clients use DoT or DoH,
so a network observer cannot see the queried names on the wire. The tunneled
TLS sessions stay encrypted end to end between the client and the destination,
so even though the traffic crosses the VPS, its contents are invisible to
anyone on the path, including the VPS operator.

The parts that are not protected follow from the routing design. Split routing
is cooperative. A client that ignores the supplied DNS and resolves the real
address of a restricted domain goes straight to the site and bypasses the VPS.
There is no way to force such a client onto the tunnel without full control of
the device. Similarly, only TCP is redirected. The SNI router inspects TCP
handshakes, so UDP traffic, QUIC, and HTTP/3 are not routed through the VPS.
Clients that can use HTTP/3 will fall back to it and bypass the tunnel; blocking
QUIC on the client closes that gap.

## The allowlist as the gate

The allowlist is the central access control. A client only receives the VPS IP
for restricted domains and only gets a tunnel for its connections if its
address is allowlisted. Loopback addresses are always treated as allowed so
that local tools and health checks keep working.

The allowlist is IP based, which has two consequences. First, a client behind a
dynamic address changes its identity when the address changes, and it stops
being routed until the new address is allowlisted. The IP generator exists
exactly for this case: point it at a dynamic DNS name and the allowlist follows
the client. Second, anyone who can spoof or share an allowlisted address is
treated as the trusted client. This is acceptable in a home network but worth
remembering in a shared environment.

## Protecting the API

The management API can change both policy lists, so its protection matters more
than anything else the service does. It is guarded by a single shared key, and
Kairo applies several measures around it.

The key comparison is constant-time to avoid timing side channels. The key is
accepted over a query parameter, a header, or a Bearer token, which gives
clients a choice of transport. Because the key can appear in a URL, you should
use HTTPS for every API call so that the URL is not visible on the wire. A long
random key is essential; the sample configuration contains a placeholder that
must be replaced. The API is rate limited independently of DNS, and every
mutation is logged.

The same API is reachable on the loopback HTTP backend on port 8080. That
listener is bound to the loopback interface only and is meant to sit behind a
reverse proxy. If you use it, keep the proxy on the same host and make sure the
port is not exposed externally.

## Trusting the reverse proxy

The DoH and API handlers need to know the real client IP to apply the
allowlist. When the service runs behind a reverse proxy, the direct peer is the
proxy itself and the real client address arrives in the `X-Forwarded-For`
header. Kairo only trusts that header when the direct peer is the loopback
interface, meaning a local proxy. A header arriving from any other peer is
ignored and the socket address is used instead. This prevents a remote client
from impersonating an allowlisted address by sending a forged header.

The SNI tunnel has the same problem in a different form. If a stream proxy
sits in front of `:443` and forwards unknown SNIs straight to the destination,
then a client that adds the VPS IP to its hosts file reaches the tunnel without
ever touching the allowlist. The `proxy_protocol` setting closes that gap: the
proxy sends the real client address in a PROXY protocol v1 header, and Kairo
uses it for the tunnel gate. The header is trusted only from a loopback peer,
so a remote client cannot forge it.

## Network exposure

The service binds four listeners. The DNS and DoT listeners and the SNI router
must be reachable from the clients that use the service, so they are exposed to
the network. The HTTP backend is loopback only. On a bare server, firewall the
management API to known networks if you want defense in depth beyond the API
key. In Docker, host networking means the container has the same exposure as the
host, and the firewall rules on the host apply unchanged.

The DNS server answers restricted domains with the VPS IP only for allowlisted
clients. Other clients get the normal upstream answer, so an unlisted device on
your network still resolves everything correctly, it just is not routed through
the VPS.

## Operational hardening checklist

Use a long random API key and rotate it if it may have leaked. Serve DoH and the
API over HTTPS with a valid certificate. Keep the policy files readable only by
the service account, since they are the only state that matters. Monitor the
health endpoint and the log for upstream failures. Block QUIC and HTTP/3 on
clients if the tunnel must be the only path for restricted traffic. Review the
restricted list periodically, because every entry changes DNS answers for the
whole allowlisted client base.
