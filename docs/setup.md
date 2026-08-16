# Kairo setup

This guide walks through a first installation from an empty server to a
working split-routing gateway. It takes a few minutes and leaves you with a
binary or a container, a configuration file, and a way to allow clients.

## What you need before starting

A server with a public IPv4 address and a domain name that points to it. The
domain is the address clients will use for DoH and for the management API, so
its DNS record should resolve to the server already. You also need SSH access
to the server and the ability to run commands as root. If you use Docker, a
recent Docker Engine with the compose plugin covers everything else.

## Generate a starting configuration

Kairo ships with a generator so you do not have to write the configuration
from scratch. It writes a valid `config.json` plus the three policy files into
the directory you give it.

On a server with the binary installed:

```bash
kairo --generate config /opt/kairo
```

In Docker, the same command produces the files on a mounted volume:

```bash
mkdir -p configs
docker run --rm -it -v "$PWD/configs:/configs" kairo --generate config /configs
```

The generator prints a summary and the paths it wrote. It only creates files;
it never overwrites an existing `config.json`, so running it again is safe if
you want to see what a default looks like.

## Edit the generated files

The generated `config.json` has placeholders. Open it and set the four values
that matter:

- `host` must be your domain name, for example `dns.example.com`.
- `vps_ip` must be the public IPv4 address of this server.
- `api_key` must be replaced with a long random string. Generate one with
  `openssl rand -hex 32` and paste it in.
- The TLS paths can stay if you plan to use the default certificate location,
  or change them to wherever you store certificates.

The policy files live in `data` next to the configuration. `domains.txt` holds
the restricted domains and `allowed.txt` holds the client IPs you trust. Both
accept one entry per line and ignore blank lines and lines starting with `#`.
Start with whatever domains you actually need routed, since every entry changes
DNS answers for your whole allowlist.

## Start the service

With the binary, run it from its working directory so the relative paths in the
configuration resolve:

```bash
cd /opt/kairo
kairo -config config.json
```

In Docker, the compose file handles the build and the volume mount:

```bash
docker compose up -d --build
```

The compose file uses host networking on purpose. The DNS servers and the SNI
router must see the real client IPs for the allowlist to work, and they must be
able to bind the privileged ports 53 and 443. A bridge network with published
ports would hide the client addresses behind NAT and break split routing.

Either way, the startup log should show the version, the hostname, and the
counts of restricted domains and allowlisted clients. That output is the first
sign that the policy files loaded correctly.

## Allow your clients

Only allowlisted clients are split-routed. Before a client sees any benefit,
its public IP must be in the allowlist. Add it with the API:

```bash
curl "https://dns.example.com/api/allow?key=<your-api-key>&ip=<client-public-ip>" -X POST
```

For several clients, list their addresses in `data/allowed.txt` instead and let
the service reload the file on its own. If your clients have stable names
rather than fixed addresses, put those names in `data/domain.txt` and either
run `kairo -gen-ips` once or set `ip_source.interval` so the allowlist follows
them automatically.

## Point the clients at Kairo

A client is ready once its IP is allowlisted and its DNS points at the server.
On a router or operating system that lets you set a custom DNS server, use the
VPS IP for plain DNS. On clients that support it, DoH at
`https://dns.example.com/dns-query` and DoT at `tls://dns.example.com:853`
work over encrypted connections.

## Verify it works

Check a restricted domain and a normal one from an allowlisted client:

```bash
nslookup instagram.com <vps-ip>
nslookup example.com <vps-ip>
```

The restricted name should answer with the VPS IP. The normal name should
answer with its real address. If a restricted name still shows the real
address, the client IP is not in the allowlist yet, or the client is not using
this server for DNS.

## What is next

The operations document covers running Kairo as a service, obtaining a
certificate, and routine maintenance. The security document explains the limits
of the protection, in particular that only TCP is redirected and that clients
which bypass your DNS are not routed at all.
