# Kairo API reference

Kairo exposes a JSON API for auth, users, allowlist, restricted/direct
domains, and domain requests. It is served over the SNI router at
`https://<host>/api/...` (requires TLS with SNI=`host`), and over the loopback
HTTP backend at `http://127.0.0.1:8080/api/...` (plain HTTP, no SNI needed;
ideal for local `curl` and Vite proxy).

Local curl without TLS:
```bash
curl http://127.0.0.1:8080/api/status -H "Authorization: Bearer <key>"
# SNI router (needs SNI):
curl -k --resolve dns.example.com:8443:127.0.0.1 https://dns.example.com:8443/api/status -H "Authorization: Bearer <key>"
```

## Authentication

Two mechanisms, checked in order:

1. **Session cookie** `kairo_session` (`HttpOnly`, `SameSite=Lax`, `Secure` when
   `r.TLS` or `X-Forwarded-Proto:https` or host not `localhost`/`127.0.0.1`;
   TTL `session_ttl` hours, hourly `ExpireOldSessions` purge, `expires_at > now` enforced).

2. **API key** fallback: `?key=` query, `X-API-Key` header, or `Authorization: Bearer <key>` (constant-time compare). The legacy `api_key` from `config.yaml` is treated as `admin`.

```bash
curl "https://dns.example.com/api/allow?key=<key>"
curl -H "X-API-Key: <key>" "https://dns.example.com/api/allow"
curl -H "Authorization: Bearer <key>" "https://dns.example.com/api/allow"
# also: curl -b "kairo_session=<id>" https://dns.example.com/api/auth/me
```

Missing/invalid auth → `401`. Admin-only endpoints → `403`. Global `rate.api` → `429`.

All mutating user-creation and login bodies are `MaxBytesReader` limited (2K-4K) and
validated (`username ^[a-zA-Z0-9._-]{3,32}`, `password 6-128`, `rate_limit ≤10000`,
domain `3-253` no space/slash).

## Responses

```json
{ "ok": true, "data": ["198.51.100.7"] }
{ "ok": true, "message": "IP allowlisted" }
{ "ok": false, "error": "invalid or missing API key" }
```

Security headers are set on every response:
`X-Frame-Options: SAMEORIGIN`, `X-Content-Type-Options: nosniff`,
`Referrer-Policy: strict-origin-when-cross-origin`,
`Content-Security-Policy` (strict for UI, `default-src 'none'` for API),
`Permissions-Policy`. CORS is allowed for origins matching `admin_url`, `doh_url`,
or `host` (`Allow-Credentials`, `Allow-Headers: Content-Type, Authorization, X-API-Key`).

## Auth

### POST /api/auth/login
```bash
curl -X POST -H "Content-Type: application/json" \
  -d '{"username":"admin","password":"..."}' \
  https://dns.example.com/api/auth/login
# → {ok:true, user:{id,username,role}} + Set-Cookie: kairo_session
```
Creates a session (`CreateSession` with `IP` from `X-Forwarded-For` if loopback, else `RemoteAddr`, plus `User-Agent`, `TTL session_ttl`) and updates `last_login`.

### POST /api/auth/logout
```bash
curl -X POST -b "kairo_session=..." https://dns.example.com/api/auth/logout
# also: curl -X POST -H "Authorization: Bearer ..." https://dns.example.com/api/auth/logout
```
Deletes the session and clears the cookie (`MaxAge:-1`, same `Secure` logic).

### GET /api/auth/me
```bash
curl -b "kairo_session=..." https://dns.example.com/api/auth/me
curl -H "Authorization: Bearer <key>" https://dns.example.com/api/auth/me
# → {ok:true, user:{id,username,role,api_key,rate_limit}}
```

## Public config

### GET /api/public-config  (any auth)
Returns the public URLs derived from config (or empty if `host` not set). Used by
the Guide to show the correct DoH URL when `doh_url` is custom.

```bash
curl -H "Authorization: Bearer <key>" https://dns.example.com/api/public-config
# → {ok:true, admin_url:"https://panel.example.com/", doh_url:"https://dns.example.com/dns-query", host:"dns.example.com"}
```

## Users (admin)

### GET /api/users
List all users sorted by `id`.

### POST /api/users
```bash
curl -X POST -H "Content-Type: application/json" \
  -d '{"username":"alice","password":"secret123","rate_limit":100}' \
  -H "Authorization: Bearer <admin_key>" https://dns.example.com/api/users
# → 201 {ok:true, user:{id,username,api_key,role,rate_limit}}
```
Validates username/password, `bcrypt` hash, random `hex(32)` API key, default `role=user`.

### DELETE /api/users/:id
Atomic `DELETE FROM sessions WHERE user_id=?` + `domain_requests` + `users` in a transaction. `admin` cannot be deleted → `403`.

### POST /api/users/:id/api-key/regenerate
```bash
curl -X POST -H "Authorization: Bearer <admin>" https://dns.example.com/api/users/4/api-key/regenerate
# → {ok:true, api_key:"..."}
```

## Traffic analytics

Device tracking was removed (unreliable JA3+IP rows); every tunnelled
connection is logged as `(ip, user_id, domain)` and attributed to the account
that allowlisted the source IP. Rows older than 30 days are pruned hourly.

### GET /api/traffic?range=24h  (admin)
`range` accepts `1h`, `24h` (default), `7d`, `30d`. Returns
`{connections, unique_ips, buckets:[{bucket,count}] (hourly UTC),
top_domains:[{name,count}], top_users:[{name,count}], recent:[...],
allowlisted, restricted, direct, total_users, uptime_seconds, version}`.

