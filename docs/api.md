# Kairo API reference

Kairo exposes a small JSON API for managing the allowlist and the restricted
domain list. It is served on the same endpoints as DoH: over the SNI router at
`https://<host>/api/...`, and over the loopback HTTP backend at
`http://127.0.0.1:8080/api/...`.

## Authentication

Every request must present the shared API key. It can be sent as a `key` query
parameter, as an `X-API-Key` header, or as a `Bearer` token in the
`Authorization` header. All three forms are equivalent.

```bash
curl "https://dns.example.com/api/allow?key=<key>"
curl -H "X-API-Key: <key>" "https://dns.example.com/api/allow"
curl -H "Authorization: Bearer <key>" "https://dns.example.com/api/allow"
```

Requests without a valid key receive a 401 response. The comparison is
constant-time, so the endpoint does not leak timing information about the key.
Rate limiting applies to the whole API: a request beyond the configured `rate.api`
limit receives a 429 response.

## Responses

Successful responses have an `ok` field set to `true`. Endpoints that return a
list put it in a `data` array. Endpoints that perform an action return a
`message`. Failed responses have `ok` set to `false` and an `error` string.

```json
{ "ok": true, "data": ["198.51.100.7"] }
{ "ok": true, "message": "IP allowlisted" }
{ "ok": false, "error": "invalid or missing API key" }
```

## Managing the allowlist

The allowlist controls which client IPs receive the VPS IP for restricted
domains and are permitted to use the transparent SNI tunnel.

### List allowed clients

```bash
curl "https://dns.example.com/api/allow?key=<key>"
```

Returns the current allowlist as a sorted array of IP strings.

### Allow a client

```bash
curl -X POST "https://dns.example.com/api/allow?key=<key>&ip=198.51.100.7"
```

Adds the given IPv4 or IPv6 address. Loopback addresses are rejected because
they are always allowed implicitly. Adding an address that is already present
returns a 409 response, and the change is written to `allowed.txt` immediately.

### Remove a client

```bash
curl -X DELETE "https://dns.example.com/api/allow?key=<key>&ip=198.51.100.7"
```

Removes the address. Removing an address that is not present returns a 404
response.

## Managing the restricted list

The restricted list holds the domains that are answered with the VPS IP for
allowlisted clients. Restricting a domain covers all of its subdomains.

### List restricted domains

```bash
curl "https://dns.example.com/api/restricted?key=<key>"
```

Returns the current restricted domains as a sorted array.

### Add a restricted domain

```bash
curl -X POST "https://dns.example.com/api/restricted?key=<key>&domain=instagram.com"
```

Adds the domain. A duplicate entry returns a 409 response. The list is written
to `domains.txt` immediately.

### Remove a restricted domain

```bash
curl -X DELETE "https://dns.example.com/api/restricted?key=<key>&domain=instagram.com"
```

Removes the domain. A missing entry returns a 404 response.

## Generating the allowlist from the IP source

```bash
curl -X POST "https://dns.example.com/api/generate?key=<key>"
```

Resolves every domain in the `ip_source.domains_file` file and merges the
resulting addresses into the allowlist. It returns the number of newly added
addresses, the number of unresolved domains, and the new total.

```json
{ "added": 4, "message": "allowlist regenerated", "ok": true, "total": 6, "unresolved": 0 }
```

The same operation is available offline through the `-gen-ips` flag, and runs
automatically in the background when `ip_source.interval` is greater than zero.

## Status

```bash
curl "https://dns.example.com/api/status?key=<key>"
```

Returns a snapshot of the running service, including the version, the
configured hostname and VPS IP, the uptime in seconds, both policy lists, the
upstream resolvers, and the IP source path and interval. It is useful for
monitoring and for verifying that a configuration change took effect.

## Other endpoints

The root path serves a human-readable status page. The `GET /healthz` endpoint
returns a plain `ok` and is meant for health checks; it requires no API key.
