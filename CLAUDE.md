# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project status

This is an in-progress reverse proxy library (`github.com/ArthurHlt/rparth`, Go 1.26). `main.go` is intentionally
empty — the project is being built as packages first, with no CLI wiring yet.

## Commands

```bash
# Build (top-level main is currently empty, but verifies all packages compile)
go build ./...

# Run all tests
ginkgo --randomize-all --randomize-suites --race -p ./...

# Run a single package's suite
ginkgo --randomize-all --randomize-suites --race -p ./proxy
ginkgo --randomize-all --randomize-suites --race -p ./models

# Run a single Ginkgo spec by description
ginkgo --randomize-all --randomize-suites --race -p ./... --focus="<spec description regex>"

# Lint (use golangci-lint, not bare `go vet` — config-driven, runs vet plus the rest)
golangci-lint run ./...

# Format
gofmt -w .
```

Always run `golangci-lint run ./...` before considering work complete; do not substitute `go vet` alone.

Tests use **Ginkgo v2 + Gomega**. Each package has a `*_suite_test.go` bootstrap that calls `RunSpecs`; actual
`Describe`/`It` specs live in sibling `*_test.go` files inside the same `_test` package, so they exercise the public API
only.

## Architecture

Two packages collaborate to do reverse-proxying:

- **`models`** owns route configuration and matching. `RPRoute` is YAML-tagged (`yaml:"..."`) — config is intended to
  come from YAML (goccy/go-yaml is a dependency). `RPRoutes.FindRoute` walks routes in order and returns the first that
  `Match`es on host (optional) + path prefix. `Validate` defaults `Prefix` to `/`, `Timeout` to 30s, and `StripPrefix`
  to `true`; it also canonicalizes any `Headers` keys via `textproto.CanonicalMIMEHeaderKey`. **Note**: `Match` reads
  the request host from `req.Host` (the field) and runs it through `net.SplitHostPort`, falling back to the raw value
  when there's no port — so a bare host like `api.example.com` does match a route with `Host: "api.example.com"`. Tests
  set `req.Host` directly.
- **`proxy`** owns request forwarding. `Proxy` is constructed with a `RoundTripper` + `RPRoutes` and implements
  `http.Handler`. `roundTrip` clones the incoming request, rewrites `URL.Host`/`URL.Scheme` *and* `req.Host` (the field)
  to the matched route's `Target` — both are required, since `http.Transport` derives the wire-level `Host:` header
  from `req.Host`, not from `req.Header["Host"]`. It then sanitizes hop-by-hop headers on the request, injects
  per-route headers, then injects proxy-set forwarding headers (see below), forwards via the transport, and sanitizes
  hop-by-hop headers on the response. If `io.Copy` from the upstream body fails mid-stream, `ServeHTTP` only logs and
  returns — it does **not** call `http.Error`, because the response has already been committed.
  `DefaultProxyTransport` is a tuned `http.Transport` — bigger idle pool, shorter idle timeout, **compression disabled**
  so the response body can be streamed straight to the client.

### Hop-by-hop header handling

`proxy.go` keeps two parallel structures (`hopByHopHeadersMap` for O(1) membership checks, `hopByHopHeaders` slice for
iteration, populated from the map in `init`). Per RFC 7230, the `Connection` header can list additional
connection-scoped headers — `sanitizeHopByHopHeaders` strips both the canonical set and anything named in `Connection`.
Preserve this dual-structure pattern when adding hop-by-hop headers.

### Proxy-set forwarding headers

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

### HTTP/2 decision

Per `journey.md.crap`, HTTP/2 was deliberately **not** forced upstream: it would reduce TCP connections but complicates
response streaming (which this proxy relies on for body forwarding). If you reintroduce HTTP/2, you must rework the
streaming response path accordingly.

## Conventions

- Files ending in `.crap` and the `.crap/` directory are personal scratch (notes, throwaway snippets). Don't read them
  as authoritative and don't modify them unless asked.
- Errors are plain `errors.New` — no error wrapping framework is in use yet.
- Proxy tests use a `fakeTransport` (implements `http.RoundTripper`) that captures the forwarded request and returns a
  programmable response — prefer this over `httptest.Server` so assertions can inspect headers directly on
  `transport.received`.
