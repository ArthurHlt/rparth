package config

import (
	"fmt"

	"github.com/goccy/go-yaml"
)

const defaultListenAddr = ":8080"

type Server struct {
	// ListenAddr is the address to listen on. Default is ":8080".
	ListenAddr string `yaml:"listen_addr"`
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
