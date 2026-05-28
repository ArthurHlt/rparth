package config

import (
	"fmt"
	"time"

	"github.com/goccy/go-yaml"
)

type Transport struct {
	Timeout               time.Duration `yaml:"timeout"`
	KeepAlive             time.Duration `yaml:"keepalive"`
	MaxIdleConns          int           `yaml:"max_idle_conns"`
	MaxIdleConnsPerHost   int           `yaml:"max_idle_conns_per_host"`
	IdleConnTimeout       time.Duration `yaml:"idle_conn_timeout"`
	ResponseHeaderTimeout time.Duration `yaml:"response_header_timeout"`
	TLSHandshakeTimeout   time.Duration `yaml:"tls_handshake_timeout"`
}

func (t *Transport) UnmarshalYAML(data []byte) error {
	type plain Transport
	err := yaml.Unmarshal(data, (*plain)(t))
	if err != nil {
		return fmt.Errorf("failed to unmarshal transport: %w", err)
	}
	if t.Timeout == 0 {
		t.Timeout = 30 * time.Second
	}
	if t.KeepAlive == 0 {
		t.KeepAlive = 30 * time.Second
	}

	// this is a transport for a proxy
	// so i enlarge connection pooling and lower down when connetion idle will timeout
	// to release faster than default, and it will be coherent with keepalive above
	if t.MaxIdleConns == 0 {
		t.MaxIdleConns = 1000
	}
	if t.MaxIdleConnsPerHost == 0 {
		t.MaxIdleConnsPerHost = 100
	}
	if t.IdleConnTimeout == 0 {
		t.IdleConnTimeout = 30 * time.Second
	}
	// ----

	if t.ResponseHeaderTimeout == 0 {
		t.ResponseHeaderTimeout = 30 * time.Second
	}
	if t.TLSHandshakeTimeout == 0 {
		t.TLSHandshakeTimeout = 10 * time.Second
	}
	return nil
}
