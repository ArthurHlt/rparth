package config

import (
	"fmt"

	"github.com/goccy/go-yaml"
)

const defaultListenAddr = ":8080"

type Server struct {
	// ListenAddr is the address to listen on. Default is ":8080".
	ListenAddr string     `yaml:"listen_addr"`
	Tls        *ServerTLS `yaml:"tls"`
}

func (s *Server) UnmarshalYAML(data []byte) error {
	type plain Server
	err := yaml.Unmarshal(data, (*plain)(s))
	if err != nil {
		return fmt.Errorf("failed to unmarshal server config: %w", err)
	}
	if s.ListenAddr == "" {
		s.ListenAddr = defaultListenAddr
	}
	return nil
}

type ServerTLS struct {
	CertFile string `yaml:"cert_file"`
	KeyFile  string `yaml:"key_file"`
}

func (u *ServerTLS) UnmarshalYAML(data []byte) error {
	type plain ServerTLS
	err := yaml.Unmarshal(data, (*plain)(u))
	if err != nil {
		return fmt.Errorf("failed to unmarshal server tls config: %w", err)
	}
	if u.CertFile == "" || u.KeyFile == "" {
		return fmt.Errorf("server tls: cert_file and key_file are required")
	}
	return nil
}
