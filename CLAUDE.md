# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project

A reverse proxy (`github.com/ArthurHlt/rparth`, Go 1.26) shipped as a single binary and a container image on ghcr.
`main.go` is a Kong CLI with two commands: `serve` (reads `./config.yml` by default, runs the proxy) and `version`.
Configuration is YAML and validated at decode time.

## Commands

```bash
# Build everything
go build ./...

# Run all tests
ginkgo --randomize-all --randomize-suites --race -p ./...

# Run a single package's suite
ginkgo --randomize-all --randomize-suites --race -p ./proxy
ginkgo --randomize-all --randomize-suites --race -p ./middlewares

# Run a single Ginkgo spec by description
ginkgo --randomize-all --randomize-suites --race -p ./... --focus="<spec description regex>"

# Lint (config-driven; runs vet plus the rest — use this, not bare `go vet`)
golangci-lint run ./...

# Format
gofmt -w .

# Run the binary against the checked-in config.yml (serves HTTPS on 127.0.0.1:8443 with cert.crt/key.key)
go run . serve -c ./config.yml
```

Always run `golangci-lint run ./...` before considering work complete; do not substitute `go vet` alone.

Tests use **Ginkgo v2 + Gomega**. Each package has a `*_suite_test.go` bootstrap that calls `RunSpecs`; `Describe`/`It`
specs live in sibling `*_test.go` files inside the same `_test` package, so they exercise the public API only.

## Architecture

The request lifecycle, top-down:

1. **`main.go`** parses CLI flags with Kong, loads config, and calls `app.NewApp(cnf).RunServer(stopCtx, forceCtx)`.
   `stopCtx` cancels on SIGINT/SIGTERM and triggers graceful shutdown (1 min timeout); a **second** signal cancels
   `forceCtx` and aborts the shutdown wait. Don't conflate them — they're two separate contexts on purpose.
2. **`app`** wires everything: logger (tint, or JSON when `log.in_json: true`), middleware chain, HTTP/HTTPS server,
   listener. `NewApp` builds the full middleware stack; `NewAppBare` returns the App with no middlewares for tests.
   `App.httpServerBuilder` is a seam: it returns a `*http.Server` *and* a pre-bound `net.Listener` so tests can inject
   an httptest-style listener instead of racing on a free port. `SetServerBuilder` / `SetMiddlewares` are the test hooks.
