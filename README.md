# Kairo

Kairo is a self-contained implementation of the split-routing gateway described
by [bepass-org/smartSNI](https://github.com/bepass-org/smartSNI). It bundles a
DNS server with a policy engine and an SNI router into one binary. A client
whose IP address is allowlisted receives the VPS IP as the answer for any
restricted domain, so its connection lands on the SNI router, which forwards it
to the real destination while the TLS session stays end-to-end encrypted. Every
other domain resolves normally and never touches the VPS.

- Plain DNS on port 53, plus DNS over HTTPS and DNS over TLS.
- An SNI router on port 443 that transparently proxies allowlisted clients.
- A key-authenticated API to manage the allowlist and the restricted domain
  list, with every change persisted to disk.
- An IP generator that resolves a plain list of client domains into
  allowlisted addresses, either periodically or on demand.
- Hot reload of the policy files, so edits are picked up without restarts.
- Reverse-proxy friendly, Docker ready, and licensed under BSD-3-Clause.

## How it works

```
 client (public IP)                 VPS (this binary)
   │  DNS query :53 / 853 / DoH         │
   │  ─────────────────────────────────▶│  policy engine
   │  restricted + allowed → A = vps_ip │
   │  otherwise            → upstream   │
   │                                    │
   │  TLS to restricted site            │
   │  ─────────────────────────────────▶│  SNI router :443
   │                                    │  SNI == host   → DoH + API
   │                                    │  SNI == site   → tunnel to site
   │                                    │  non-allowed   → rejected
```

## Quick start (Docker)

```bash
mkdir -p configs
docker run --rm -it -v "$PWD/configs:/configs" ghcr.io/behdanisohrab/kairo:latest generate config /configs
# edit configs/config.yaml: host, vps_ip, tls paths
docker compose up -d
```

Images are published to the GitHub Container Registry on every release
(`ghcr.io/behdanisohrab/kairo:latest` and the tagged version).

The `generate config` command writes a complete starting config and the policy
files into `configs/`, generating a fresh random `api_key` for you. The compose
file mounts that same `./configs` directory at `/data` in the container, where
the image expects `config.yaml` to live, so no copying is needed. The bare
binary uses the same command via `kairo generate config <dir>`. A step by step
walkthrough lives in [docs/setup.md](docs/setup.md).

When you upgrade to a newer image, run `migrate` once against your config to add
any settings the new version introduced:

```bash
docker run --rm -it -v "$PWD/configs:/configs" ghcr.io/behdanisohrab/kairo:latest migrate /configs/config.yaml
```

It only adds what is missing, never touches your values, and is a no-op when the
config is already current.

The compose file uses host networking so the listeners see the real client IPs,
which the allowlist depends on, and so they can bind the privileged ports.

## Manual build

```bash
go build -ldflags="-X kairo/internal/version.Version=0.2.0" -o kairo ./cmd/kairo
./kairo generate config configs
./kairo run --config configs/config.yaml
```

### systemd

```ini
[Unit]
Description=Kairo split-routing gateway
After=network-online.target

[Service]
User=root
WorkingDirectory=/opt/kairo
ExecStart=/opt/kairo/kairo run --config /opt/kairo/config.yaml
Restart=always
RestartSec=3

[Install]
WantedBy=multi-user.target
```

## Configuration

The full reference lives in [docs/configuration.md](docs/configuration.md). The
essential fields are:

| Field | Description |
| --- | --- |
| `host` | Public hostname used for DoH and the API, matched via SNI. |
| `vps_ip` | Public IP of this server, returned for restricted domains. |
| `api_key` | Shared secret required by every `/api/*` call. |
| `upstream_dns` | Recursive resolvers for non-split queries. |
| `data_dir` | Directory holding the policy files `allowed.txt` and `domains.txt`. |
| `ip_source` | Optional generator that turns a `domain.txt` file into allowlisted IPs. |

The policy files are plain text, one entry per line, and can be edited by hand.
Subdomains of a restricted domain are covered automatically.

## Client setup

1. Allow your client's public IP, once:
   ```bash
   curl "https://<host>/api/allow?key=<key>&ip=<your-public-ip>" -X POST
   ```
2. Point DNS at the VPS: plain DNS on `<vps_ip>:53`, DoH at
   `https://<host>/dns-query`, or DoT at `tls://<host>:853`.

Restricted domains now route through the VPS; everything else keeps using the
normal connection.

## API reference

Authenticate with the shared key as `?key=...`, an `X-API-Key` header, or a
Bearer token. See [docs/api.md](docs/api.md) for details.

| Method | Path | Description |
| --- | --- | --- |
| `POST` | `/api/allow?ip=1.2.3.4` | Allow a client IP. |
| `DELETE` | `/api/allow?ip=1.2.3.4` | Remove a client IP. |
| `GET` | `/api/allow` | List allowed client IPs. |
| `POST` | `/api/restricted?domain=example.com` | Add a restricted domain. |
| `DELETE` | `/api/restricted?domain=example.com` | Remove a restricted domain. |
| `GET` | `/api/restricted` | List restricted domains. |
| `POST` | `/api/generate` | Rebuild the allowlist from the IP source file. |
| `GET` | `/api/status` | Full status snapshot. |

Other endpoints are `GET /` for the status page and `GET /healthz` for health
checks.

## TLS certificates

DoT and the SNI-based DoH and API endpoints need a certificate for `host`. Kairo
can manage it automatically with [lego](https://github.com/go-acme/lego) using
the `http-01` challenge. Set `acme.email` in the config and it will:

1. Register an ACME account with your email.
2. Obtain a certificate for `host` on first run (the `http-01` challenge is
   served on port 80, so make sure `:80` is open and forwarded to the VPS, and
   that `host` resolves to `vps_ip`).
3. Renew it automatically before it expires (`renew_before_days`, default 30).

The account key and certificate are stored under `<data_dir>/certs` (inside the
`/data` volume in Docker), so they persist across restarts.

If you prefer a hand-managed certificate, leave `acme.email` empty and point
`tls.cert`/`tls.key` at the files instead, e.g. from certbot. Configuring both
`acme.email` and `tls.cert`/`tls.key` is an error and Kairo refuses to start.

## Documentation

- [Overview](docs/overview.md) for the concepts and architecture.
- [Setup](docs/setup.md) for a first installation from scratch.
- [Configuration](docs/configuration.md) for every setting, explained.
- [Operations](docs/operations.md) for deployment, systemd, Docker, and TLS.
- [API reference](docs/api.md) for endpoints, authentication, and examples.
- [Security model](docs/security.md) for the threat model and its limits.

## License

BSD 3-Clause. See [LICENSE](LICENSE). This project is an implementation of the
concepts published by the bepass-org/smartSNI project and credits it as its
inspiration.