### GET /api/me/traffic?range=24h  (any auth)
Caller-scoped: `{total_requests, unique_domains, buckets, recent,
rate_limit, unlimited}`.

The removed device endpoints answer **410 Gone** with a pointer to
`/api/traffic`: `GET /api/devices`, `GET /api/me/devices`,
`GET /api/users/:id/devices`.

### POST /api/me/api-key/regenerate  (any auth)
Regenerates the caller's own API key.

## Managing the allowlist

The client allowlist lives in the database (`user_allowed_ips`), one set of
rows per user; DNS and SNI gate on the union of all users' IPs. `allowed.txt`
from 0.2.x is imported into the admin account once at startup and retired as
`allowed.txt.legacy`.

### GET /api/allow  (admin)
Sorted union of all users' allowlisted IPs.

### POST /api/allow?ip=1.2.3.4  (any auth)
Stored on the **calling account** — whoever authenticates (session cookie or
API key) owns the entry, so panel and API stay in sync. `net.ParseIP`
validated, loopback rejected, `409` if already on your account, `403` when a
non-admin hits their `ip_limit`. Takes effect for DNS/SNI immediately.

### DELETE /api/allow?ip=1.2.3.4  (any auth)
Removes the IP from the calling account (`404` if not present there). The
global gate keeps routing it until the last owner removes it.

Per-user management endpoints: `GET/POST/DELETE /api/me/ips`,
`GET/POST/DELETE /api/users/:id/ips` (admin).

## Managing the restricted list

### GET /api/restricted  (admin)
Sorted `domains.txt` (subdomains covered: `youtube.com` matches `www.youtube.com` via `IsRestricted` parent walk).

### POST /api/restricted?domain=instagram.com  (admin)
`NormalizeDomain` lower, trim, `409` if duplicate, `400` if empty.

### DELETE /api/restricted?domain=instagram.com  (admin)
`404` if not present.

## Direct-mode domains

Restricted names listed here are answered with real upstream records instead
of the VPS IP — for sites whose connection is killed mid-tunnel by external
filtering but load fine when connected directly. Stored in `data/direct.txt`,
parent matching applies (`youtube.com` covers `www.youtube.com`).

### GET /api/direct  (admin)
Sorted direct-mode list.

### POST /api/direct?domain=youtube.com  (admin)
`409` if already direct, `400` if empty. Takes effect immediately (hot-reload).

### DELETE /api/direct?domain=youtube.com  (admin)
Returns the name to tunnelled answering; `404` if not present.

## Domain check and requests

### GET /api/domain/check?domain=example.com  (any auth)
No admin needed. Returns whether the domain is proxied via `s.st.IsRestricted` (parent walk), `400` if missing/`>253`.

```bash
curl -H "Authorization: Bearer <key>" "https://dns.example.com/api/domain/check?domain=example.com"
# → {ok:true, restricted:true/false, domain:"example.com"}
```

### POST /api/domain/request  (any auth)
Body `{"domain":"example.com"}` or `?domain=` fallback, `MaxBytesReader 1K`, `3-253` no space/slash, `409` if already restricted or `UNIQUE(user_id,domain)` duplicate. Inserts into `domain_requests` (`pending`).

```bash
curl -X POST -H "Content-Type: application/json" -d '{"domain":"example.com"}' \
  -H "Authorization: Bearer <key>" https://dns.example.com/api/domain/request
# → 201 {ok:true, request:{id,user_id,username,domain,status,created_at}}
```

### GET /api/domain/requests  (admin)
```bash
curl -H "Authorization: Bearer <admin>" https://dns.example.com/api/domain/requests
# → {ok:true, requests:[{id,user_id,username,domain,status,created_at}]}
```

### POST /api/domain/requests/:id/approve  (admin)
Looks up request, `AddRestricted(domain)` (hot-reload via `saveDomains()`; `already` is tolerated), then `UPDATE status='approved'`.

### POST /api/domain/requests/:id/reject  (admin)
`UPDATE status='rejected'`.

## Generating the allowlist from the IP source

```bash
curl -X POST -H "Authorization: Bearer <key>" https://dns.example.com/api/generate
# → {ok:true, added:4, unresolved:0, total:6, message:"allowlist regenerated"}
```

## Status

```bash
curl -H "Authorization: Bearer <key>" https://dns.example.com/api/status
```
Admin only. Returns `version`, `host`, `admin_url`, `doh_url`, `vps_ip`, `uptime_seconds`, `allowlisted`, `restricted`, `upstream_dns`, `ip_source` (+ interval). `admin_url`/`doh_url` are `Effective*URL()` (derived from `host` if empty).

## Other endpoints

`GET /` serves the SPA from `web/dist` (fallback `index.html`) or the status template if `web_dir` missing. `GET /healthz` → `ok` (no auth). `GET /metrics` on `127.0.0.1:9090` (Prometheus, loopback-only).

## Web UI

The UI is built with `bun` (`oven/bun:1-alpine` in Docker, `web/bun.lock`), Vite 8, React 19, Tailwind 4, `react-icons`. Routes: `/login`, `/admin` (Overview with system status), `/admin/users`, `/admin/domains` (Tunnelled/Direct tabs), `/traffic` (charts + sortable top tables, both roles), `/dashboard`, `/guide`. Features: light/dark (`system`), EN/FA RTL (`Vazirmatn`), responsive minimal design, domain/IP management (hot-reload), user domain checker (`/api/domain/check` + request).