3. **`config`** decodes YAML via `goccy/go-yaml` with `yaml.UseJSONUnmarshaler()` — this is how `slog.Level` parses from
   strings like `"debug"` (slog's `UnmarshalJSON` does the work). Each config struct has its own `UnmarshalYAML` that
   fills defaults: server listen `:8080`, LRU size `100` / ttl `10m`, cache `max_size_item` 1 MiB. `RPRoutes` registers
   a custom `*url.URL` unmarshaler so `target:` can be a plain string.
4. **Middleware chain** (`middlewares.Chain` — first arg is outermost, runs first on the way in):
   `proxy.MarkRPRouteRequest` → `AccessLog` → `Prometheus` (`/_metrics`) → `MetricsHttp` → `Cache` (only if configured)
   → `Proxy` handler. **`MarkRPRouteRequest` must stay first** because it puts the matched `*RPRoute` on the request
   context; everything downstream (access log `route_name`, prom labels, cache key) reads it via
   `contexts.GetRPRoute`. Unmatched requests get no route in context, and downstream code defaults the label to
   `"unknown"` — `contexts.SetRPRoute` returns the request unchanged when the route is nil, so don't add a "nil route"
   sentinel.

### Proxy package

`proxy.Proxy` is constructed with a `RoundTripper` + `RPRoutes` and implements `http.Handler`. `ServeHTTP` reads the
matched route from the request context (set by `MarkRPRouteRequest`, *not* re-matched here); if there is no route it
returns `404 Not Found` without touching the transport. Otherwise it times the call, invokes `roundTrip(route, req)`,
and records the proxy-level metrics (`http_proxy_requests_total` + duration on success, `http_proxy_errors_total` +
duration labelled `502` on failure). `roundTrip` takes the route as an argument, clones the request, rewrites
`URL.Host`/`URL.Scheme` *and* `req.Host` (the field) to the route's `Target` — both are required, since
`http.Transport` derives the wire-level `Host:` header from `req.Host`, not from `req.Header["Host"]`. It then
sanitizes hop-by-hop headers on the request, injects per-route headers, then injects proxy-set forwarding headers
(see below), forwards via the transport, and sanitizes hop-by-hop headers on the response. Any error from `roundTrip`
is an upstream failure and maps to `502 Bad Gateway`. If `io.Copy` from the upstream body fails mid-stream,
`ServeHTTP` only logs and returns — it does **not** call `http.Error`, because the response has already been
committed. (`models.ErrNoRoute` is still returned by `RPRoutes.FindRoute`, but `MarkRPRouteRequest` discards it and
nothing maps it to a status anymore.)

`DefaultProxyTransport` is a tuned `http.Transport` — bigger idle pool, shorter idle timeout, **compression disabled**
so the response body can be streamed straight to the client.

#### Hop-by-hop header handling

`proxy.go` keeps two parallel structures (`hopByHopHeadersMap` for O(1) membership checks, `hopByHopHeaders` slice for
iteration, populated from the map in `init`). Per RFC 7230, the `Connection` header can list additional
connection-scoped headers — `sanitizeHopByHopHeaders` strips both the canonical set and anything named in `Connection`.
Preserve this dual-structure pattern when adding hop-by-hop headers.

#### Proxy-set forwarding headers

`proxyHeaders` adds these headers to the **upstream request** (not the response sent to the client):

- `X-Forwarded-For` — chain built from any incoming `X-Forwarded-For` (split on `,` with each element trimmed) plus
  the client IP parsed from `req.RemoteAddr` (via `net.SplitHostPort`; falls back to the raw value if there's no port).
- `X-Forwarded-Scheme` — `https` when `req.TLS != nil`, otherwise `http`.
- `X-Forwarded-Host` — taken from `req.Host` (the field, not the header).
- `Forwarded` (RFC 7239) — single element describing **this** hop only:
  `by=rparth;for=<ip>;host=<host>;proto=<scheme>`. The upstream chain is conveyed via `X-Forwarded-For`, not duplicated
  here (each proxy adds exactly one Forwarded element per RFC 7239). IPv6 client IPs are bracket-quoted (`for="[...]"`).
  The `by=` token is the package-level `byName` constant.

These are applied *after* hop-by-hop sanitization and *after* per-route headers, so route headers cannot override the
forwarding chain.

#### HTTP/2 decision

Per `journey.md.crap`, HTTP/2 was deliberately **not** forced upstream: it would reduce TCP connections but complicates
response streaming (which this proxy relies on for body forwarding). If you reintroduce HTTP/2, you must rework the
streaming response path accordingly.

### Models package

`RPRoute` is YAML-tagged (`yaml:"..."`). `RPRoutes.FindRoute` walks routes in order and returns the first that `Match`es
on host (optional) + path prefix. `Validate` defaults `Prefix` to `/`, `Timeout` to 30s, and `StripPrefix` to `true`,
canonicalizes any `Headers` keys via `textproto.CanonicalMIMEHeaderKey`, and rejects duplicate route names. **Note**:
`Match` reads the request host from `req.Host` (the field) and runs it through `net.SplitHostPort`, falling back to the
raw value when there's no port — so a bare host like `api.example.com` does match a route with `Host:
"api.example.com"`. Tests set `req.Host` directly.

### Caching

Two-layer system: a `caches.Cache` interface (`Get`/`Set`/`Contains`) with `LRUExpirable` (hashicorp/golang-lru/v2) as
the only implementation, wrapped by `CacheMetrics` (decorator that records Prometheus metrics). Cache wiring lives in
`app.makeCacheHandler` — if `cache.lru` is absent in YAML, the cache middleware is omitted from the chain entirely
(logged as "cache is disabled").

The HTTP-level cache logic is in `middlewares/cache.go`:

- **Key** = xxhash of `method + URL + routeName + Authorization + Cookie`. Authorization/Cookie are included so
  authenticated users don't read each other's cached responses.
- **Request rules**: only `GET`; bypassed if request has `Cache-Control: no-store` or `no-cache`.
- **Response rules**: skip if `Set-Cookie` / `Vary` present, or `Cache-Control` lists `no-store`/`no-cache`/`private`,
  or status `<200` / `>=400`, or body exceeds `cache.max_size_item`.
- Sets `ETag: <key>` on every cached/cacheable response; returns `304 Not Modified` on matching `If-None-Match`;
  adds `X-Cache: HIT` when serving from cache.
- `cacheResponseWriter` implements `Flush` so streaming responses still flush through the wrapper.

### Observability

- **Access log**: `httplog/v3` with OTEL schema, panics recovered, `route_name` added as an extra attribute.
- **Prometheus**: scrape endpoint on `/_metrics` (served by the `Prometheus` middleware short-circuiting the chain).
  All on the default registry via `promauto` (see the README table for the full list):
  - `http_requests_total{route_name,method,status}` / `http_request_duration_seconds` / `http_requests_in_flight` —
    middleware-level (sees the full chain including cache hits).
  - `http_proxy_requests_total{...}` / `http_proxy_request_duration_seconds` (success) and
    `http_proxy_errors_total{route_name,method,reason}` (transport failures) — proxy-level, recorded in
    `Proxy.ServeHTTP`. The `reason` is from `proxyErrorReason` (timeout/canceled/dns/connection_refused/tls/...).
  - `cache_hits_total` / `cache_misses_total` / `cache_lookup_latency_seconds` / `cache_skip_total{route_name,reason}`,
    plus `cache_size` — a scrape-time `GaugeFunc` reading the store's `Len()`, registered per `CacheMetrics` and
    unregistered on `Close()` (so don't construct two live `CacheMetrics` on the default registry at once).
  - `rparth_build_info{version,commit,date}` set to `1` in `main`.

## Release & container image

CI (`.github/workflows/test.yml`) runs the Ginkgo suite and `golangci-lint` on every push to `main` and every PR.
Releases are cut by pushing a `v*` tag, which triggers `.github/workflows/release.yml` → GoReleaser
(`goreleaser release --clean`).

- **Version injection is implicit.** `main.version` / `main.commit` / `main.date` are filled by GoReleaser's *default*
  ldflags (`-X main.version=… -X main.commit=… -X main.date=…`). There is deliberately **no `ldflags:` block** in
  `.goreleaser.yaml` — the var names already match the defaults. Renaming those vars silently breaks `rparth version`.
- **Container image** is built by the `dockers_v2` block and pushed to `ghcr.io/arthurhlt/rparth` (`:{{ .Version }}`,
  plus `:latest` for non-prerelease tags). `platforms:` is unset, so it defaults to `linux/amd64,linux/arm64` (multi-arch
  manifest). The release workflow sets up buildx and logs in to ghcr with `GITHUB_TOKEN`; the job needs
  `packages: write`. No QEMU is needed because the Dockerfile is `COPY`-only (no `RUN` executes target-arch code) — the
  binaries are cross-compiled by Go, not emulated.
- **Dockerfile** is `FROM alpine` and copies the prebuilt binary via `COPY $TARGETPLATFORM/rparth /usr/bin/`. That
  `<os>/<arch>/rparth` context layout is created by `dockers_v2`/buildx, **not** by a plain `docker build`. To build the
  image by hand you must recreate it (use your host arch, e.g. `arm64` on Apple Silicon):

  ```bash
  mkdir -p /tmp/ctx/linux/arm64
  CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -o /tmp/ctx/linux/arm64/rparth .
  cp Dockerfile /tmp/ctx/
  docker build --build-arg TARGETPLATFORM=linux/arm64 -t rparth:test /tmp/ctx
  ```
- **Runtime config resolution.** The image sets no `WORKDIR`, so the container cwd is `/` (a Docker default, not an
  alpine one). The proxy's default `./config.yml` therefore resolves to `/config.yml`, and the *relative*
  `cert_file`/`key_file` in the config resolve against cwd too — so mount config + certs at `/`, or pass an absolute
  `serve -c <path>`, or set `docker run -w <dir>`. Also note `listen_addr` must not be loopback (`127.0.0.1`) inside a
  container or the published port is unreachable from the host.
- Validate config edits with `goreleaser check`. `goreleaser release --snapshot --clean` dry-runs the whole pipeline,
  but the multi-arch image it builds can't be `--load`ed into the local daemon — use the manual build above for local
  image testing.

## Conventions

- Files ending in `.crap` and the `.crap/` directory are personal scratch (notes, throwaway snippets). Don't read them
  as authoritative and don't modify them unless asked.
- Errors are plain `errors.New` / `fmt.Errorf` (with `%w` where wrapping is useful) — no error wrapping framework.
- Per-package metrics go in a `metrics.go` file in that package, registered with `promauto` against the default
  registry. There is no central metrics module.
- Proxy tests use a `fakeTransport` (implements `http.RoundTripper`) that captures the forwarded request and returns a
  programmable response — prefer this over `httptest.Server` so assertions can inspect headers directly on
  `transport.received`.
- App-level tests should use `app.NewAppBare(cnf)` plus `SetMiddlewares` / `SetServerBuilder` to keep the unit under
  test small.
- Shared test helpers live in the `testutils` package (`RequestWithRoute`, `MustYamlParseURL`, `AssetPath`). Reach for
  them before inventing local equivalents.
- Per `.golangci.yml`, the `typecheck` linter is disabled in `*_test.go` files, and `errcheck` excludes a curated list
  of "we deliberately ignore this" functions (e.g. `(http.ResponseWriter).Write`, `xxhash.Digest.WriteString`). If you
  add another such call, extend the exclusion list rather than littering `_ = ...` assignments.
