# Kairo configuration

Kairo reads a single YAML file on startup, selected with the `--config` flag of
the `run` subcommand (default `config.yaml`). Every field has a default, so the
file only needs to set the values that differ. This document explains each
field, the file layout, and the command line subcommands.

## The configuration file

The file is split into logical sections. The identity section describes the
public face of the service. The public URL section controls how clients see the
admin panel and DoH. The policy section names the upstream resolver and the
files used for the allowlist and the restricted list. The listener section
defines the network ports. The rate section controls throughput limits. The web
section configures the built frontend.

```yaml
host: dns.example.com
host_backend: ""
admin_url: https://dns.example.com/
doh_url: https://dns.example.com/dns-query
vps_ip: 1.2.3.4
api_key: REPLACE_WITH_A_STRONG_SECRET
admin_password: ""
web_dir: ./web/dist
session_ttl: 24
upstream_dns:
  - 1.1.1.1:53
  - 8.8.8.8:53
ip_source:
  domains_file: domain.txt
  interval: 300
listen:
  dns: ":53"
  dot: ":853"
  https: ":443"
  http: "127.0.0.1:8080"
  metrics: "127.0.0.1:9090"
acme:
  email: ""
  storage: ""
  directory: ""
  renew_before_days: 30
  http_listen: ":80"
tls:
  cert: /etc/letsencrypt/live/dns.example.com/fullchain.pem
  key: /etc/letsencrypt/live/dns.example.com/privkey.pem
data_dir: data
ttl: 300
proxy_protocol: false
rate:
  dns: 200
  dns_burst: 400
  api: 20
  api_burst: 40
```

The example above is generated for you, complete with a fresh random `api_key`,
by `kairo generate config <dir>`.

## Identity

The `host` value is the hostname at which clients reach the DoH endpoint and
the management API. It must match the certificate presented by the server and
it is matched against the SNI of every TLS connection. The SNI router treats a
connection whose ServerName equals `host` as local traffic and serves DoH and
the API for it. A connection for any other name is handled by the tunnel.

The `vps_ip` value is the public IPv4 address of this server. It is the address
returned as the A record for restricted domains, and it is what clients end up
connecting to for those domains. It must be a valid IP address.

The `api_key` value is the shared secret that guards every `/api/*` request
when no user session exists. Choose a long random string and keep it out of
version control. A short or guessable key makes the management API available to
anyone who finds it. The legacy key also acts as an admin bearer token
(`Authorization: Bearer <api_key>`).

## Public URLs and Web UI

The `admin_url` and `doh_url` values are the public URLs users see in the web
UI and Guide. When empty, they are derived from `host` as `https://<host>/`
and `https://<host>/dns-query`. Set them explicitly when behind a reverse proxy
or when the panel lives on a different domain (e.g. `https://panel.example.com/`).

| Key | Default | Purpose |
| --- | --- | --- |
| `admin_url` | `https://<host>/` | Public URL for the admin panel. Shown in status and used for CORS. |
| `doh_url` | `https://<host>/dns-query` | Public URL for DNS-over-HTTPS. Shown in Guide and status. |
| `web_dir` | `./web/dist` | Path to built frontend (`web/dist`). Served as SPA fallback. `bun run build` populates it. Must be `npm`/`bun` built before deployment; Docker does this in the `oven/bun:1-alpine` stage. |
| `admin_password` | `api_key` | Password for `admin` web login. Falls back to `api_key` when empty. Hashed with bcrypt in `kairo.db`. |
| `session_ttl` | `24` | Session lifetime in hours. Cookie is `HttpOnly`, `SameSite=Lax`, `Secure` when TLS/`X-Forwarded-Proto:https` or non-localhost host. Expired sessions are purged hourly. |

The web UI is a single-page app (React 19 + Vite 8 + Tailwind 4, built with `bun`). It
supports light/dark (`system` default) and English/Persian (RTL via `Vazirmatn`). The
backend serves `web/dist` as static files; unknown paths fall back to `index.html`
for client-side routing. If `web_dir` is missing, the old status template is served
at `/`.

Example for a panel on a separate domain:

```yaml
host: dns.example.com
admin_url: https://panel.example.com/
doh_url: https://dns.example.com/dns-query
```

CORS is automatically allowed for origins matching `admin_url`, `doh_url`, and `host`.

## Policy

The `upstream_dns` list names the recursive resolvers used for every query that
is not answered locally. Kairo tries the entries in order and falls back to the
next one when a resolver fails or times out, so two independent resolvers make
the service more resilient.

The `data_dir` path is where runtime state lives. It holds `kairo.db` (users,
sessions, devices, `domain_requests`) plus the three plain text policy files.
The directory is created if it does not exist.

The `ip_source` section controls the automatic allowlist generator. Its
`domains_file` is a plain text file listing the domains whose addresses should
be allowlisted. The generator resolves both A and AAAA records for each domain and
merges the results into the allowlist without removing existing entries. The
`interval` value, in seconds, controls how often the generator runs in the
background. A value of zero disables the background job, and generation can
still be triggered manually with `kairo gen-ips` or `POST /api/generate`.

