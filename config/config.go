package config

import (
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/ArthurHlt/rparth/models"
	"github.com/goccy/go-yaml"
)

type Config struct {
	Routes    models.RPRoutes `yaml:"routes"`
	Server    *Server         `yaml:"server"`
	Log       Log             `yaml:"log"`
	Cache     Cache           `yaml:"cache"`
	Transport Transport       `yaml:"transport"`
}

func (c *Config) UnmarshalYAML(data []byte) error {
	type plain Config
	err := yaml.Unmarshal(data, (*plain)(c))
	if err != nil {
		return fmt.Errorf("failed to unmarshal config: %w", err)
	}
	if c.Server == nil {
		c.Server = &Server{
			ListenAddr: defaultListenAddr,
		}
	}
	if len(c.Routes) == 0 {
		return fmt.Errorf("no routes configured")
	}
	return nil
}

func ReadConfig(configPath string) (*Config, error) {
	cnf := &Config{}
	f, err := os.Open(configPath)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	dec := yaml.NewDecoder(f,
		yaml.UseJSONUnmarshaler(),
	)
	err = dec.Decode(cnf)
	if err != nil {
		if errors.Is(err, io.EOF) {
			return nil, fmt.Errorf("config file is empty")
		}
		return nil, err
	}
	return cnf, nil
}
