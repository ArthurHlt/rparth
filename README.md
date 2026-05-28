# rparth

A small, fast reverse proxy shipped as a single Go binary (and a container image). It does prefix/host based
routing, response streaming, request forwarding headers (RFC 7239), an optional response cache (in-process LRU or
shared Redis), Prometheus metrics, and structured access logs.

## Features

- **Routing** by host and/or path prefix, first-match-wins, with optional prefix stripping.
- **Forwarding headers** added to upstream requests: `X-Forwarded-For`, `X-Forwarded-Scheme`, `X-Forwarded-Host`,
  and a single-hop `Forwarded` element (RFC 7239).
- **Response caching** (optional): in-process LRU or shared Redis, with `ETag`/`If-None-Match` revalidation and
  per-user isolation.
- **Observability**: Prometheus metrics on `/_metrics` and OTEL-schema access logs.
- **Graceful shutdown** on `SIGINT`/`SIGTERM`; a second signal forces an immediate stop.
- **HTTPS** with a supplied certificate/key.

## Install

**Download a pre-built binary** — grab the latest build for your OS/architecture from the
[GitHub releases page](https://github.com/ArthurHlt/rparth/releases/latest). Each release attaches
ready-to-run binaries and archives (Linux, macOS, Windows) plus checksums; extract the archive and the
`rparth` binary is ready to run.

Other options:

```bash
# From source (Go 1.26+)
go install github.com/ArthurHlt/rparth@latest

# Container image
docker pull ghcr.io/arthurhlt/rparth:latest
```

## Usage

```bash
# Run the proxy (reads ./config.yml by default)
rparth serve -c ./config.yml

# Print version / build info
rparth version
```

### Running with Docker

The image sets no working directory, so the container runs at `/` and `rparth serve` looks for `/config.yml` by
default. Mount your config (and TLS files, if any) there. Make sure `listen_addr` is **not** loopback
(`127.0.0.1`) or the published port is unreachable from the host.

```bash
docker run --rm -p 8443:8443 \
  -v "$PWD/config.yml:/config.yml:ro" \
  -v "$PWD/cert.crt:/cert.crt:ro" \
  -v "$PWD/key.key:/key.key:ro" \
  ghcr.io/arthurhlt/rparth:latest serve
```

## Configuration

Configuration is a single YAML file, validated when it is loaded. A fully populated example lives in
[`config.yml.tpl`](./config.yml.tpl).

Generic placeholders are defined as follows:

- `<boolean>`: a boolean, either `true` or `false`
- `<int>`: an integer
- `<bytes>`: an integer number of bytes
- `<seconds>`: an integer number of seconds
- `<duration>`: a Go duration string such as `30s`, `10m`, `1h` (parsed by `time.ParseDuration`)
- `<string>`: a regular string
- `<filename>`: a path to a file on the host running rparth
- `<url>`: an absolute URL

Notation: a field wrapped in `[ ]` is optional. Required fields have no brackets. Defaults are shown as
`| default = …`.

### Top level

```yaml
# A list of routes, evaluated top-to-bottom; the first one that matches wins.
# At least one route is required.
routes:
  [ - <route> ... ]

# HTTP/HTTPS server settings.
[ server: <server> ]

# Logging settings.
[ log: <log> ]

# Response cache settings. The cache is disabled unless exactly one of `lru` or `redis` is set.
[ cache: <cache> ]

# Upstream HTTP transport tuning.
[ transport: <transport> ]
```

### `<route>`

```yaml
# Unique name for the route. Used in access logs and metric labels.
name: <string>

# Upstream URL that matched requests are proxied to, e.g. http://backend:8080.
target: <url>

# Incoming request Host to match (compared without port). If omitted, the route matches any host.
[ host: <string> ]

# URL path prefix to match.
[ prefix: <string> | default = "/" ]

# Strip the matched `prefix` from the path sent upstream.
[ strip_prefix: <boolean> | default = true ]

# Per-request upstream timeout, in seconds.
[ timeout: <seconds> | default = 30 ]

# Disable response caching for this route only.
[ no_cache: <boolean> | default = false ]

# Extra headers added to the upstream request. Header names are canonicalized; each value is a list.
headers:
  [ <string>: [ <string>, ... ] ... ]
```

### `<server>`

```yaml
# Address to bind, as host:port. Use ":8443" or "0.0.0.0:8443" to accept external traffic.
[ listen_addr: <string> | default = ":8080" ]

# Enable HTTPS. When present, both fields below are required.
[ tls: <tls> ]
```

### `<tls>`

```yaml
# PEM certificate and private key. Paths are resolved relative to the process working directory.
cert_file: <filename>
key_file: <filename>
```

### `<log>`

```yaml
# Minimum log level: one of debug | info | warn | error.
[ level: <string> | default = "info" ]

# Disable ANSI colors in the human-readable logger.
[ no_color: <boolean> | default = false ]

# Emit JSON logs instead of the human-readable (tint) format.
[ in_json: <boolean> | default = false ]
```

### `<cache>`

```yaml
# Maximum cacheable response body size, in bytes. Larger responses are streamed through but not cached.
[ max_size_item: <bytes> | default = 1048576 ]   # 1 MiB

# In-process LRU cache. Mutually exclusive with `redis`.
[ lru: <lru> ]

# Shared Redis cache. Mutually exclusive with `lru`.
[ redis: <redis> ]
```

### `<lru>`

```yaml
# Maximum number of cached entries.
[ size: <int> | default = 100 ]

# Time-to-live per entry.
[ ttl: <duration> | default = 10m ]
```

### `<redis>`

```yaml
# Connection URL, e.g. redis://user:password@host:6379/0 (rediss:// for TLS).
url: <url>

# Time-to-live per entry.
[ ttl: <duration> | default = 10m ]
```

### `<transport>`

```yaml
# Total upstream request timeout (dialing).
[ timeout: <duration> | default = 30s ]

# Keep-alive interval for upstream connections.
[ keepalive: <duration> | default = 30s ]

# Maximum number of idle upstream connections across all hosts.
[ max_idle_conns: <int> | default = 1000 ]

# Maximum number of idle upstream connections per host.
[ max_idle_conns_per_host: <int> | default = 100 ]

# How long an idle upstream connection is kept before closing.
[ idle_conn_timeout: <duration> | default = 30s ]

# How long to wait for the upstream response headers after the request is written.
[ response_header_timeout: <duration> | default = 30s ]

# TLS handshake timeout for upstream connections.
[ tls_handshake_timeout: <duration> | default = 10s ]
```

## Caching behavior

When a cache backend is configured, responses are cached subject to these rules:

- **Requests**: only `GET` is cached, and the request is bypassed if it carries `Cache-Control: no-store` or
  `no-cache`. Routes with `no_cache: true` are never cached.
- **Responses** are *not* cached when they set `Set-Cookie` or `Vary`, when `Cache-Control` lists
  `no-store`/`no-cache`/`private`, when the status is `< 200` or `>= 400`, or when the body exceeds
  `cache.max_size_item`.
- The cache **key** includes the method, URL, route name, and the `Authorization`/`Cookie` headers, so
  authenticated users never read each other's cached responses.
- Every cacheable response gets an `ETag`; a matching `If-None-Match` yields `304 Not Modified`, and responses
  served from cache carry `X-Cache: HIT`.

## Observability

A Prometheus exposition is served on `GET /_metrics`. Access logs are emitted per request using the OTEL log
schema, labeled with the matched `route_name`.

| Metric | Type | Labels | Description |
| --- | --- | --- | --- |
| `rparth_build_info` | gauge | `version`, `commit`, `date` | Always `1`; the build metadata is carried in the labels. |
| `http_requests_total` | counter | `route_name`, `method`, `status` | Requests handled by the full middleware chain, including cache hits. |
| `http_request_duration_seconds` | histogram | `route_name`, `method`, `status` | Request duration measured across the full middleware chain. |
| `http_requests_in_flight` | gauge | `route_name` | Requests currently being handled. |
| `http_proxy_requests_total` | counter | `route_name`, `method`, `status` | Requests forwarded upstream (proxy layer only; excludes cache hits). |
| `http_proxy_request_duration_seconds` | histogram | `route_name`, `method`, `status` | Duration of upstream requests only. |
| `http_proxy_errors_total` | counter | `route_name`, `method`, `reason` | Upstream requests that failed before a response was received, by `reason` (see below). |
| `cache_hits_total` | counter | — | Number of cache hits. |
| `cache_misses_total` | counter | — | Number of cache misses. |
| `cache_lookup_latency_seconds` | histogram | — | Cache lookup latency, observed on hits. |
| `cache_size` | gauge | — | Number of items currently in the cache (sampled at scrape time). |
| `cache_skip_total` | counter | `route_name`, `reason` | Responses that were not cached, by `reason` (see below). |

The `cache_*` metrics are only emitted when a cache backend is configured.

### The `reason` label

`http_proxy_errors_total` classifies each upstream failure into one of:

| `reason` | Meaning |
| --- | --- |
| `timeout` | The upstream did not respond within the route/transport timeout (includes context deadline). |
| `canceled` | The client disconnected before the upstream responded (context canceled). |
| `dns` | The upstream host could not be resolved. |
| `connection_refused` | The upstream refused the TCP connection. |
| `tls` | The upstream TLS handshake/certificate verification failed. |
| `connection` | Other network-level failure (reset, unreachable, broken pipe, …). |
| `unknown` | An error that did not match any of the above. |

`cache_skip_total` records why a cacheable path was not stored:

| `reason` | Meaning |
| --- | --- |
| `disabled` | The matched route has `no_cache: true`. |
| `method_not_allowed` | The request method is not `GET`. |
| `cache_control` | The request or response asked to bypass the cache (`no-store`/`no-cache`/`private`). |
| `too_large` | The response body exceeded `cache.max_size_item`. |
| `status_code` | The response status was `< 200` or `>= 400`. |
| `set_cookie` | The response set a `Set-Cookie` header (per-client). |
| `vary` | The response carried a `Vary` header. |

## Development

Requires Go 1.26+. Tests use [Ginkgo v2](https://onsi.github.io/ginkgo/) + Gomega.

```bash
# Build
go build ./...

# Run the full test suite (randomized, race-enabled, parallel)
ginkgo --randomize-all --randomize-suites --race -p ./...

# Lint (config-driven; do not substitute bare `go vet`)
golangci-lint run ./...

# Format
gofmt -w .
```

### Generating mocks

Test doubles for interfaces are generated with [`go.uber.org/mock`](https://github.com/uber-go/mock). For example,
the `caches.Cache` interface carries a `//go:generate mockgen` directive that produces `caches/mocks/mock_cache.go`.
Install the generator once, then regenerate after changing any mocked interface:

```bash
# One-time: install the mockgen binary
go install go.uber.org/mock/mockgen@latest

# Regenerate every //go:generate directive in the module
go generate ./...
```

Generated mocks are committed, so a plain `go test` / `ginkgo` run does not require `mockgen` to be installed — only
regenerating them does.

## License

See [LICENSE](./LICENSE).