## Listeners

The `listen` section defines five addresses, each a host and port pair.

| Key | Default | Purpose |
| --- | --- | --- |
| `listen.dns` | `:53` | Plain DNS over UDP and TCP. |
| `listen.dot` | `:853` | DNS over TLS, requires a certificate. |
| `listen.https` | `:443` | The SNI router carrying DoH, the API, and the tunnel. |
| `listen.http` | `127.0.0.1:8080` | Loopback HTTP backend for reverse proxies and local `curl` without TLS. |
| `listen.metrics` | `127.0.0.1:9090` | Prometheus `/metrics` endpoint, loopback-only. |

For local `curl` without TLS use `http://127.0.0.1:8080/api/...`. The SNI router
on `:443`/`:8443` requires TLS with SNI=`host` (`curl -k --resolve host:443:127.0.0.1 https://host/api/...`).

The allowlist decides who is split-routed, not who may query. A restricted
domain resolves to the VPS IP only for allowlisted clients; everyone else gets
the normal upstream answer and simply is not routed. The same policy applies on
plain DNS, DoT, and DoH.

Binding `:53` and `:443` requires root or `cap_net_bind_service`. In Docker
this is handled by host networking; on a bare server run as root or grant the
binary the capability.

## Proxy protocol

When nginx (or another stream proxy) fronts `:443` and forwards unknown SNIs to
Kairo, the tunnel gate would see the proxy's address instead of the client's.
Setting `proxy_protocol` to `true` makes the SNI router accept the client
address from a PROXY protocol v1 header, but only when the direct peer is the
loopback interface.

Point `listen.https` at a loopback-only port, enable `proxy_protocol`, and have
nginx attach the header with `proxy_protocol on;`.

## ACME

The `acme` section enables automatic certificate management through Let's
Encrypt via `http-01`/`github.com/go-acme/lego`.

| Key | Default | Purpose |
| --- | --- | --- |
| `acme.email` | empty | Email used to register the ACME account. Empty disables ACME. |
| `acme.storage` | `<data_dir>/certs` | Where the account key and certificate are kept (persisted under `/data` in Docker). |
| `acme.directory` | Let's Encrypt production | ACME directory URL; set to staging to test. |
| `acme.renew_before_days` | `30` | Renew when the certificate expires within this many days. |
| `acme.http_listen` | `:80` | Address the `http-01` challenge is served on. Must be publicly reachable on port 80. |

ACME and the static `tls.cert`/`tls.key` fallback are mutually exclusive.

## TLS

The `tls.cert` and `tls.key` values point at a certificate chain and its private
key for the `host` name. They are used for the DoT listener and for terminating
TLS on the SNI router so that DoH and the API can be served directly over HTTPS.
They are only consulted when `acme.email` is empty.

When neither ACME nor a TLS section is configured, the SNI router does not
terminate TLS. In that case `host_backend` names a local TLS endpoint, typically
a reverse proxy on `127.0.0.1:8443`, and the router forwards all `host` traffic
to it unchanged.

## TTL and rates

The `ttl` value is the lifetime, in seconds, of the synthesized A records that
Kairo returns for restricted domains. The default of 300 keeps responses
reasonably cached while still letting a domain leave the restricted list take
effect quickly.

The `rate` section limits the global request throughput. The `dns` and
`dns_burst` values apply to the DNS servers and the DoH endpoint together, and
the `api` and `api_burst` values apply to the management API. Requests beyond
the limit receive a `429` response.

## The data directory

The data directory holds the SQLite database `kairo.db` (users, sessions with
`expires_at > now`, devices `ip+ja3_hash`, `connection_logs`, `domain_requests`
`pending/approved/rejected`) plus three plain text files. Each file is read on
startup and on every change, ignoring blank lines and lines that start with `#`.

`domains.txt` lists restricted domains, one per line. Restricting `youtube.com`
also covers `www.youtube.com`. `allowed.txt` lists client IP addresses, one per
line. Loopback addresses are always allowed. `domain.txt` is the input for the IP
generator named by `ip_source.domains_file`. All are written atomically and
watched every 5s for hot-reload; `POST /api/restricted?domain=` etc. write
through the same path and take effect immediately.

## Command line subcommands

| Command | Description |
| --- | --- |
| `kairo run --config path` | Run the server. Config defaults to `config.yaml`. |
| `kairo generate config [dir]` | Write default config and policy files into `dir`, with a fresh random `api_key` and `admin_url`/`doh_url` derived from `dns.example.com`. |
| `kairo migrate [path]` | Add settings missing from an existing config and write it back. Defaults to `config.yaml`. |
| `kairo gen-ips --config path` | Resolve the IP source file into the allowlist and exit. |
| `kairo version` | Print the version and exit. |

## Upgrades

When `migrate` is needed:

```bash
kairo migrate configs/config.yaml
docker run --rm -v "$PWD/configs:/configs" ghcr.io/behdanisohrab/kairo:latest migrate /configs/config.yaml
```

`migrate` only adds what is missing, never overwrites a value you set.
