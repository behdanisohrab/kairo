# Kairo operations

This document describes how to deploy, run, and maintain Kairo in production,
including the web panel, DNS for clients, and the tasks that keep the service
healthy.

## Deployment layout

A typical installation puts the binary, the configuration, and the data
directory under `/opt/kairo`, plus the built web UI in `web/dist`. The service
needs the listeners on port 53, 443, and 853, which are privileged ports. On
Linux you can run it as root, or grant the single binary the right capability:

```bash
sudo setcap cap_net_bind_service=+ep /opt/kairo/kairo
```

With that capability a regular user can bind the privileged ports without a
superuser service account. Everything else the service writes is confined to the
data directory (`kairo.db` + `allowed.txt`/`domains.txt`/`domain.txt`).

## Running as a systemd service

```ini
[Unit]
Description=Kairo split-routing gateway
After=network-online.target
Wants=network-online.target

[Service]
User=kairo
Group=kairo
WorkingDirectory=/opt/kairo
ExecStart=/opt/kairo/kairo run --config /opt/kairo/config.yaml
Restart=always
RestartSec=3
LimitNOFILE=65536

[Install]
WantedBy=multi-user.target
```

Install it as `/etc/systemd/system/kairo.service`, then run
`sudo systemctl daemon-reload` and `sudo systemctl enable --now kairo`. Check the
log with `journalctl -u kairo -f`. The startup log prints the version, the
hostname, `admin_url`/`doh_url`, and the counts of restricted domains and
allowlisted clients.

## Running in Docker

The included `Dockerfile` builds the Go binary (`golang:1.25-alpine`) and the
web UI (`oven/bun:1-alpine` with `bun install --frozen-lockfile && bun run build`
→ `web/dist`), then copies both into `alpine:3.20`. The compose file deliberately
uses host networking. Published ports would hide the client IP address behind NAT,
which breaks the allowlist, and they cannot remap privileged ports without extra
configuration. With host networking the container sees the real client addresses.

```bash
mkdir -p configs
docker run --rm -v "$PWD/configs:/configs" ghcr.io/behdanisohrab/kairo:latest generate config /configs
# edit configs/config.yaml (host, vps_ip, admin_url, doh_url, api_key)
docker compose up -d --build
docker compose logs -f
```

The `configs` directory is mounted at `/data` inside the container, so `kairo.db`
and policy files plus any certificate under `<data_dir>/certs` survive rebuilds.
`web/dist` is baked into the image at `/app/web/dist` (`web_dir` default).
`configs/` and `web/dist`/`web/node_modules` are now in `.gitignore`; the test
`kairo.db` is removed before commit and not pushed.

## Frontend build

Outside Docker:

```bash
cd web
bun install          # respects bun.lock + bunfig.toml
bun run build        # tsc -b + vite build → web/dist (SPA, chunked vendor + lazy routes)
bun run dev          # Vite dev at :5173, proxies /api → http://127.0.0.1:8080 (plain HTTP, no SNI)
bun run preview      # preview prod build
bun run lint         # oxlint
bun run typecheck    # tsc -b --noEmit
```

The backend serves `web/dist` as static files with SPA fallback (`index.html`);
if `web_dir` is missing the old status template is served at `/`.

## Metrics

Kairo exposes Prometheus metrics at `GET /metrics` on a dedicated listener,
bound to `127.0.0.1:9090` by default (`listen.metrics`). It is never served on
the public DoH/API/SNI surface. The registry includes DNS queries by type and
outcome, SNI connections by outcome, HTTP request counts, rate-limited requests,
and live gauges for the number of allowlisted clients and restricted domains.

```yaml
# config.yaml
listen:
  metrics: "127.0.0.1:9090"
```

## Obtaining a certificate

DoT and the SNI-based DoH endpoint present a certificate for the `host` name.

The recommended way is to let Kairo manage it automatically with lego and the
`http-01` challenge. Set `acme.email` in the config, make sure port 80 is open
and forwarded to the VPS, and confirm `host` resolves to `vps_ip`. On the first
start Kairo registers an ACME account, obtains the certificate and renews it
before it expires; the account key and certificate live under
`<data_dir>/certs` (inside the `/data` volume in Docker).

Alternatively, point `tls.cert`/`tls.key` at hand-managed files, e.g. from
certbot, while `acme.email` is empty. When neither is set, `host_backend`
names a local TLS endpoint (e.g. reverse proxy on `127.0.0.1:8443`) and the
router forwards `host` traffic there without terminating TLS.

