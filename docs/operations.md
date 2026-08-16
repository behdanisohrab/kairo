# Kairo operations

This document describes how to deploy, run, and maintain Kairo in production.
It covers the bare metal setup, the Docker image, DNS configuration for clients,
and the tasks that keep the service healthy.

## Deployment layout

A typical installation puts the binary, the configuration, and the data
directory under `/opt/kairo`. The service needs the listeners on port 53, 443,
and 853, which are privileged ports. On Linux you can run it as root, or grant
the single binary the right capability instead:

```bash
sudo setcap cap_net_bind_service=+ep /opt/kairo/kairo
```

With that capability a regular user can bind the privileged ports without a
superuser service account. Everything else the service writes is confined to the
data directory.

## Running as a systemd service

The following unit file covers the standard deployment. It starts Kairo, keeps
it running across failures, and starts it again after a reboot.

```ini
[Unit]
Description=Kairo split-routing gateway
After=network-online.target
Wants=network-online.target

[Service]
User=kairo
Group=kairo
WorkingDirectory=/opt/kairo
ExecStart=/opt/kairo/kairo -config /opt/kairo/config.json
Restart=always
RestartSec=3
LimitNOFILE=65536

[Install]
WantedBy=multi-user.target
```

Install it as `/etc/systemd/system/kairo.service`, then run
`sudo systemctl daemon-reload` and `sudo systemctl enable --now kairo`. Check the
log with `journalctl -u kairo -f`. The startup log prints the version, the
hostname, and the counts of restricted domains and allowlisted clients, which
confirms the policy files were loaded correctly.

## Running in Docker

The included Dockerfile builds a small static binary, and the compose file
wires everything together. The compose file deliberately uses host networking.
Published ports would hide the client IP address behind NAT, which breaks the
allowlist, and they cannot remap privileged ports to low port numbers in the
container without extra configuration. With host networking the container sees
the real client addresses and binds port 53 directly.

```bash
mkdir -p data
cp config.json data/config.json
# edit data/config.json
docker compose up -d --build
docker compose logs -f
```

The data directory is mounted at `/data` inside the container, so the policy
files and any certificate you place there survive container rebuilds. Set
`data_dir` to `/data` and point the TLS fields at files inside it.

## Obtaining a certificate

DoT and the SNI-based DoH endpoint present a certificate for the `host` name.
Certbot issues one with a standalone challenge while the port is free, or you
can use any other ACME client.

```bash
sudo apt install certbot
sudo certbot certonly --standalone -d dns.example.com
```

The certificate files should be readable by the service account. A renewal hook
keeps the service in sync with the new certificate:

```bash
sudo certbot renew --deploy-hook "systemctl restart kairo"
```

## Configuring clients

A client is ready in two steps. First its public IP must be in the allowlist.
Second its DNS must point at the VPS. The ordering matters because the DNS
server only answers with the VPS IP for allowlisted clients.

```bash
curl "https://dns.example.com/api/allow?key=<key>&ip=<client-public-ip>" -X POST
```

For the DNS side, plain DNS is the simplest and works with any router or
operating system. Set the DNS server of the client network to `<vps_ip>`. DoH
uses `https://dns.example.com/dns-query` and is useful on networks where UDP
port 53 is intercepted. DoT uses `tls://dns.example.com:853` and works with
resolvers that support it. Clients should disable QUIC and HTTP/3 where
possible, since those protocols bypass the SNI router and are not redirected.

## The allowlist generator

If your trusted clients have stable public names, the IP generator keeps the
allowlist up to date without manual API calls. List the names in
`domain.txt`, set `ip_source.interval` in seconds, and Kairo resolves them in
the background. The interval of 300 seconds matches a typical dynamic DNS
update rate. For a one-off refresh, run `kairo -config config.json -gen-ips`
or call `POST /api/generate`. Each run merges new addresses and never removes
existing ones, so a temporarily unresolvable name does not drop a working
client.

## Backups and recovery

Everything that matters is in three files: `config.json`, and the `data`
directory with `allowed.txt`, `domains.txt`, and `domain.txt`. A nightly copy
of these is a complete backup. Restoring them and starting the service brings
the whole routing setup back. The files are written atomically by Kairo, so a
copy taken at any moment is either the old or the new version and never a
partially written one.

## Routine maintenance

Watch the log for repeated upstream failures, which indicate a resolver that
should be replaced in `upstream_dns`. Review the restricted list occasionally
and remove domains that no longer need routing, since every restricted domain
adds DNS answers that differ from the real ones. When the certificate renews,
confirm the DoT endpoint still connects. The health endpoint at `/healthz`
returns a plain success response and is easy to add to a monitoring check or a
load balancer.
