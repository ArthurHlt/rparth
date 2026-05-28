# rparth configuration — fully populated example using the in-process LRU cache.
# See README.md for the complete reference of every field and its default.

# Routes are evaluated top-to-bottom; the first one that matches (host + path prefix) wins.
routes:
  # Host-based route: only requests with Host "api.example.com" match.
  - name: api
    host: api.example.com
    prefix: /
    target: http://api-backend:8080
    strip_prefix: true
    timeout: 30
    no_cache: false
    headers:
      X-Proxied-By:
        - rparth

  # Prefix-based route: keep the /assets prefix when forwarding upstream.
  - name: static-assets
    prefix: /assets
    target: http://cdn-origin:8080
    strip_prefix: false
    timeout: 15

  # Catch-all example proxying to a public service.
  - name: httpbin
    prefix: /httpbin
    target: https://httpbin.org/

# HTTPS server. Drop the `tls` block to serve plain HTTP.
server:
  listen_addr: ":8443"
  tls:
    cert_file: ./cert.crt
    key_file: ./key.key

log:
  level: info
  no_color: false
  in_json: false

# Response cache backed by an in-process LRU (mutually exclusive with `redis`).
cache:
  max_size_item: 1048576 # 1 MiB; larger responses are streamed but not cached
  lru:
    size: 1000
    ttl: 10m

# Upstream HTTP transport tuning (all optional; values shown are the defaults).
transport:
  timeout: 30s
  keepalive: 30s
  max_idle_conns: 1000
  max_idle_conns_per_host: 100
  idle_conn_timeout: 30s
  response_header_timeout: 30s
  tls_handshake_timeout: 10s