## Configuring clients

A client is ready in two steps. First its public IP must be in the allowlist.
Second its DNS must point at the VPS.

```bash
# loopback (local, no TLS)
curl -X POST "http://127.0.0.1:8080/api/allow?ip=<client-ip>" -H "Authorization: Bearer <key>"
# SNI router (needs SNI=host)
curl -k --resolve dns.example.com:8443:127.0.0.1 -X POST "https://dns.example.com:8443/api/allow?ip=<client-ip>" -H "Authorization: Bearer <key>"
```

For the DNS side, plain DNS is the simplest and works with any router. Set the
DNS server of the client network to `<vps_ip>`. DoH uses the `doh_url` shown in
the Guide (`https://dns.example.com/dns-query` by default, also
`http://127.0.0.1:8080/dns-query` locally and `tls://dns.example.com:853` for
DoT). Clients should disable QUIC and HTTP/3 where possible, since those
protocols bypass the SNI router. The Guide in the panel documents per-platform
steps (Windows/macOS/Linux/Android/iOS/Firefox/Chrome) bilingual EN/FA, plus IP
whitelisting (`curl -4 ifconfig.me`) and the domain checker (`Dashboard` →
`Check a domain` → `Request` → admin `Domains` queue).

## The allowlist generator

If your trusted clients have stable public names, the IP generator keeps the
allowlist up to date without manual API calls. List the names in
`domain.txt`, set `ip_source.interval` in seconds, and Kairo resolves them in
the background (merged, never removed). For a one-off refresh, run
`kairo gen-ips` or call `POST /api/generate`.

## Web panel operations

The panel is at `admin_url` (default `https://<host>/`). Features:

- **Auth**: `POST /api/auth/login` → `kairo_session` cookie, `GET /api/auth/me`,
  `POST /api/auth/logout`, or `Bearer` API key. `POST /api/me/api-key/regenerate`.
- **Admin**: `Overview`, `Users` (create/delete, regen key, `GET /users/:id/devices`),
  `Devices` (filter/paginate), `Domains` (single/bulk add, delete, `User requests`
  `Approve`/`Reject` via `POST /api/domain/requests/:id/approve`).
- **User**: `Dashboard` (API key reveal/copy/regenerate, devices, `Check a domain`
  via `GET /api/domain/check` → `POST /api/domain/request`), `Guide` (IP
  whitelist, platform setup, `curl` for DoH `application/dns-message` and
  `GET /api/me/devices`, `doh_url` from `GET /api/public-config`).

All mutating `POST` bodies are `MaxBytesReader` limited (1K-4K) and validated
(`username ^[a-zA-Z0-9._-]{3,32}`, `password 6-128`, `rate_limit ≤10000`,
`domain 3-253`). `DELETE /api/users/:id` is atomic (`sessions` + `domain_requests`
+ `users` in a transaction). `GET /api/public-config` and `GET /api/domain/check`
are available to any authenticated user; `GET /api/domain/requests` and
`approve`/`reject` are admin-only. Security headers (`SAMEORIGIN`, `nosniff`,
`CSP` with Google Fonts, `Permissions-Policy`) and CORS for `admin_url`/`doh_url`
origins are set on every response. Expired sessions are purged hourly.

## Backups and recovery

Everything that matters is in the `config.yaml` file and the `data` directory
with `kairo.db` and `domains.txt`, `domain.txt`. A nightly copy of these is a
complete backup — the client allowlist lives inside `kairo.db`
(`user_allowed_ips`), so the database is what protects your users' IPs. The
files are written atomically, so a copy taken at any moment is either the old
or the new version. An `allowed.txt.legacy` in the data directory is a 0.2.x
relic kept for reference only; it needs no backup.

## Routine maintenance

Watch the log for repeated upstream failures. Review the restricted list
occasionally and remove domains that no longer need routing. When the
certificate renews, confirm the DoT endpoint still connects. The health endpoint
at `/healthz` returns a plain `ok` and is easy to add to a monitoring check.

Check `GET /api/status` (admin, now includes `admin_url`/`doh_url`) and
`GET /api/public-config` (any auth) to verify `host` and URL derivation.

For local curl without TLS always use `http://127.0.0.1:8080` — the SNI router on
`:443`/`:8443` requires TLS with SNI=`host` (`curl: (52) Empty reply from server`
means plain HTTP was sent to the TLS port; fix with `-k --resolve host:443:127.0.0.1`).
