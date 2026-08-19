# Kairo overview

Kairo implements domain-based split routing in a single binary. It combines a
DNS server with a policy engine and an SNI router so that only traffic for a
restricted set of domains is sent through your VPS, while everything else keeps
using the client's normal connection. This design is an implementation of the
gateway originally described by the bepass-org/smartSNI project.

## The problem

A normal DNS resolver answers every domain with its real address. If the local
network blocks or interferes with some of those addresses, the affected domains
are simply unreachable. A full proxy solves that but forces every byte of
traffic through the VPS, which is slow, wastes bandwidth, and changes the
address seen by every service.

## The approach

Kairo keeps the two concerns separate. A policy engine decides what to do with a
DNS query, and an SNI router decides what to do with a TLS connection.

The policy engine looks at two lists. The first list holds restricted domains,
for example social networks or any service you want to reach through the VPS.
The second list holds allowlisted client IPs, which are the machines allowed to
use the split routing at all. When a query arrives for a restricted domain from
an allowlisted client, Kairo answers with the VPS IP instead of the real one.
IPv6 answers for those domains are suppressed so that the traffic is forced over
IPv4, where the SNI router sits. Every other query is forwarded unchanged to an
upstream resolver.

The SNI router listens on port 443 and inspects the Server Name Indication of
every TLS handshake. If the name belongs to a restricted destination and the
client is allowlisted, the router opens a connection to the real server and
relays the bytes in both directions. The client and the destination still
complete a real TLS handshake with each other, so the session stays encrypted
end to end. If the name is the configured hostname of the VPS, the router
answers locally with the DoH endpoint and the management API. Anything else is
rejected.

The result is transparent from the client's point of view. There is no proxy
software to install and no certificate to trust. The client only has to point
its DNS at the VPS, and the routing happens automatically based on the DNS
answers it receives.

## The components

The service exposes five listeners. A plain DNS server on port 53 accepts UDP
and TCP queries. A DNS over TLS server on port 853 provides the same answers
over an encrypted connection. A DNS over HTTPS endpoint is served over the SNI
router on port 443 and again over a loopback HTTP listener on port 8080 for use
behind a reverse proxy. The SNI router itself lives on port 443 and carries both
the DoH and API traffic for your hostname and the transparent tunnel for
restricted domains. A separate loopback listener on port 9090 serves the
Prometheus `/metrics` endpoint so it is never exposed publicly.

All policy lives in a data directory as three plain text files. The `domains.txt`
file lists restricted domains. The `allowed.txt` file lists client IPs. The
`domain.txt` file feeds the IP generator, which resolves every domain in it and
merges the resulting addresses into the allowlist. Kairo watches the first two
files and applies edits without a restart, and every change made through the API
is written back to disk atomically.

## Two routing decisions

A client only receives the VPS IP for a restricted domain if it is also in the
allowlist. An unknown client gets normal answers and therefore never reaches the
SNI router for a tunneled connection. The allowlist is what limits split routing
to the machines you own, for example your home router or your phone, while
anyone else on your network keeps plain connectivity.

The routing decision is made twice. First the DNS answer sends the allowlisted
client to the VPS. Then the SNI router checks the allowlist again before it
opens a tunnel. Both checks must pass, and together they mean that unrestricted
domains never touch the VPS at all.

## What is guaranteed and what is not

Split routing is cooperative. A client that bypasses your DNS and resolves the
real address directly is not routed through the VPS, because there is no way to
force traffic without full device control. Only TCP traffic is tunneled. UDP
and HTTP/3 sessions are not redirected, since the SNI router only inspects TCP
handshakes. For strict enforcement you should block QUIC on the client. These
limits are discussed further in the security document.

The next documents cover the configuration format, deployment and operations,
the management API, and the security model in detail.
