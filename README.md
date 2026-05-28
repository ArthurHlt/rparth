# rparth

A small reverse proxy shipped as a single Go binary for an interview. It does prefix/host based
routing, response streaming, request forwarding headers (RFC 7239), an optional response cache (in-process LRU or
shared Redis), Prometheus metrics, and structured access logs.

Websocket are not supported in this reverse proxy to simplify the implementation.

> [!IMPORTANT]
> For the interviewer
>
> Commit titles follow the Conventional Commits specification, while the commit bodies are intentionally written as a
> development log (with no AI involved).
>
> The goal is to provide visibility into my workflow, technical reasoning, and decision-making process throughout the
> project.
>
> This is not something I would typically do in a real production project, but I believe it makes the project's
> evolution and technical choices easier to follow for the reader.
>
> You can find a generated [tldr-devlog.md](/tldr-devlog.md) generated from the commit messages.
> This is useful to help you scan and jump to the right commit to see my devlog.


> [!NOTE]
> About my use of AI
>
> AI is now an inevitable part of modern software development.
>
> Here is how I currently use it:
>
> 1. I write most of the code myself, while using AI-assisted IDE auto-completion. I only rely more heavily on AI when
     the implementation is straightforward and the requirements are already clearly defined.
>
> 2. I use AI generation mostly to generate documentation, and tests, but I always review, validate, and refine the
     generated output.
>
> 3. I also use AI to review my changes, while keeping the final triage, decisions, and improvements under my own
     responsibility.
>
> In summary, I do not primarily use AI to save time, but rather to reduce repetitive/bothering work and help improve
> software quality or at least my PR quality in real life.

## Table of contents

