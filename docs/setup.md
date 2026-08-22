# Kairo setup

This guide walks through a first installation from an empty server to a
working split-routing gateway with the web admin panel. It takes a few minutes
and leaves you with a binary or a container, a configuration file, policy
files, and a way to allow clients and manage domains.

## What you need before starting

A server with a public IPv4 address and a domain name that points to it. The
domain is the address clients will use for DoH and for the management API, so
its DNS record should resolve to the server already. You also need SSH access
and the ability to run commands as root. For Docker, a recent Docker Engine
with the compose plugin covers everything else. For local frontend dev you need
`bun` ≥1.1 (`oven/bun:1-alpine` in Docker, `bun --version` locally).

## Generate a starting configuration

Kairo ships with a generator so you do not have to write the configuration
from scratch. It writes a valid `config.yaml`, a fresh random `api_key`
(64-char hex), and the three policy files into the directory you give it, plus
derived `admin_url`/`doh_url` from `host`.

On a server with the binary installed:

```bash
kairo generate config /opt/kairo
cat /opt/kairo/config.yaml # check host, vps_ip, admin_url, doh_url
```

In Docker, the same command produces the files on a mounted volume:

```bash
mkdir -p configs
docker run --rm -v "$PWD/configs:/configs" ghcr.io/behdanisohrab/kairo:latest generate config /configs
```

The generator prints a summary and the paths it wrote. It never overwrites an
existing `config.yaml`.

## Edit the generated files

Open `config.yaml` and set the values that matter:

- `host` must be your domain name, for example `dns.example.com`.
- `admin_url` and `doh_url` default to `https://<host>/` and `https://<host>/dns-query`. Override if behind a reverse proxy or panel on a separate domain (`https://panel.example.com/`).
- `vps_ip` must be the public IPv4 address of this server.
- `api_key` is already filled with a freshly generated random secret; leave it as is.
- `admin_password` defaults to `api_key` (bcrypt in `kairo.db`); set a separate password if you want.
- `web_dir` points at the built frontend (`./web/dist` locally, `/app/web/dist` in Docker). Run `bun run build` after frontend changes or rely on the Docker `oven/bun:1-alpine` stage that does `bun install --frozen-lockfile && bun run build`.
- The TLS paths can stay if you plan to use the default certificate location, or change them to wherever you store certificates. Alternatively set `acme.email` for automatic Let's Encrypt via `http-01`.

The policy files live in `data` next to the configuration (plus `kairo.db` for
users/sessions/devices/requests). `domains.txt` holds the restricted domains
and `allowed.txt` holds the client IPs you trust. Both accept one entry per
line and ignore blank lines and lines starting with `#`. Start with whatever
domains you actually need routed.

## Build the frontend (if building outside Docker)

The Docker image already builds the UI. For bare-metal or local dev:

```bash
cd web
bun install
bun run build # → web/dist (ignored in git, served as SPA fallback)
bun run dev    # Vite dev at :5173, proxies /api → http://127.0.0.1:8080
```

`bun.lock` and `bunfig.toml` ensure reproducible installs. `web/dist` is served
at `/` with SPA fallback (`index.html`); if `web_dir` is missing the old status
template is served.

## Start the service

With the binary, run it from its working directory so the relative paths in the
configuration resolve:

```bash
cd /opt/kairo
kairo run --config /opt/kairo/config.yaml
```

In Docker, the compose file pulls the image and mounts the configs directory:

```bash
docker compose up -d --build
docker compose logs -f
```

The compose file uses **host networking** on purpose. The DNS servers and the SNI
router must see the real client IPs for the allowlist to work, and they must be
able to bind the privileged ports 53 and 443. A bridge network with published
ports would hide the client addresses behind NAT and break split routing. The
data directory is mounted at `/data` inside the container, so `kairo.db` and
policy files plus any certificate under `<data_dir>/certs` survive rebuilds.

The startup log should show the version, the hostname, `admin_url`/`doh_url`,
and the counts of restricted domains and allowlisted clients.

