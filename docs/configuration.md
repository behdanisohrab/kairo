# Kairo configuration

Kairo reads a single YAML file on startup, selected with the `--config` flag of
the `run` subcommand (default `config.yaml`). Every field has a default, so the
file only needs to set the values that differ. This document explains each
field, the file layout, and the command line subcommands.

## The configuration file

The file is split into logical sections. The identity section describes the
public face of the service. The policy section names the upstream resolver and
the files used for the allowlist and the restricted list. The listener section
defines the network ports. The rate section controls throughput limits.

```yaml
host: dns.example.com
host_backend: ""
vps_ip: 1.2.3.4
api_key: REPLACE_WITH_A_STRONG_SECRET
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

The `api_key` value is the shared secret that guards every `/api/*` request.
Choose a long random string and keep it out of version control. A short or
guessable key makes the management API available to anyone who finds it.

## Policy

The `upstream_dns` list names the recursive resolvers used for every query that
is not answered locally. Kairo tries the entries in order and falls back to the
next one when a resolver fails or times out, so two independent resolvers make
the service more resilient.

The `data_dir` path is where the runtime state files live. All paths inside it
can be relative to the directory where Kairo was started. The directory is
created if it does not exist, and its three files are described below.

The `ip_source` section controls the automatic allowlist generator. Its
`domains_file` is a plain text file listing the domains whose addresses should
be allowlisted, for example the public address of a home router or a trusted
workstation. The generator resolves both A and AAAA records for each domain and
merges the results into the allowlist without removing existing entries. The
`interval` value, in seconds, controls how often the generator runs in the
background. A value of zero disables the background job, and generation can
still be triggered manually with `kairo gen-ips` or the API endpoint.

## Listeners

The `listen` section defines five addresses, each a host and port pair.

| Key | Default | Purpose |
| --- | --- | --- |
| `listen.dns` | `:53` | Plain DNS over UDP and TCP. |
| `listen.dot` | `:853` | DNS over TLS, requires a certificate. |
| `listen.https` | `:443` | The SNI router carrying DoH, the API, and the tunnel. |
| `listen.http` | `127.0.0.1:8080` | Loopback HTTP backend for reverse proxies. |
| `listen.metrics` | `127.0.0.1:9090` | Prometheus `/metrics` endpoint, loopback-only. |

The allowlist decides who is split-routed, not who may query. A restricted
domain resolves to the VPS IP only for allowlisted clients; everyone else gets
the normal upstream answer and simply is not routed. The same policy applies on
plain DNS, DoT, and DoH.

Binding `:53` and `:443` requires root or the relevant capabilities. In Docker
this is handled by host networking; on a bare server you either run as root or
grant the binary the `NET_BIND_SERVICE` capability.

## Proxy protocol

When nginx (or another stream proxy) fronts `:443` and forwards unknown SNIs to
Kairo, the tunnel gate would see the proxy's address instead of the client's.
Setting `proxy_protocol` to `true` makes the SNI router accept the client
address from a PROXY protocol v1 header, but only when the direct peer is the
loopback interface. This keeps the gate meaningful: without it, any client
could add the VPS IP to its hosts file and reach the tunnel through nginx, and
the allowlist would not see the real client.

Point `listen.https` at a loopback-only port, enable `proxy_protocol`, and have
nginx attach the header with `proxy_protocol on;` on the public listener, which
carries the real client address because that listener's peer is the client
itself. Backends that do not want the header need an internal strip hop. Never
enable this on a listener reachable by untrusted peers, since a remote client
could forge the header. See `nginx.conf.example` for a working setup. Do not
route the tunnel through a second nginx hop that re-emits the header: nginx
only re-emits the address it parsed from a PROXY header if the stream realip
module rewrites `$remote_addr`, otherwise the header carries the hop's own
loopback address and every client looks allowlisted.

## ACME

The `acme` section enables automatic certificate management through Let's
Encrypt using the `http-01` challenge, driven by
[github.com/go-acme/lego](https://github.com/go-acme/lego).

| Key | Default | Purpose |
| --- | --- | --- |
| `acme.email` | empty | Email used to register the ACME account. Empty disables ACME. |
| `acme.storage` | `<data_dir>/certs` | Where the account key and certificate are kept (persisted under `/data` in Docker). |
| `acme.directory` | Let's Encrypt production | ACME directory URL; set to a staging directory to test. |
| `acme.renew_before_days` | `30` | Renew when the certificate expires within this many days. |
| `acme.http_listen` | `:80` | Address the `http-01` challenge is served on. Must be publicly reachable on port 80. |

When `acme.email` is set, Kairo registers an account, obtains a certificate for
`host` on first run, and renews it automatically. Because the challenge is
served on port 80, the VPS must accept connections on `:80`, and `host` must
resolve to `vps_ip` from the public internet. The account key and certificate
are stored under `acme.storage` so they survive restarts without re-issuing.

ACME and the static `tls.cert`/`tls.key` fallback are mutually exclusive:
configure one or the other, never both. If both are set, Kairo refuses to
start with an explanatory error.

## TLS

The `tls.cert` and `tls.key` values point at a certificate chain and its private
key, normally issued for the `host` name. They are used for the DoT listener and
for terminating TLS on the SNI router so that DoH and the API can be served
directly over HTTPS. They are only consulted when `acme.email` is empty.

When neither ACME nor a TLS section is configured, the SNI router does not
terminate TLS. In that case `host_backend` names a local TLS endpoint, typically
a reverse proxy on `127.0.0.1:8443`, and the router forwards all `host` traffic
to it unchanged. This keeps DoH and the API reachable at the public hostname
while the proxy handles the certificates.

## TTL and rates

The `ttl` value is the lifetime, in seconds, of the synthesized A records that
Kairo returns for restricted domains. The default of 300 keeps responses
reasonably cached while still letting a domain leave the restricted list take
effect quickly.

The `rate` section limits the global request throughput. The `dns` and
`dns_burst` values apply to the DNS servers and the DoH endpoint together, and
the `api` and `api_burst` values apply to the management API. The first value is
the sustained rate per second and the second is the burst allowed at once.
Requests beyond the limit are answered with a rate limit error.

## The data directory

The data directory holds three plain text files. Each file is read on startup
and on every change, ignoring blank lines and lines that start with `#`. The
files are managed by the API and by the generator, and they can also be edited
by hand while the service is running.

`domains.txt` lists restricted domains, one per line. Restricting `youtube.com`
also covers `www.youtube.com` and every deeper subdomain. `allowed.txt` lists
client IP addresses, one per line. Loopback addresses are always treated as
allowed and never need to be listed. `domain.txt` is the input for the IP
generator named by `ip_source.domains_file`.

## Command line subcommands

Kairo's CLI is a set of subcommands, each with its own options.

| Command | Description |
| --- | --- |
| `kairo run --config path` | Run the server. Config defaults to `config.yaml`. |
| `kairo generate config [dir]` | Write default config and policy files into `dir`, with a fresh random `api_key`. |
| `kairo migrate [path]` | Add settings missing from an existing config and write it back. Defaults to `config.yaml`. |
| `kairo gen-ips --config path` | Resolve the IP source file into the allowlist and exit. |
| `kairo version` | Print the version and exit. |

The `gen-ips` subcommand is useful in a cron job or during first setup. It
performs a single generation pass and exits, which makes the result visible in
the log and in `allowed.txt` without starting the full service.

## Upgrades

New releases occasionally add configuration options, like `proxy_protocol`.
Kairo accepts a config that is missing them, filling in the defaults, but the
file itself stays as you wrote it. Run `migrate` after an upgrade to bring the
file up to the current schema:

```bash
kairo migrate configs/config.yaml
docker run --rm -it -v "$PWD/configs:/configs" ghcr.io/behdanisohrab/kairo:latest migrate /configs/config.yaml
```

`migrate` only adds what is missing, never overwrites a value you set, and
keeps unknown keys. It prints the settings it added and is a no-op when the
config is already up to date.