- [Features](#features)
- [Install](#install)
- [Usage](#usage)
  - [Running with Docker](#running-with-docker)
- [Configuration](#configuration)
  - [Top level](#top-level)
  - [`<route>`](#route)
  - [`<server>`](#server)
  - [`<tls>`](#tls)
  - [`<log>`](#log)
  - [`<cache>`](#cache)
  - [`<lru>`](#lru)
  - [`<redis>`](#redis)
  - [`<transport>`](#transport)
- [Caching behavior](#caching-behavior)
- [Observability](#observability)
  - [The `reason` label](#the-reason-label)
- [Design decisions & trade-offs](#design-decisions--trade-offs)
  - [Proxying & streaming](#proxying--streaming)
  - [Caching](#caching)
  - [Observability](#observability-1)
  - [Operations](#operations)
  - [Tooling & testing](#tooling--testing)
- [Development](#development)
  - [Generating mocks](#generating-mocks)
- [License](#license)

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
default. Mount your config (and TLS files, if any) there.

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
  [- <route> ...]

    # HTTP/HTTPS server settings.
    [server: <server>]

    # Logging settings.
    [log: <log>]

    # Response cache settings. The cache is disabled unless exactly one of `lru` or `redis` is set.
    [cache: <cache>]

    # Upstream HTTP transport tuning.
    [transport: <transport>]
```

### `<route>`

```yaml
# Unique name for the route. Used in access logs and metric labels.
name: <string>

# Upstream URL that matched requests are proxied to, e.g. http://backend:8080.
target: <url>

  # Incoming request Host to match (compared without port). If omitted, the route matches any host.
  [host: <string>]

  # URL path prefix to match.
  [prefix: <string> | default = "/"]

  # Strip the matched `prefix` from the path sent upstream.
  [strip_prefix: <boolean> | default = true]

  # Per-request upstream timeout, in seconds.
  [timeout: <seconds> | default = 30]

  # Disable response caching for this route only.
  [no_cache: <boolean> | default = false]

# Extra headers added to the upstream request. Header names are canonicalized; each value is a list.
headers:
  [<string>: [<string>, ...] ...]
```

### `<server>`

```yaml
# Address to bind, as host:port. Use ":8443" or "0.0.0.0:8443" to accept external traffic.
[listen_addr: <string> | default = ":8080"]

  # Enable HTTPS. When present, both fields below are required.
  [tls: <tls>]
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
[level: <string> | default = "info"]

  # Disable ANSI colors in the human-readable logger.
  [no_color: <boolean> | default = false]

  # Emit JSON logs instead of the human-readable (tint) format.
  [in_json: <boolean> | default = false]
```

### `<cache>`

```yaml
# Maximum cacheable response body size, in bytes. Larger responses are streamed through but not cached.
[max_size_item: <bytes> | default = 1048576]   # 1 MiB

  # In-process LRU cache. Mutually exclusive with `redis`.
  [lru: <lru>]

  # Shared Redis cache. Mutually exclusive with `lru`.
  [redis: <redis>]
```

### `<lru>`

```yaml
# Maximum number of cached entries.
[size: <int> | default = 100]

  # Time-to-live per entry.
  [ttl: <duration> | default = 10m]
```

### `<redis>`

```yaml
# Connection URL, e.g. redis://user:password@host:6379/0 (rediss:// for TLS).
url: <url>

  # Time-to-live per entry.
  [ttl: <duration> | default = 10m]
```

### `<transport>`

```yaml
# Total upstream request timeout (dialing).
[timeout: <duration> | default = 30s]

  # Keep-alive interval for upstream connections.
  [keepalive: <duration> | default = 30s]

  # Maximum number of idle upstream connections across all hosts.
  [max_idle_conns: <int> | default = 1000]

  # Maximum number of idle upstream connections per host.
  [max_idle_conns_per_host: <int> | default = 100]

  # How long an idle upstream connection is kept before closing.
  [idle_conn_timeout: <duration> | default = 30s]

  # How long to wait for the upstream response headers after the request is written.
  [response_header_timeout: <duration> | default = 30s]

  # TLS handshake timeout for upstream connections.
  [tls_handshake_timeout: <duration> | default = 10s]
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

| Metric                                | Type      | Labels                           | Description                                                                            |
|---------------------------------------|-----------|----------------------------------|----------------------------------------------------------------------------------------|
| `rparth_build_info`                   | gauge     | `version`, `commit`, `date`      | Always `1`; the build metadata is carried in the labels.                               |
| `http_requests_total`                 | counter   | `route_name`, `method`, `status` | Requests handled by the full middleware chain, including cache hits.                   |
| `http_request_duration_seconds`       | histogram | `route_name`, `method`, `status` | Request duration measured across the full middleware chain.                            |
| `http_requests_in_flight`             | gauge     | `route_name`                     | Requests currently being handled.                                                      |
| `http_proxy_requests_total`           | counter   | `route_name`, `method`, `status` | Requests forwarded upstream (proxy layer only; excludes cache hits).                   |
| `http_proxy_request_duration_seconds` | histogram | `route_name`, `method`, `status` | Duration of upstream requests only.                                                    |
| `http_proxy_errors_total`             | counter   | `route_name`, `method`, `reason` | Upstream requests that failed before a response was received, by `reason` (see below). |
| `cache_hits_total`                    | counter   | —                                | Number of cache hits.                                                                  |
| `cache_misses_total`                  | counter   | —                                | Number of cache misses.                                                                |
| `cache_lookup_latency_seconds`        | histogram | —                                | Cache lookup latency, observed on hits.                                                |
| `cache_size`                          | gauge     | —                                | Number of items currently in the cache (sampled at scrape time).                       |
| `cache_skip_total`                    | counter   | `route_name`, `reason`           | Responses that were not cached, by `reason` (see below).                               |

The `cache_*` metrics are only emitted when a cache backend is configured.

### The `reason` label

`http_proxy_errors_total` classifies each upstream failure into one of:

| `reason`             | Meaning                                                                                      |
|----------------------|----------------------------------------------------------------------------------------------|
| `timeout`            | The upstream did not respond within the route/transport timeout (includes context deadline). |
| `canceled`           | The client disconnected before the upstream responded (context canceled).                    |
| `dns`                | The upstream host could not be resolved.                                                     |
| `connection_refused` | The upstream refused the TCP connection.                                                     |
| `tls`                | The upstream TLS handshake/certificate verification failed.                                  |
| `connection`         | Other network-level failure (reset, unreachable, broken pipe, …).                            |
| `unknown`            | An error that did not match any of the above.                                                |

`cache_skip_total` records why a cacheable path was not stored:

| `reason`             | Meaning                                                                              |
|----------------------|--------------------------------------------------------------------------------------|
| `disabled`           | The matched route has `no_cache: true`.                                              |
| `method_not_allowed` | The request method is not `GET`.                                                     |
| `cache_control`      | The request or response asked to bypass the cache (`no-store`/`no-cache`/`private`). |
| `too_large`          | The response body exceeded `cache.max_size_item`.                                    |
| `status_code`        | The response status was `< 200` or `>= 400`.                                         |
| `set_cookie`         | The response set a `Set-Cookie` header (per-client).                                 |
| `vary`               | The response carried a `Vary` header.                                                |

## Design decisions & trade-offs

> [!NOTE]
> I would not put this in a real project, this is for the interviewer to see why I've made certain
> choices.
>
> You can see in devlog those decision being made in "live"

The notable choices made during the project and the reasoning behind them.

### Proxying & streaming

- **HTTP/2 is not forced upstream.** It would reduce the number of TCP connections, but it
  complicates the streaming response path this proxy relies on. For this exercise the complexity
  isn't worth the benefit; in a real deployment I'd enable it to serve more requests for the same
  hardware.
- **WebSocket is not supported.** Hop-by-hop headers are sanitized before reaching the backend (as
  they must be), and handling bidirectional streaming would take more time than the scope
  justifies. The current implementation is enough.
- **Responses are streamed, not buffered.** Upstream compression is disabled so the body can be
  forwarded straight through, and each write is flushed (via `http.Flusher` when available) so SSE
  and other long-running requests aren't held in an internal buffer and don't appear to stall.

### Caching

- **LRU with expiration + per-item size cap.** The LRU bounds the entry count and the per-entry
  size limit (default 1 MiB) bounds per-entry memory, so worst-case cache memory stays predictable
  and we avoid unbounded growth.
- **The cache key includes `Authorization`/`Cookie`.** This isolates cached responses per user so
  an authenticated user can never read another user's cached response. The key is hashed with
  xxhash for speed and low collision rate.
- **Per-route `no_cache` flag.** Caching used to be effectively forced; this hands the
  cacheability decision to the operator on a per-route basis.
- **Redis as an optional shared backend.** In a multi-instance deployment, a shared cache avoids
  every instance independently rebuilding and storing the same entries. The Redis client is closed
  cleanly on shutdown; it is intentionally **not** wired through dependency injection because it's
  only needed in one place and DI would add needless complexity.

### Observability

- **Middleware-based observability.** Access logs (`go-chi/httplog`) and Prometheus metrics are
  middlewares in front of the proxy. The `/_metrics` endpoint is itself a middleware that
  short-circuits the chain, which avoids pulling in a router just for one endpoint.
- **Route is carried on the request context.** A dedicated middleware matches the route and puts
  it on the context; everything downstream (access log `route_name`, metric labels, cache key)
  reads it from there instead of re-matching. Using the route *name* as the label (rather than the
  raw path) keeps metric cardinality — and therefore memory — bounded.
- **Upstream errors get their own metric and a `reason`.** Separating proxy-level request/error
  metrics from the full-chain metrics lets you tell a slow or failing backend apart from a slow
  proxy, and classifying failures by reason (timeout, DNS, connection refused, TLS, …) makes the
  cause visible. Upstream failures map to `502`.

### Operations

- **HTTPS is supported and expected.** Running behind TLS is the production default, so it's a
  first-class config option rather than an afterthought.
- **Graceful shutdown with a force escape hatch.** On `SIGINT`/`SIGTERM` the server drains
  in-flight requests; a **second** signal forces an immediate stop (mainly a development
  convenience when you don't want to wait).

### Tooling & testing

- **Kong for the CLI** — flags via struct tags, no hand-rolled argument parsing.
- **`goccy/go-yaml` for config** — the older `go-yaml/yaml` is archived; this library also allows a
  custom unmarshaler for `url.URL`, and config is validated at decode time.
- **Ginkgo + Gomega, with generated mocks.** BDD specs read well and stay behavior-focused;
  interface doubles are generated with `mockgen` (rather than hand-written fakes) and kept with
  loose expectations.

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