## Allow your clients

Only allowlisted clients are split-routed. Before a client sees any benefit, its
public IP must be in the allowlist. Add it with the API (via the loopback
backend without TLS, or via the SNI router with TLS and SNI=`host`):

```bash
# loopback (no TLS, easiest locally)
curl -X POST "http://127.0.0.1:8080/api/allow?ip=<client-public-ip>" \
  -H "Authorization: Bearer <your-api-key>"

# SNI router (needs TLS + SNI=host)
curl -k --resolve dns.example.com:8443:127.0.0.1 \
  -X POST "https://dns.example.com:8443/api/allow?ip=<client-public-ip>" \
  -H "Authorization: Bearer <your-api-key>"

# or via session cookie after web login:
# login at https://dns.example.com/login → Dashboard copies Bearer key
```

For several clients, list their addresses in `data/allowed.txt` instead and let
the service reload the file on its own (watched every 5s). If your clients have
stable names, put those names in `data/domain.txt` and either run `kairo gen-ips`
once or set `ip_source.interval` so the allowlist follows them automatically.
Each `POST /api/allow` writes atomically to `allowed.txt` and takes effect
immediately.

The Guide in the web UI (EN/FA, dark/light) also documents whitelisting with
`curl -4 ifconfig.me` and `GET /api/allow` verify.

## Manage domains

Restricted domains are those answered with the VPS IP for allowlisted clients
(and proxied via the SNI router). Add them via the admin panel at
`admin_url` → `Admin → Domains` (single add, bulk paste one per line → `Apply
and reload` hot-reloads `domains.txt` immediately), or via API:

```bash
curl -X POST "http://127.0.0.1:8080/api/restricted?domain=instagram.com" \
  -H "Authorization: Bearer <admin-key>"

# bulk via shell loop
for d in example.com another.org; do
  curl -X POST -H "Authorization: Bearer <admin>" \
    "http://127.0.0.1:8080/api/restricted?domain=$d"
done
```

Users can check a domain in the Dashboard (`Check a domain` → `GET
/api/domain/check?domain=`) and, if not proxied, `POST /api/domain/request`
`{"domain":"example.com"}` to queue a request. Admins approve/reject in
`Admin → Domains` → `User requests` (`POST /api/domain/requests/:id/approve`
adds to `domains.txt` and marks `approved`; `/reject` marks `rejected`).

## Point the clients at Kairo

A client is ready once its IP is allowlisted and its DNS points at the server.
Use the `doh_url` shown in the Guide (`https://dns.example.com/dns-query`,
also `http://127.0.0.1:8080/dns-query` locally and `tls://dns.example.com:853`
for DoT) or plain DNS at `<vps_ip>:53`. The Guide has per-platform steps for
Windows/macOS/Linux/Android/iOS/Firefox/Chrome (bilingual) and `curl` examples
for `application/dns-message` with `Authorization: Bearer <key>`.

## Verify it works

Check a restricted domain and a normal one from an allowlisted client:

```bash
nslookup instagram.com <vps-ip>
nslookup example.com <vps-ip>
# or: dig @<vps-ip> instagram.com +short # → <vps-ip>
# also: curl http://127.0.0.1:8080/api/status -H "Authorization: Bearer <key>" | jq
```

The restricted name should answer with the VPS IP. The normal name should
answer with its real address. If a restricted name still shows the real
address, the client IP is not in the allowlist yet, or the client is not using
this server for DNS. Check `GET /api/allow` and the device list in the dashboard
(`JA3` + `device_type` via `POST /api/me/devices`).

## What is next

The operations document covers running Kairo as a service, obtaining a
certificate (ACME `http-01` on `:80`), and routine maintenance. The security
document explains the limits of the protection, in particular that only TCP is
redirected and that clients which bypass your DNS are not routed at all. The
configuration document details `admin_url`/`doh_url` and the API document lists
all endpoints including `public-config` and domain requests.
